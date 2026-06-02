package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"light-ai/internal/eino"
	"light-ai/internal/kb"
	"light-ai/internal/storage"
)

// KnowledgeHandler 管理知识库的创建、文档上传和检索。
type KnowledgeHandler struct {
	ctx context.Context
}

func NewKnowledgeHandler() *KnowledgeHandler {
	return &KnowledgeHandler{}
}

func (h *KnowledgeHandler) SetContext(ctx context.Context) {
	h.ctx = ctx
}

// kbBaseDir 返回知识库根目录
func kbBaseDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".wails-chat", "knowledgebases")
}

// kbDir 返回单个知识库目录
func kbDir(kbID string) string {
	return filepath.Join(kbBaseDir(), kbID)
}

// ListKnowledgeBases 列出所有知识库
func (h *KnowledgeHandler) ListKnowledgeBases() ([]storage.KnowledgeBase, error) {
	return storage.ListKnowledgeBases()
}

// CreateKnowledgeBase 新建知识库，同时创建文件系统目录
func (h *KnowledgeHandler) CreateKnowledgeBase(name, description string) (*storage.KnowledgeBase, error) {
	if name == "" {
		return nil, fmt.Errorf("知识库名称不能为空")
	}
	kb, err := storage.CreateKnowledgeBase(name, description)
	if err != nil {
		return nil, err
	}
	// 预创建目录（GetStore 也会创建，这里提前确保）
	if err := os.MkdirAll(kbDir(kb.ID), 0755); err != nil {
		slog.Warn("CreateKnowledgeBase: mkdir failed", "id", kb.ID, "error", err)
	}
	return kb, nil
}

// DeleteKnowledgeBase 删除知识库（主库记录 + 文件系统）
func (h *KnowledgeHandler) DeleteKnowledgeBase(id string) error {
	kb.CloseStore(id)
	if err := storage.DeleteKnowledgeBase(id); err != nil {
		return err
	}
	return os.RemoveAll(kbDir(id))
}

// ListDocuments 列出知识库内的文档
func (h *KnowledgeHandler) ListDocuments(kbID string) ([]kb.KBDocument, error) {
	s, err := kb.GetStore(kbID, kbDir(kbID))
	if err != nil {
		return nil, err
	}
	return s.ListDocuments()
}

// GetDocumentStatus 查询文档处理状态（前端轮询用）
func (h *KnowledgeHandler) GetDocumentStatus(kbID, docID string) (string, error) {
	s, err := kb.GetStore(kbID, kbDir(kbID))
	if err != nil {
		return "error", err
	}
	return s.GetDocumentStatus(docID)
}

// DeleteDocument 删除文档（含原始文件和所有 chunks）
func (h *KnowledgeHandler) DeleteDocument(kbID, docID string) error {
	s, err := kb.GetStore(kbID, kbDir(kbID))
	if err != nil {
		return err
	}
	// 删除原始文件（glob 匹配 {docID}_*）
	pattern := filepath.Join(s.FilesDir(), docID+"_*")
	matches, _ := filepath.Glob(pattern)
	for _, m := range matches {
		os.Remove(m)
	}
	if err := s.DeleteDocumentChunks(docID); err != nil {
		return err
	}
	return storage.IncrKBDocCount(kbID, -1)
}

const maxKBFileSize = 50 * 1024 * 1024 // 50MB

// PickAndUploadDocuments 弹出系统文件选择框，选中后直接读取处理，无需前端传文件内容。
func (h *KnowledgeHandler) PickAndUploadDocuments(kbID string) ([]kb.KBDocument, error) {
	paths, err := runtime.OpenMultipleFilesDialog(h.ctx, runtime.OpenDialogOptions{
		Title: "选择文档",
		Filters: []runtime.FileFilter{
			{DisplayName: "文档文件", Pattern: "*.pdf;*.docx;*.xlsx;*.txt;*.md;*.csv;*.json;*.yaml;*.xml;*.html"},
			{DisplayName: "代码文件", Pattern: "*.go;*.py;*.js;*.ts;*.java;*.sql;*.sh;*.rs;*.cpp;*.c"},
			{DisplayName: "所有文件", Pattern: "*"},
		},
	})
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, nil
	}

	store, err := kb.GetStore(kbID, kbDir(kbID))
	if err != nil {
		return nil, err
	}

	var docs []kb.KBDocument
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			slog.Warn("PickAndUploadDocuments: stat failed", "path", p, "error", err)
			continue
		}
		if info.Size() > maxKBFileSize {
			slog.Warn("PickAndUploadDocuments: file too large", "path", p, "size", info.Size())
			continue
		}
		mimeType := mime.TypeByExtension(filepath.Ext(p))
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		name := filepath.Base(p)
		docID, err := store.InsertDocument(name, mimeType, info.Size())
		if err != nil {
			slog.Warn("PickAndUploadDocuments: insert doc failed", "name", name, "error", err)
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			store.UpdateDocumentStatus(docID, "error", err.Error(), 0)
			continue
		}
		destPath := filepath.Join(store.FilesDir(), docID+"_"+name)
		os.WriteFile(destPath, data, 0644)
		go processDocument(store, kbID, docID, name, data)
		docs = append(docs, kb.KBDocument{
			ID: docID, Name: name, MimeType: mimeType,
			Size: info.Size(), Status: "processing",
		})
	}
	return docs, nil
}

// processDocument 异步解析文档、写入 FTS5、生成摘要、向量化
func processDocument(store *kb.Store, kbID, docID, name string, data []byte) {
	text, err := kb.ParseText(data, name)
	if err != nil {
		slog.Warn("processDocument: parse failed", "name", name, "error", err)
		store.UpdateDocumentStatus(docID, "error", err.Error(), 0)
		return
	}
	if text == "" {
		store.UpdateDocumentStatus(docID, "error", "文档无可提取文本（可能是扫描版或加密文件）", 0)
		return
	}
	chunks := kb.SplitChunks(text)
	if err := store.InsertChunks(docID, chunks); err != nil {
		slog.Warn("processDocument: insert chunks failed", "name", name, "error", err)
		store.UpdateDocumentStatus(docID, "error", err.Error(), 0)
		return
	}
	store.UpdateDocumentStatus(docID, "ready", "", len(chunks))
	storage.IncrKBDocCount(kbID, 1)

	// TODO1: 异步生成摘要（不阻塞主流程）
	go func() {
		if err := generateAndStoreSummary(store, docID, name, text); err != nil {
			slog.Warn("processDocument: summary failed", "name", name, "error", err)
		}
	}()

	// TODO2: 异步向量化（不阻塞主流程）
	go func() {
		if err := kb.VectorizeDocument(store, docID, chunks); err != nil {
			slog.Warn("processDocument: vectorize failed", "name", name, "error", err)
		}
	}()
}

// generateAndStoreSummary 调用 LLM 生成文档摘要和关键实体，存入 summaries 表
func generateAndStoreSummary(store *kb.Store, docID, docName, fullText string) error {
	// 取前 3000 字符作为摘要生成的输入（避免超出 context）
	runes := []rune(fullText)
	if len(runes) > 3000 {
		runes = runes[:3000]
	}

	// 获取第一个可用的 enabled provider + model
	providers, err := storage.ListProviders()
	if err != nil || len(providers) == 0 {
		return fmt.Errorf("no provider available")
	}
	var provider *storage.LLMProvider
	for i := range providers {
		if providers[i].Enabled {
			provider = &providers[i]
			break
		}
	}
	if provider == nil {
		return fmt.Errorf("no enabled provider")
	}
	models, err := storage.ListModels(provider.ID)
	if err != nil || len(models) == 0 {
		return fmt.Errorf("no models for provider %s", provider.Name)
	}

	chat := eino.NewChatService()
	if err := chat.Configure(provider.Type, models[0].Name, provider.APIKey, provider.BaseURL); err != nil {
		return fmt.Errorf("configure LLM: %w", err)
	}

	prompt := fmt.Sprintf(`请分析以下文档内容，以JSON格式返回：
{
  "summary": "100字以内的文档摘要，概括主题、主要人物、核心事件",
  "key_entities": ["实体1", "实体2"]
}

只返回JSON，不要任何其他内容。key_entities最多10个关键人名/地名/概念。

文档名：%s
文档内容（前3000字）：
%s`, docName, string(runes))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := chat.Generate(ctx, []*schema.Message{
		{Role: schema.User, Content: prompt},
	})
	if err != nil {
		return fmt.Errorf("LLM generate: %w", err)
	}

	raw := strings.TrimSpace(resp.Content)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	var result struct {
		Summary     string   `json:"summary"`
		KeyEntities []string `json:"key_entities"`
	}
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		slog.Warn("generateSummary: JSON parse failed, using raw", "raw", raw)
		result.Summary = raw
	}
	if result.Summary == "" {
		return fmt.Errorf("empty summary from LLM")
	}

	entitiesJSON, _ := json.Marshal(result.KeyEntities)
	return store.UpsertSummary(docID, docName, result.Summary, string(entitiesJSON))
}

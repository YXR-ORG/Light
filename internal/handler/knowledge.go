package handler

import (
	"context"
	"fmt"
	"log/slog"
	"mime"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2/pkg/runtime"

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

// processDocument 异步解析文档并写入 FTS5 索引
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
}

package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"

	mcpTool "github.com/cloudwego/eino-ext/components/tool/mcp"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"light-ai/internal/eino"
	"light-ai/internal/storage"
)
// maxToolLoops prevents infinite tool-call cycles.
const maxToolLoops = 15

type ChatHandler struct {
	chat     *eino.ChatService
	ctx      context.Context
	cancel   context.CancelFunc
	cancelMu sync.Mutex
}

func NewChatHandler(chat *eino.ChatService) *ChatHandler {
	return &ChatHandler{chat: chat}
}

func (h *ChatHandler) SetContext(ctx context.Context) {
	h.ctx = ctx
}

type Attachment struct {
	Name     string `json:"name"`
	MimeType string `json:"mime_type"`
	Data     string `json:"data"` // base64
}

type SendMessageRequest struct {
	ConversationID  string       `json:"conversation_id"`
	Content         string       `json:"content"`
	Provider        string       `json:"provider"`
	Model           string       `json:"model"`
	AgentID         string       `json:"agent_id"`
	MCPServerIDs    []string     `json:"mcp_server_ids"`
	SkillIDs        []string     `json:"skill_ids"`
	WebSearch       bool         `json:"web_search"`
	IgnoreContext   bool         `json:"ignore_context"`
	ContextCutoffID string       `json:"context_cutoff_id"`
	Attachments     []Attachment `json:"attachments"`
	Mode            string       `json:"mode"`            // "chat" | "knowledge"
	KnowledgeBaseID string       `json:"knowledge_base_id"` // kb_id，mode=knowledge 时有效
}

type SendMessageResponse struct {
	MessageID string `json:"message_id"`
}

type StreamChunk struct {
	Content  string `json:"content"`
	Thinking string `json:"thinking,omitempty"`
	Done     bool   `json:"done"`
	Error    string `json:"error,omitempty"`
}

func (h *ChatHandler) CancelStream() {
	h.cancelMu.Lock()
	defer h.cancelMu.Unlock()
	if h.cancel != nil {
		h.cancel()
		h.cancel = nil
	}
}

const maxAttachmentSize = 10 * 1024 * 1024 // 10MB

// kbDirForChat 返回知识库目录（与 KnowledgeHandler 保持一致）
func kbDirForChat(kbID string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".wails-chat", "knowledgebases", kbID)
}

// PickAttachments 弹出系统文件选择框，读取文件内容并返回附件列表（含 base64 data）。
// 由后端完成文件读取，前端无需处理文件 IO 或 base64 编码。
func (h *ChatHandler) PickAttachments() ([]Attachment, error) {
	paths, err := runtime.OpenMultipleFilesDialog(h.ctx, runtime.OpenDialogOptions{
		Title: "选择附件",
		Filters: []runtime.FileFilter{
			{DisplayName: "图片", Pattern: "*.png;*.jpg;*.jpeg;*.gif;*.webp;*.bmp"},
			{DisplayName: "文档", Pattern: "*.txt;*.md;*.csv;*.json;*.yaml;*.xml;*.html;*.pdf"},
			{DisplayName: "代码", Pattern: "*.go;*.py;*.js;*.ts;*.java;*.sql;*.sh;*.rs;*.cpp;*.c"},
			{DisplayName: "所有文件", Pattern: "*"},
		},
	})
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, nil
	}

	var attachments []Attachment
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			slog.Warn("PickAttachments: read file failed", "path", p, "error", err)
			continue
		}
		if len(data) > maxAttachmentSize {
			slog.Warn("PickAttachments: file too large, skipped", "path", p, "size", len(data))
			continue
		}
		mimeType := mime.TypeByExtension(strings.ToLower(filepath.Ext(p)))
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		// 去掉参数（如 "text/plain; charset=utf-8" → "text/plain"）
		mimeType = strings.SplitN(mimeType, ";", 2)[0]
		attachments = append(attachments, Attachment{
			Name:     filepath.Base(p),
			MimeType: mimeType,
			Data:     base64.StdEncoding.EncodeToString(data),
		})
	}
	return attachments, nil
}

// loadMCPTools connects to all enabled MCP servers and returns their tools.
// Errors are logged but do not abort — partial tool sets are acceptable.
func (h *ChatHandler) loadMCPTools(ctx context.Context) []tool.BaseTool {
	servers, err := storage.ListMCPServers()
	if err != nil {
		slog.Warn("loadMCPTools: list servers failed", "error", err)
		return nil
	}

	var allTools []tool.BaseTool
	for _, srv := range servers {
		if !srv.Enabled {
			continue
		}
		tools := h.connectAndGetTools(ctx, srv)
		allTools = append(allTools, tools...)
	}
	return allTools
}

// loadSelectedMCPTools 只加载 selectedIDs 中指定的 MCP 服务器工具。
// 若 selectedIDs 为空，不加载任何 MCP 工具。
func (h *ChatHandler) loadSelectedMCPTools(ctx context.Context, selectedIDs []string) []tool.BaseTool {
	if len(selectedIDs) == 0 {
		return nil
	}
	servers, err := storage.ListMCPServers()
	if err != nil {
		slog.Warn("loadSelectedMCPTools: list servers failed", "error", err)
		return nil
	}
	idSet := make(map[string]bool, len(selectedIDs))
	for _, id := range selectedIDs {
		idSet[id] = true
	}
	var allTools []tool.BaseTool
	for _, srv := range servers {
		if !srv.Enabled || !idSet[srv.ID] {
			continue
		}
		allTools = append(allTools, h.connectAndGetTools(ctx, srv)...)
	}
	return allTools
}

func (h *ChatHandler) connectAndGetTools(ctx context.Context, srv storage.MCPServer) []tool.BaseTool {
	connCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	var cli *mcpclient.Client
	var err error

	switch srv.Type {
	case "sse":
		cli, err = mcpclient.NewSSEMCPClient(srv.URL)
	default: // stdio
		args := parseArgs(srv.Args)
		envPairs := parseEnv(srv.Env)
		cli, err = mcpclient.NewStdioMCPClient(srv.Command, envPairs, args...)
	}
	if err != nil {
		slog.Warn("MCP client create failed", "server", srv.Name, "error", err)
		return nil
	}

	if err = cli.Start(connCtx); err != nil {
		slog.Warn("MCP client start failed", "server", srv.Name, "error", err)
		return nil
	}

	_, err = cli.Initialize(connCtx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo:      mcp.Implementation{Name: "light-ai", Version: "1.0.0"},
		},
	})
	if err != nil {
		slog.Warn("MCP initialize failed", "server", srv.Name, "error", err)
		cli.Close()
		return nil
	}

	tools, err := mcpTool.GetTools(connCtx, &mcpTool.Config{Cli: cli})
	if err != nil {
		slog.Warn("MCP GetTools failed", "server", srv.Name, "error", err)
		cli.Close()
		return nil
	}

	slog.Info("MCP tools loaded", "server", srv.Name, "count", len(tools))
	return tools
}

func (h *ChatHandler) StreamChat(req SendMessageRequest) error {
	ctx, cancel := context.WithCancel(h.ctx)
	h.cancelMu.Lock()
	if h.cancel != nil {
		h.cancel()
	}
	h.cancel = cancel
	h.cancelMu.Unlock()

	defer func() {
		h.cancelMu.Lock()
		h.cancel = nil
		h.cancelMu.Unlock()
	}()

	slog.Info("StreamChat start", "provider", req.Provider, "model", req.Model, "conv_id", req.ConversationID)

	apiKey, _ := storage.GetSetting(fmt.Sprintf("%s_api_key", req.Provider))
	baseURL, _ := storage.GetSetting(fmt.Sprintf("%s_base_url", req.Provider))
	slog.Info("StreamChat settings", "provider", req.Provider, "has_key", apiKey != "", "base_url", baseURL)

	if req.Provider != "ollama" && apiKey == "" {
		err := fmt.Errorf("请先在设置中配置 %s 的 API Key", req.Provider)
		runtime.EventsEmit(h.ctx, "chat:chunk", StreamChunk{Done: true, Error: err.Error()})
		return err
	}

	if err := h.chat.Configure(req.Provider, req.Model, apiKey, baseURL); err != nil {
		runtime.EventsEmit(h.ctx, "chat:chunk", StreamChunk{Done: true, Error: err.Error()})
		return err
	}

	// Load MCP tools + selected skill tools + web search, bind to model
	allTools := h.loadSelectedMCPTools(ctx, req.MCPServerIDs)
	if req.WebSearch {
		engine, _ := storage.GetSetting("search_engine") // tavily|exa|brave|searxng
		if engine == "" {
			engine = "tavily"
		}
		var apiKey string
		switch engine {
		case "exa":
			apiKey, _ = storage.GetSetting("exa_api_key")
		case "brave":
			apiKey, _ = storage.GetSetting("brave_api_key")
		case "searxng":
			apiKey, _ = storage.GetSetting("searxng_url") // holds instance URL
		default:
			apiKey, _ = storage.GetSetting("tavily_api_key")
			engine = "tavily"
		}
		if apiKey == "" && engine != "searxng" {
			runtime.EventsEmit(h.ctx, "chat:chunk", StreamChunk{Done: true,
				Error: fmt.Sprintf("请先在设置中配置 %s 的 API Key", engine)})
			return fmt.Errorf("%s api key not configured", engine)
		}
		slog.Info("WebSearch enabled", "engine", engine, "has_key", apiKey != "")
		allTools = append(allTools, eino.NewWebSearchTool(engine, apiKey, 5))
	}
	if len(req.SkillIDs) > 0 {
		for _, sid := range req.SkillIDs {
			skill, err := storage.GetSkill(sid)
			if err != nil {
				slog.Warn("skill not found", "id", sid)
				continue
			}
			allTools = append(allTools, eino.NewSkillTool(skill.ID, skill.Name, skill.Description, skill.Content))
		}
	}
	// 知识库模式：绑定 search_knowledge tool
	if req.Mode == "knowledge" && req.KnowledgeBaseID != "" {
		kbPath := kbDirForChat(req.KnowledgeBaseID)
		if kbTool, err := eino.NewKnowledgeSearchTool(req.KnowledgeBaseID, kbPath); err != nil {
			slog.Warn("KnowledgeSearchTool init failed", "kb", req.KnowledgeBaseID, "error", err)
		} else {
			allTools = append(allTools, kbTool)
		}
	}
	slog.Info("BindTools", "count", len(allTools), "web_search", req.WebSearch)
	if len(allTools) > 0 {
		if err := h.chat.BindTools(ctx, allTools); err != nil {
			slog.Warn("BindTools failed, continuing without tools", "error", err)
		}
	}

	// Serialize attachment metadata (no base64 data, just name/type/size)
	attachmentsMeta := ""
	if len(req.Attachments) > 0 {
		type meta struct {
			Name     string `json:"name"`
			MimeType string `json:"mime_type"`
		}
		metas := make([]meta, len(req.Attachments))
		for i, a := range req.Attachments {
			metas[i] = meta{Name: a.Name, MimeType: a.MimeType}
		}
		if b, err := json.Marshal(metas); err == nil {
			attachmentsMeta = string(b)
		}
	}

	mcpJSON, _ := json.Marshal(req.MCPServerIDs)
	if _, err := storage.SaveMessage(req.ConversationID, "user", req.Content, "", "", "", req.AgentID, string(mcpJSON), attachmentsMeta); err != nil {		slog.Error("StreamChat save user message failed", "error", err)
	}

	// Check if this is the first user message (for auto title generation)
	isFirstMessage := false
	if prevMsgs, err := storage.GetMessages(req.ConversationID); err == nil {
		isFirstMessage = len(prevMsgs) == 1
	}

	history, err := storage.GetMessages(req.ConversationID)
	if err != nil {
		runtime.EventsEmit(h.ctx, "chat:chunk", StreamChunk{Done: true, Error: err.Error()})
		return err
	}

	var einoMsgs []*schema.Message

	// Prepend system prompt if the conversation has one
	conv, convErr := storage.GetConversation(req.ConversationID)

	// Always inject current date + web search hint when web search is enabled
	dateInfo := fmt.Sprintf("Today's date is %s.", time.Now().Format("2006-01-02 (Monday)"))
	var systemContent string
	if convErr == nil && conv.SystemPrompt != "" {
		systemContent = conv.SystemPrompt + "\n\n" + dateInfo
	} else {
		systemContent = dateInfo
	}
	if req.WebSearch {
		systemContent += "\nYou have access to a web_search tool. Use it to find current information. After 1-2 searches, synthesize the results and provide a final answer."
	}
	if req.Mode == "knowledge" && req.KnowledgeBaseID != "" {
		systemContent += "\nYou have access to a search_knowledge tool. When answering questions that require document knowledge, you MUST call search_knowledge first. Always cite your sources."
	}
	einoMsgs = append(einoMsgs, &schema.Message{
		Role:    schema.System,
		Content: systemContent,
	})

	if req.IgnoreContext {
		einoMsgs = append(einoMsgs, buildUserMessage(req.Content, req.Attachments))
	} else if req.ContextCutoffID != "" {
		cutoffPassed := false
		for _, m := range history {
			if m.ID == req.ContextCutoffID {
				cutoffPassed = true
				continue
			}
			if cutoffPassed {
				einoMsgs = append(einoMsgs, &schema.Message{
					Role:    storage.ToEinoRole(m.Role),
					Content: m.Content,
				})
			}
		}
	} else {
		// All history except the last user message (which we rebuild with attachments)
		for i, m := range history {
			if i == len(history)-1 && m.Role == "user" {
				einoMsgs = append(einoMsgs, buildUserMessage(m.Content, req.Attachments))
			} else {
				einoMsgs = append(einoMsgs, historyToEinoMsg(m))
			}
		}
	}

	fullContent, fullThinking := h.runToolLoop(ctx, einoMsgs)

	if _, err := storage.SaveMessage(req.ConversationID, "assistant", fullContent, fullThinking, "", "", "", ""); err != nil {
		slog.Error("StreamChat save assistant message failed", "error", err)
	}

	// Auto-generate title for the first message asynchronously
	if isFirstMessage && req.Content != "" {
		convID := req.ConversationID
		userMsg := req.Content
		appCtx := h.ctx
		go func() {
			titleCtx, titleCancel := context.WithTimeout(appCtx, 10*time.Second)
			defer titleCancel()
			prompt := fmt.Sprintf("请用5个字以内总结这个问题的主题，只输出标题，不要标点符号：%s", userMsg)
			msgs := []*schema.Message{{Role: schema.User, Content: prompt}}
			title := ""
			resp, err := h.chat.Chat(titleCtx, msgs)
			if err == nil {
				title = strings.TrimSpace(resp.Content)
			}
			// 规则兜底：截取前 12 个字符，去掉标点
			if title == "" {
				title = extractTitle(userMsg, 12)
			}
			if title == "" {
				return
			}
			if err := storage.UpdateConversationTitle(convID, title); err != nil {
				slog.Warn("update conversation title failed", "error", err)
				return
			}
			runtime.EventsEmit(appCtx, "conversation:updated", convID)
			slog.Info("auto title generated", "conv_id", convID, "title", title)
		}()
	}

	return nil
}

// extractTitle extracts a short title from text by taking the first maxChars
// runes and stripping leading punctuation/whitespace.
func extractTitle(text string, maxChars int) string {
	text = strings.TrimSpace(text)
	// Strip common leading punctuation
	text = strings.TrimLeft(text, "，。！？,.!? \t\n\r")
	runes := []rune(text)
	if len(runes) > maxChars {
		runes = runes[:maxChars]
	}
	return strings.TrimSpace(string(runes))
}
// buildUserMessage constructs a user message, with optional multimodal attachments.
func buildUserMessage(content string, attachments []Attachment) *schema.Message {
	if len(attachments) == 0 {
		return &schema.Message{Role: schema.User, Content: content}
	}
	parts := []schema.MessageInputPart{}
	if content != "" {
		parts = append(parts, schema.MessageInputPart{
			Type: schema.ChatMessagePartTypeText,
			Text: content,
		})
	}
	for _, a := range attachments {
		slog.Debug("buildUserMessage attachment",
			"name", a.Name,
			"mime_type", a.MimeType,
			"data_len", len(a.Data),
		)
		// 统一小写 mime type，去掉参数（如 "; charset=utf-8"）
		mimeType := strings.ToLower(strings.SplitN(a.MimeType, ";", 2)[0])
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		if strings.HasPrefix(mimeType, "image/") {
			b64 := a.Data
			parts = append(parts, schema.MessageInputPart{
				Type: schema.ChatMessagePartTypeImageURL,
				Image: &schema.MessageInputImage{
					MessagePartCommon: schema.MessagePartCommon{
						Base64Data: &b64,
						MIMEType:   mimeType,
					},
				},
			})
		} else {
			decoded, err := base64.StdEncoding.DecodeString(a.Data)
			if err != nil {
				slog.Warn("buildUserMessage: base64 decode failed", "name", a.Name, "error", err)
				// 降级：直接把 data 当文本（可能是纯文本文件直接传来的）
				parts = append(parts, schema.MessageInputPart{
					Type: schema.ChatMessagePartTypeText,
					Text: fmt.Sprintf("\n\n[文件: %s]\n%s", a.Name, a.Data),
				})
			} else {
				parts = append(parts, schema.MessageInputPart{
					Type: schema.ChatMessagePartTypeText,
					Text: fmt.Sprintf("\n\n[文件: %s]\n%s", a.Name, string(decoded)),
				})
			}
		}
	}
	return &schema.Message{Role: schema.User, UserInputMultiContent: parts}
}

// historyToEinoMsg 将数据库历史消息还原为 eino Message，保留 tool call 信息。
func historyToEinoMsg(m storage.Message) *schema.Message {
	msg := &schema.Message{
		Role:    storage.ToEinoRole(m.Role),
		Content: m.Content,
	}
	// 还原 assistant 的 tool_calls
	if m.ToolCalls != "" {
		var tc []schema.ToolCall
		if err := json.Unmarshal([]byte(m.ToolCalls), &tc); err == nil {
			msg.ToolCalls = tc
		}
	}
	// 还原 tool 消息的 tool_call_id 和 tool_name
	if m.Role == "tool" && m.ToolResult != "" {
		// ToolResult 格式: {"tool_call_id":"...","tool_name":"...","content":"..."}
		var tr struct {
			ToolCallID string `json:"tool_call_id"`
			ToolName   string `json:"tool_name"`
		}
		if err := json.Unmarshal([]byte(m.ToolResult), &tr); err == nil {
			msg.ToolCallID = tr.ToolCallID
			msg.ToolName = tr.ToolName
		}
	}
	return msg
}

func (h *ChatHandler) runToolLoop(ctx context.Context, messages []*schema.Message) (string, string) {	fullContent := ""
	fullThinking := ""

	for loopCount := 0; loopCount < maxToolLoops; loopCount++ {
		stream, err := h.chat.Stream(ctx, messages)
		if err != nil {
			slog.Error("StreamChat stream failed", "error", err)
			runtime.EventsEmit(h.ctx, "chat:chunk", StreamChunk{Done: true, Error: err.Error()})
			return fullContent, fullThinking
		}

		var chunks []*schema.Message
		chunkContent := ""

		for {
			chunk, recvErr := stream.Recv()
			if recvErr != nil {
				break
			}
			chunks = append(chunks, chunk)
			if chunk.ReasoningContent != "" {
				fullThinking += chunk.ReasoningContent
				runtime.EventsEmit(h.ctx, "chat:chunk", StreamChunk{Thinking: chunk.ReasoningContent})
			}
			if chunk.Content != "" {
				chunkContent += chunk.Content
				fullContent += chunk.Content
				runtime.EventsEmit(h.ctx, "chat:chunk", StreamChunk{Content: chunk.Content})
			}
		}
		stream.Close()

		// Merge all streaming chunks into one complete message (handles tool call argument merging)
		var toolCalls []schema.ToolCall
		if len(chunks) > 0 {
			merged, mergeErr := schema.ConcatMessages(chunks)
			if mergeErr == nil && merged != nil {
				toolCalls = merged.ToolCalls
			}
		}

		slog.Info("runToolLoop iteration", "loop", loopCount, "tool_calls", len(toolCalls), "content_len", len(chunkContent))

		if len(toolCalls) == 0 {
			break
		}

		messages = append(messages, &schema.Message{
			Role:      schema.Assistant,
			Content:   chunkContent,
			ToolCalls: toolCalls,
		})

		for _, tc := range toolCalls {
			toolName := tc.Function.Name
			toolArgs := tc.Function.Arguments
			slog.Info("Calling tool", "name", toolName, "args", toolArgs)
			runtime.EventsEmit(h.ctx, "chat:chunk", StreamChunk{
				Content: fmt.Sprintf("\n🔧 调用工具: %s...\n", toolName),
			})
			result, toolErr := h.chat.RunTool(ctx, toolName, toolArgs)
			if toolErr != nil {
				slog.Warn("Tool call failed", "name", toolName, "error", toolErr)
				result = fmt.Sprintf("工具调用失败: %v", toolErr)
			}
			messages = append(messages, &schema.Message{
				Role:       schema.Tool,
				Content:    result,
				ToolCallID: tc.ID,
			})
		}
		fullContent = ""
	}

	runtime.EventsEmit(h.ctx, "chat:chunk", StreamChunk{Done: true})
	return fullContent, fullThinking
}


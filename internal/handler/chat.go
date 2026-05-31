package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
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
	SkillIDs        []string     `json:"skill_ids"`
	WebSearch       bool         `json:"web_search"`
	IgnoreContext   bool         `json:"ignore_context"`
	ContextCutoffID string       `json:"context_cutoff_id"`
	Attachments     []Attachment `json:"attachments"`
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
	allTools := h.loadMCPTools(ctx)
	if req.WebSearch {
		tavilyKey, _ := storage.GetSetting("tavily_api_key")
		slog.Info("WebSearch enabled", "has_key", tavilyKey != "", "key_prefix", func() string {
			if len(tavilyKey) > 8 { return tavilyKey[:8] + "..." }
			return tavilyKey
		}())
		if tavilyKey == "" {
			runtime.EventsEmit(h.ctx, "chat:chunk", StreamChunk{Done: true, Error: "请先在设置中配置 Tavily API Key"})
			return fmt.Errorf("tavily api key not configured")
		}
		allTools = append(allTools, eino.NewWebSearchTool(tavilyKey, 5))
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

	if _, err := storage.SaveMessage(req.ConversationID, "user", req.Content, "", "", attachmentsMeta); err != nil {		slog.Error("StreamChat save user message failed", "error", err)
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
				einoMsgs = append(einoMsgs, &schema.Message{
					Role:    storage.ToEinoRole(m.Role),
					Content: m.Content,
				})
			}
		}
	}

	fullContent, fullThinking := h.runToolLoop(ctx, einoMsgs)

	if _, err := storage.SaveMessage(req.ConversationID, "assistant", fullContent, fullThinking, "", ""); err != nil {
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
		if strings.HasPrefix(a.MimeType, "image/") {
			b64 := a.Data
			mimeType := a.MimeType
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
			if err == nil {
				parts = append(parts, schema.MessageInputPart{
					Type: schema.ChatMessagePartTypeText,
					Text: fmt.Sprintf("\n\n[文件: %s]\n%s", a.Name, string(decoded)),
				})
			}
		}
	}
	return &schema.Message{Role: schema.User, UserInputMultiContent: parts}
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


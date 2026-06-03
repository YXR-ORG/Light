package handler

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"light-ai/internal/eino"
	"light-ai/internal/storage"
)

// TaskHandler 处理 task 模式的所有前端调用。
type TaskHandler struct {
	chat *eino.ChatService
	ctx  context.Context

	cancelMu sync.Mutex
	cancels  map[string]context.CancelFunc // convID → cancel

	bashToolMu sync.Mutex
	bashTools  map[string]*eino.BashTool // convID → BashTool（当前活跃）
}

// NewTaskHandler 创建 TaskHandler。
func NewTaskHandler(chat *eino.ChatService) *TaskHandler {
	return &TaskHandler{
		chat:      chat,
		cancels:   make(map[string]context.CancelFunc),
		bashTools: make(map[string]*eino.BashTool),
	}
}

// SetContext 由 app.startup 调用，注入 Wails context。
func (h *TaskHandler) SetContext(ctx context.Context) {
	h.ctx = ctx
}

// StreamTaskRequest 前端发送给 StreamTask 的请求体。
type StreamTaskRequest struct {
	ConversationID    string `json:"conversation_id"`
	Content           string `json:"content"`
	WorkDir           string `json:"work_dir"`
	Provider          string `json:"provider"`
	Model             string `json:"model"`
	AgentID           string `json:"agent_id"`
	RegenerateGroupID string `json:"regenerate_group_id"`
	IgnoreContext     bool   `json:"ignore_context"`
}

// StreamTask 启动 task 模式 ReAct Agent，通过 task:step events 推送推理链。
func (h *TaskHandler) StreamTask(req StreamTaskRequest) error {
	// 校验工作目录
	if req.WorkDir == "" {
		home, _ := os.UserHomeDir()
		req.WorkDir = storage.GetSettingWithDefault("task_default_work_dir", home+"/Documents")
	}
	if _, err := os.Stat(req.WorkDir); err != nil {
		msg := fmt.Sprintf("工作目录不存在: %s", req.WorkDir)
		runtime.EventsEmit(h.ctx, "task:step", eino.TaskStep{ConvID: req.ConversationID, Type: "error", Error: msg})
		return fmt.Errorf(msg)
	}

	// 取消前一个同 conv 的 task
	ctx, cancel := context.WithCancel(h.ctx)
	h.cancelMu.Lock()
	if old, ok := h.cancels[req.ConversationID]; ok {
		old()
	}
	h.cancels[req.ConversationID] = cancel
	h.cancelMu.Unlock()

	defer func() {
		h.cancelMu.Lock()
		delete(h.cancels, req.ConversationID)
		h.cancelMu.Unlock()
	}()

	// 配置 LLM
	var providerType, apiKey, baseURL string
	if p, err := storage.GetProvider(req.Provider); err == nil {
		apiKey = p.APIKey
		baseURL = p.BaseURL
		providerType = p.Type
	} else {
		providerType = req.Provider
		apiKey, _ = storage.GetSetting(fmt.Sprintf("%s_api_key", req.Provider))
		baseURL, _ = storage.GetSetting(fmt.Sprintf("%s_base_url", req.Provider))
	}
	if providerType != "ollama" && apiKey == "" {
		msg := fmt.Sprintf("请先在设置中配置 %s 的 API Key", req.Provider)
		runtime.EventsEmit(h.ctx, "task:step", eino.TaskStep{ConvID: req.ConversationID, Type: "error", Error: msg})
		return fmt.Errorf(msg)
	}
	if err := h.chat.Configure(providerType, req.Model, apiKey, baseURL); err != nil {
		runtime.EventsEmit(h.ctx, "task:step", eino.TaskStep{ConvID: req.ConversationID, Type: "error", Error: err.Error()})
		return err
	}

	llm := h.chat.GetToolCallingModel()
	if llm == nil {
		msg := "当前模型不支持 tool calling，请切换模型"
		runtime.EventsEmit(h.ctx, "task:step", eino.TaskStep{ConvID: req.ConversationID, Type: "error", Error: msg})
		return fmt.Errorf(msg)
	}
	slog.Info("StreamTask: LLM configured", "provider", req.Provider, "model", req.Model)

	// 保存用户消息（标记 mode=task）
	// 判断是否需要生成标题：会话标题还是默认值时生成
	needTitle := false
	if conv, err := storage.GetConversation(req.ConversationID); err == nil {
		needTitle = conv.Title == "New Chat" || conv.Title == ""
	}
	if _, err := storage.SaveTaskMessage(req.ConversationID, "user", req.Content); err != nil {
		slog.Warn("StreamTask: save user message failed", "error", err)
	}

	// 发送 user_msg 事件，触发前端初始化流式 UI
	runtime.EventsEmit(h.ctx, "task:step", eino.TaskStep{
		ConvID: req.ConversationID, Type: "user_msg", Content: req.Content,
	})

	// 加载历史，只保留 task 模式的消息（过滤 chat/knowledge 模式的历史，避免污染 agent 行为）
	var einoHistory []*schema.Message
	if !req.IgnoreContext {
		history, err := storage.GetLatestMessages(req.ConversationID)
		if err != nil {
			slog.Warn("StreamTask: load history failed", "error", err)
		}
		var taskHistory []storage.Message
		for _, m := range history {
			if m.Mode == "task" || m.Mode == "" {
				taskHistory = append(taskHistory, m)
			}
		}
		einoHistory = historyToEinoMsgs(taskHistory)
		// 移除最后一条 user 消息（本轮，已单独传入 userMsg）
		if len(einoHistory) > 0 {
			last := einoHistory[len(einoHistory)-1]
			if last.Role == "user" {
				einoHistory = einoHistory[:len(einoHistory)-1]
			}
		}
	}
	slog.Info("StreamTask: history", "len", len(einoHistory), "ignore_context", req.IgnoreContext)

	// BashTool emitter（通道注入在 RunTaskAgent 内部）
	var bashTool *eino.BashTool
	tools := eino.BuildTaskTools(ctx, req.WorkDir, func(stepType, content, cmd, confirmID string) {
		runtime.EventsEmit(h.ctx, "task:step", eino.TaskStep{
			ConvID: req.ConversationID, Type: stepType, Content: content, Cmd: cmd, ConfirmID: confirmID,
		})
	})
	// 找出 BashTool 引用
	for _, t := range tools {
		if bt, ok := t.(*eino.BashTool); ok {
			bashTool = bt
			break
		}
	}

	// 注册 BashTool，供 ConfirmBash 调用
	if bashTool != nil {
		h.bashToolMu.Lock()
		h.bashTools[req.ConversationID] = bashTool
		h.bashToolMu.Unlock()
		defer func() {
			h.bashToolMu.Lock()
			delete(h.bashTools, req.ConversationID)
			h.bashToolMu.Unlock()
		}()
	}

	// 更新工作目录到数据库
	_ = storage.UpdateConversationWorkDir(req.ConversationID, req.WorkDir)

	// 启动 ReAct agent
	slog.Info("StreamTask: starting RunTaskAgent", "workDir", req.WorkDir, "historyLen", len(einoHistory))
	stepCh, err := eino.RunTaskAgent(ctx, llm, tools, bashTool, req.WorkDir, einoHistory, req.Content)
	if err != nil {
		runtime.EventsEmit(h.ctx, "task:step", eino.TaskStep{ConvID: req.ConversationID, Type: "error", Error: err.Error()})
		return err
	}

	// 消费 step channel，转发 task:step events，收集最终回答
	var finalContent string
	hadDone := false
	for step := range stepCh {
		step.ConvID = req.ConversationID
		slog.Info("StreamTask: step", "type", step.Type, "content_len", len(step.Content))
		if step.Type == "content" {
			finalContent += step.Content
		}
		if step.Type == "done" {
			hadDone = true
			// 先保存 AI 回答到 DB，再发 done 事件，确保前端 loadTaskHistory 能读到数据
			slog.Info("StreamTask: done received", "finalContent_len", len(finalContent), "convID", req.ConversationID)
			if finalContent != "" {
				msgID, err := storage.SaveTaskMessage(req.ConversationID, "assistant", finalContent)
				if err != nil {
					slog.Warn("StreamTask: save assistant message failed", "error", err)
				} else {
					slog.Info("StreamTask: assistant message saved", "msgID", msgID)
				}
			} else {
				slog.Warn("StreamTask: finalContent is empty, skipping SaveMessage")
			}
			runtime.EventsEmit(h.ctx, "task:step", step)
			// 自动生成标题（会话标题还是默认值时异步生成）
			if needTitle && req.Content != "" {
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
					if title == "" {
						title = extractTitle(userMsg, 12)
					}
					if title == "" {
						return
					}
					if err := storage.UpdateConversationTitle(convID, title); err != nil {
						slog.Warn("StreamTask: update title failed", "error", err)
						return
					}
					runtime.EventsEmit(appCtx, "conversation:updated", convID)
					slog.Info("StreamTask: auto title generated", "conv_id", convID, "title", title)
				}()
			}
			break
		}
		if step.Type == "error" {
			runtime.EventsEmit(h.ctx, "task:step", step)
			break
		}
		runtime.EventsEmit(h.ctx, "task:step", step)
	}

	// channel 关闭但没有 done（context canceled / 超时）→ 补发 done 保证前端不卡住
	if !hadDone {
		slog.Info("StreamTask: agent stopped without done, emitting fallback done", "finalContent_len", len(finalContent), "convID", req.ConversationID)
		if finalContent != "" {
			msgID, err := storage.SaveTaskMessage(req.ConversationID, "assistant", finalContent)
			if err != nil {
				slog.Warn("StreamTask: save assistant message failed (fallback)", "error", err)
			} else {
				slog.Info("StreamTask: assistant message saved (fallback)", "msgID", msgID)
			}
		}
		runtime.EventsEmit(h.ctx, "task:step", eino.TaskStep{
			ConvID: req.ConversationID, Type: "done",
		})
		// 自动生成标题
		if needTitle && req.Content != "" {
			convID := req.ConversationID
			appCtx := h.ctx
			go func() {
				titleCtx, titleCancel := context.WithTimeout(appCtx, 10*time.Second)
				defer titleCancel()
				prompt := fmt.Sprintf("请用5个字以内总结这个问题的主题，只输出标题，不要标点符号：%s", req.Content)
				msgs := []*schema.Message{{Role: schema.User, Content: prompt}}
				title := ""
				resp, err := h.chat.Chat(titleCtx, msgs)
				if err == nil {
					title = strings.TrimSpace(resp.Content)
				}
				if title == "" {
					title = extractTitle(req.Content, 12)
				}
				if title == "" {
					return
				}
				if err := storage.UpdateConversationTitle(convID, title); err != nil {
					return
				}
				runtime.EventsEmit(appCtx, "conversation:updated", convID)
			}()
		}
	}

	return nil
}

// ConfirmBash 由前端在 BashConfirmDialog 用户点击后调用。
func (h *TaskHandler) ConfirmBash(convID, confirmID string, approved bool) error {
	h.bashToolMu.Lock()
	bt, ok := h.bashTools[convID]
	h.bashToolMu.Unlock()
	if !ok {
		return fmt.Errorf("no active bash tool for conv %s", convID)
	}
	bt.Confirm(confirmID, approved)
	return nil
}

// StopTask 中断正在运行的 task。
func (h *TaskHandler) StopTask(convID string) error {
	h.cancelMu.Lock()
	cancel, ok := h.cancels[convID]
	h.cancelMu.Unlock()
	if ok {
		cancel()
	}
	return nil
}

// SelectWorkDir 弹出系统文件夹选择对话框，返回选择的路径。
func (h *TaskHandler) SelectWorkDir() (string, error) {
	dir, err := runtime.OpenDirectoryDialog(h.ctx, runtime.OpenDialogOptions{
		Title: "选择工作目录",
	})
	if err != nil {
		return "", err
	}
	return dir, nil
}

// historyToEinoMsgs 将 storage.Message 列表转换为 eino schema.Message 列表。
func historyToEinoMsgs(msgs []storage.Message) []*schema.Message {
	result := make([]*schema.Message, 0, len(msgs))
	for _, m := range msgs {
		msg := historyToEinoMsg(m)
		if msg != nil {
			result = append(result, msg)
		}
	}
	return result
}

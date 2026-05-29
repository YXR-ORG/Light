package handler

import (
	"context"
	"fmt"
	"log/slog"

	"wails-ai-chat/internal/eino"
	"wails-ai-chat/internal/storage"

	"github.com/cloudwego/eino/schema"
)

type ChatHandler struct {
	chat *eino.ChatService
}

func NewChatHandler(chat *eino.ChatService) *ChatHandler {
	return &ChatHandler{chat: chat}
}

type SendMessageRequest struct {
	ConversationID string `json:"conversation_id"`
	Content        string `json:"content"`
	Provider       string `json:"provider"`
	Model          string `json:"model"`
}

type SendMessageResponse struct {
	MessageID string `json:"message_id"`
}

func (h *ChatHandler) SendMessage(ctx context.Context, req SendMessageRequest) (*SendMessageResponse, error) {
	apiKey, _ := storage.GetSetting(fmt.Sprintf("%s_api_key", req.Provider))
	baseURL, _ := storage.GetSetting(fmt.Sprintf("%s_base_url", req.Provider))

	if err := h.chat.Configure(req.Provider, req.Model, apiKey, baseURL); err != nil {
		return nil, err
	}

	_, err := storage.SaveMessage(req.ConversationID, "user", req.Content, "", "")
	if err != nil {
		slog.Error("save user message failed", "error", err)
	}

	history, err := storage.GetMessages(req.ConversationID)
	if err != nil {
		return nil, err
	}

	var einoMsgs []*schema.Message
	for _, m := range history {
		einoMsgs = append(einoMsgs, &schema.Message{
			Role:    storage.ToEinoRole(m.Role),
			Content: m.Content,
		})
	}

	result, err := h.chat.Chat(ctx, einoMsgs)
	if err != nil {
		return nil, err
	}

	assistantMsg, err := storage.SaveMessage(req.ConversationID, "assistant", result.Content, "", "")
	if err != nil {
		slog.Error("save assistant message failed", "error", err)
	}

	return &SendMessageResponse{MessageID: assistantMsg.ID}, nil
}

type StreamChunk struct {
	Content string `json:"content"`
	Done    bool   `json:"done"`
	Error   string `json:"error,omitempty"`
}

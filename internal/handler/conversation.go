package handler

import (
	"wails-ai-chat/internal/storage"
)

type ConversationHandler struct{}

func NewConversationHandler() *ConversationHandler {
	return &ConversationHandler{}
}

func (h *ConversationHandler) Create(provider, model string) (*storage.Conversation, error) {
	return storage.CreateConversation(provider, model)
}

func (h *ConversationHandler) List() ([]storage.Conversation, error) {
	return storage.ListConversations()
}

func (h *ConversationHandler) Get(id string) (*storage.Conversation, error) {
	return storage.GetConversation(id)
}

func (h *ConversationHandler) Rename(id, title string) error {
	return storage.UpdateConversationTitle(id, title)
}

func (h *ConversationHandler) Delete(id string) error {
	return storage.DeleteConversation(id)
}

func (h *ConversationHandler) GetMessages(convID string) ([]storage.Message, error) {
	return storage.GetMessages(convID)
}

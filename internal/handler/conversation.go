package handler

import (
	"light-ai/internal/storage"
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

func (h *ConversationHandler) Search(query string) ([]storage.Conversation, error) {
	if query == "" {
		return storage.ListConversations()
	}
	return storage.SearchConversations(query)
}

func (h *ConversationHandler) Delete(id string) error {
	return storage.DeleteConversation(id)
}

func (h *ConversationHandler) SetSystemPrompt(id, prompt string) error {
	return storage.UpdateSystemPrompt(id, prompt)
}

func (h *ConversationHandler) SetModel(id, provider, model string) error {
	return storage.UpdateConversationModel(id, provider, model)
}

func (h *ConversationHandler) GetMessages(convID string) ([]storage.Message, error) {
	return storage.GetMessages(convID)
}

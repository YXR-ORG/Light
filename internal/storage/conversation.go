package storage

import (
	"time"

	"gorm.io/gorm"
)

func CreateConversation(provider, model string) (*Conversation, error) {
	c := &Conversation{
		ID:       NewID(),
		Provider: provider,
		Model:    model,
	}
	err := DB.Create(c).Error
	return c, err
}

func GetConversation(id string) (*Conversation, error) {
	var c Conversation
	err := DB.First(&c, "id = ?", id).Error
	return &c, err
}

func ListConversations() ([]Conversation, error) {
	var list []Conversation
	err := DB.Order("updated_at DESC").Find(&list).Error
	return list, err
}

func UpdateConversationTitle(id, title string) error {
	return DB.Model(&Conversation{}).Where("id = ?", id).
		Updates(map[string]any{"title": title, "updated_at": time.Now()}).Error
}

func DeleteConversation(id string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		tx.Where("conversation_id = ?", id).Delete(&Message{})
		tx.Delete(&Conversation{}, "id = ?", id)
		return nil
	})
}

func SaveMessage(convID, role, content, toolCalls, toolResult string) (*Message, error) {
	m := &Message{
		ID:             NewID(),
		ConversationID: convID,
		Role:           role,
		Content:        content,
		ToolCalls:      toolCalls,
		ToolResult:     toolResult,
	}
	err := DB.Create(m).Error
	if err == nil {
		DB.Model(&Conversation{}).Where("id = ?", convID).
			Update("updated_at", time.Now())
	}
	return m, err
}

func GetMessages(convID string) ([]Message, error) {
	var msgs []Message
	err := DB.Where("conversation_id = ?", convID).
		Order("created_at ASC").Find(&msgs).Error
	return msgs, err
}

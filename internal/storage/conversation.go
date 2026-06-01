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

func SearchConversations(query string) ([]Conversation, error) {
	var list []Conversation
	err := DB.Where("title LIKE ?", "%"+query+"%").
		Order("updated_at DESC").Find(&list).Error
	return list, err
}

func UpdateSystemPrompt(id, prompt string) error {
	return DB.Model(&Conversation{}).Where("id = ?", id).
		Updates(map[string]any{"system_prompt": prompt, "updated_at": time.Now()}).Error
}

// SetAgent 更新对话的智能体 ID 和 system_prompt
func SetAgent(convID, agentID, systemPrompt string) error {
	return DB.Model(&Conversation{}).Where("id = ?", convID).
		Updates(map[string]any{
			"agent_id":      agentID,
			"system_prompt": systemPrompt,
			"updated_at":    time.Now(),
		}).Error
}

func UpdateConversationModel(id, provider, model string) error {
	return DB.Model(&Conversation{}).Where("id = ?", id).
		Updates(map[string]any{"provider": provider, "model": model, "updated_at": time.Now()}).Error
}

func DeleteConversation(id string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		tx.Where("conversation_id = ?", id).Delete(&Message{})
		tx.Delete(&Conversation{}, "id = ?", id)
		return nil
	})
}

func SaveMessage(convID, role, content, thinking, toolCalls, toolResult, agentID, mcpServerIDs string, attachments ...string) (*Message, error) {
	m := &Message{
		ID:             NewID(),
		ConversationID: convID,
		Role:           role,
		Content:        content,
		Thinking:       thinking,
		ToolCalls:      toolCalls,
		ToolResult:     toolResult,
		AgentID:        agentID,
		MCPServerIDs:   mcpServerIDs,
	}
	if len(attachments) > 0 {
		m.Attachments = attachments[0]
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

package storage

import (
	"time"

	"github.com/google/uuid"
	"github.com/cloudwego/eino/schema"
	"gorm.io/gorm"
)

type Conversation struct {
	ID        string    `gorm:"primaryKey;size:36" json:"id"`
	Title     string    `gorm:"size:256;not null;default:'New Chat'" json:"title"`
	Provider  string    `gorm:"size:32;not null" json:"provider"`
	Model     string    `gorm:"size:64;not null" json:"model"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Message struct {
	ID             string    `gorm:"primaryKey;size:36" json:"id"`
	ConversationID string    `gorm:"index;size:36;not null" json:"conversation_id"`
	Role           string    `gorm:"size:16;not null" json:"role"`
	Content        string    `gorm:"type:text" json:"content"`
	ToolCalls      string    `gorm:"type:text" json:"tool_calls,omitempty"`
	ToolResult     string    `gorm:"type:text" json:"tool_result,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type Setting struct {
	Key   string `gorm:"primaryKey;size:64" json:"key"`
	Value string `gorm:"type:text" json:"value"`
}

func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(&Conversation{}, &Message{}, &Setting{})
}

func NewID() string {
	return uuid.New().String()
}

func ToEinoRole(role string) schema.RoleType {
	switch role {
	case "user":
		return schema.User
	case "assistant":
		return schema.Assistant
	case "system":
		return schema.System
	case "tool":
		return schema.Tool
	default:
		return schema.User
	}
}

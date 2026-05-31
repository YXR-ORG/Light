package storage

import (
	"time"

	"github.com/google/uuid"
	"github.com/cloudwego/eino/schema"
	"gorm.io/gorm"
)

type Conversation struct {
	ID           string    `gorm:"primaryKey;size:36" json:"id"`
	Title        string    `gorm:"size:256;not null;default:'New Chat'" json:"title"`
	Provider     string    `gorm:"size:32;not null" json:"provider"`
	Model        string    `gorm:"size:64;not null" json:"model"`
	SystemPrompt string    `gorm:"type:text;default:''" json:"system_prompt"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Message struct {
	ID             string    `gorm:"primaryKey;size:36" json:"id"`
	ConversationID string    `gorm:"index;size:36;not null" json:"conversation_id"`
	Role           string    `gorm:"size:16;not null" json:"role"`
	Content        string    `gorm:"type:text" json:"content"`
	Thinking       string    `gorm:"type:text" json:"thinking,omitempty"`
	ToolCalls      string    `gorm:"type:text" json:"tool_calls,omitempty"`
	ToolResult     string    `gorm:"type:text" json:"tool_result,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type Setting struct {
	Key   string `gorm:"primaryKey;size:64" json:"key"`
	Value string `gorm:"type:text" json:"value"`
}

type MCPServer struct {
	ID        string `gorm:"primaryKey;size:36" json:"id"`
	Name      string `gorm:"size:128;not null" json:"name"`
	Type      string `gorm:"size:16;not null" json:"type"`
	URL       string `gorm:"size:512" json:"url"`
	Command   string `gorm:"size:256" json:"command"`
	Args      string `gorm:"type:text" json:"args"`
	Env       string `gorm:"type:text" json:"env"`
	Enabled   bool   `gorm:"default:true" json:"enabled"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type LLMProvider struct {
	ID        string `gorm:"primaryKey;size:36" json:"id"`
	Name      string `gorm:"size:128;not null" json:"name"`
	Type      string `gorm:"size:16;not null" json:"type"` // openai|google|claude|ollama
	APIKey    string `gorm:"type:text" json:"api_key"`
	BaseURL   string `gorm:"size:512" json:"base_url"`
	Enabled   bool   `gorm:"default:false" json:"enabled"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type LLMModel struct {
	ID         string `gorm:"primaryKey;size:36" json:"id"`
	ProviderID string `gorm:"index;size:36;not null" json:"provider_id"`
	Name       string `gorm:"size:128;not null" json:"name"`
	CreatedAt  string `json:"created_at"`
}

type Agent struct {
	ID           string `gorm:"primaryKey;size:36" json:"id"`
	Name         string `gorm:"size:64;not null" json:"name"`
	Icon         string `gorm:"size:16" json:"icon"`
	Description  string `gorm:"size:256" json:"description"`
	SystemPrompt string `gorm:"type:text" json:"system_prompt"`
	SortOrder    int    `gorm:"default:0" json:"sort_order"`
	Builtin      bool   `gorm:"default:false" json:"builtin"`
}

type Skill struct {
	ID          string `gorm:"primaryKey;size:36" json:"id"`
	Name        string `gorm:"size:128;not null" json:"name"`
	Description string `gorm:"size:512" json:"description"`
	Content     string `gorm:"type:text" json:"content"` // SKILL.md body
	Enabled     bool   `gorm:"default:false" json:"enabled"`
	SortOrder   int    `gorm:"default:0" json:"sort_order"`
}

func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(&Conversation{}, &Message{}, &Setting{}, &MCPServer{}, &LLMProvider{}, &LLMModel{}, &Agent{}, &Skill{})
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

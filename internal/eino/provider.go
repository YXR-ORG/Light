package eino

import (
	"context"

	"github.com/cloudwego/eino/components/model"
)

type ProviderConfig struct {
	Provider string
	Model    string
	APIKey   string
	BaseURL  string
}

type ChatProvider interface {
	CreateChatModel(ctx context.Context, cfg ProviderConfig) (model.ChatModel, error)
}

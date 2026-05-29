package eino

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/cloudwego/eino-ext/components/model/claude"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type ChatService struct {
	mu       sync.RWMutex
	provider string
	model    string
	apiKey   string
	baseURL  string
	llm      model.ChatModel
}

func NewChatService() *ChatService {
	return &ChatService{}
}

func (s *ChatService) Configure(provider, modelName, apiKey, baseURL string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.provider = provider
	s.model = modelName
	s.apiKey = apiKey
	s.baseURL = baseURL

	var err error
	switch provider {
	case "openai":
		s.llm, err = openai.NewChatModel(context.Background(), &openai.ChatModelConfig{
			Model:   modelName,
			APIKey:  apiKey,
			BaseURL: baseURL,
		})
	case "claude":
		claudeCfg := &claude.Config{
			Model:  modelName,
			APIKey: apiKey,
		}
		if baseURL != "" {
			claudeCfg.BaseURL = &baseURL
		}
		s.llm, err = claude.NewChatModel(context.Background(), claudeCfg)
	case "ollama":
		if baseURL == "" {
			baseURL = "http://localhost:11434"
		}
		s.llm, err = openai.NewChatModel(context.Background(), &openai.ChatModelConfig{
			Model:   modelName,
			APIKey:  "ollama",
			BaseURL: fmt.Sprintf("%s/v1", baseURL),
		})
	default:
		err = fmt.Errorf("不支持的供应商: %s", provider)
	}
	if err != nil {
		slog.Error("创建对话模型失败", "provider", provider, "error", err)
	}
	return err
}

func (s *ChatService) Chat(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
	s.mu.RLock()
	llm := s.llm
	s.mu.RUnlock()

	if llm == nil {
		return nil, fmt.Errorf("对话模型未配置")
	}
	result, err := llm.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("生成失败: %w", err)
	}
	return result, nil
}

func (s *ChatService) Stream(ctx context.Context, messages []*schema.Message) (*schema.StreamReader[*schema.Message], error) {
	s.mu.RLock()
	llm := s.llm
	s.mu.RUnlock()

	if llm == nil {
		return nil, fmt.Errorf("对话模型未配置")
	}
	return llm.Stream(ctx, messages)
}

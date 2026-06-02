package handler

import (
	"context"
	"fmt"
	"time"

	"light-ai/internal/eino"
	"light-ai/internal/storage"

	"github.com/cloudwego/eino/schema"
)

type ProviderHandler struct{}

func NewProviderHandler() *ProviderHandler { return &ProviderHandler{} }

func (h *ProviderHandler) ListProviders() ([]storage.LLMProvider, error) {
	return storage.ListProviders()
}

func (h *ProviderHandler) SaveProvider(p storage.LLMProvider) (storage.LLMProvider, error) {
	if err := storage.SaveProvider(&p); err != nil {
		return storage.LLMProvider{}, err
	}
	return p, nil
}

func (h *ProviderHandler) DeleteProvider(id string) error {
	return storage.DeleteProvider(id)
}

func (h *ProviderHandler) ToggleProvider(id string, enabled bool) error {
	return storage.ToggleProvider(id, enabled)
}

func (h *ProviderHandler) ListModels(providerID string) ([]storage.LLMModel, error) {
	return storage.ListModels(providerID)
}

func (h *ProviderHandler) ListEnabledModels() ([]storage.LLMModel, error) {
	return storage.ListAllEnabledModels()
}

func (h *ProviderHandler) AddModel(m storage.LLMModel) error {
	return storage.AddModel(&m)
}

func (h *ProviderHandler) DeleteModel(id string) error {
	return storage.DeleteModel(id)
}

// TestConnection 测试 provider 连接是否可用，返回空字符串表示成功，否则返回错误信息
func (h *ProviderHandler) TestConnection(p storage.LLMProvider) string {
	if p.APIKey == "" && p.Type != "ollama" {
		return "API Key 未填写"
	}

	// 找第一个可用的模型名
	modelName := ""
	if models, err := storage.ListModels(p.ID); err == nil && len(models) > 0 {
		modelName = models[0].Name
	}
	if modelName == "" {
		// 用各 provider 的已知默认模型兜底
		defaults := map[string]string{
			"openai":  "gpt-4o-mini",
			"claude":  "claude-3-haiku-20240307",
			"google":  "gemini-1.5-flash",
			"ollama":  "llama3",
		}
		modelName = defaults[p.Type]
		if modelName == "" {
			modelName = "gpt-4o-mini"
		}
	}

	chat := eino.NewChatService()
	if err := chat.Configure(p.Type, modelName, p.APIKey, p.BaseURL); err != nil {
		return fmt.Sprintf("配置失败：%v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_, err := chat.Generate(ctx, []*schema.Message{
		{Role: schema.User, Content: "hi"},
	})
	if err != nil {
		return fmt.Sprintf("连接失败：%v", err)
	}
	return "" // 成功
}

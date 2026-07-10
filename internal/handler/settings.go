package handler

import (
	"light-ai/internal/storage"
)

type SettingsHandler struct{}

func NewSettingsHandler() *SettingsHandler {
	return &SettingsHandler{}
}

func (h *SettingsHandler) Get(key string) (string, error) {
	return storage.GetSetting(key)
}

func (h *SettingsHandler) Set(key, value string) error {
	return storage.SetSetting(key, value)
}

func (h *SettingsHandler) GetAll() ([]storage.Setting, error) {
	return storage.GetAllSettings()
}

// GetUsageStats 返回用量汇总。rangeKey: today|week|month|all
func (h *SettingsHandler) GetUsageStats(rangeKey string) (*storage.UsageSummary, error) {
	return storage.GetUsageSummary(rangeKey)
}

// GetConversationUsage 返回指定会话累计 token 与估算费用。
func (h *SettingsHandler) GetConversationUsage(convID string) (map[string]interface{}, error) {
	total, cost, err := storage.GetConversationTokenTotal(convID)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"total_tokens":   total,
		"estimated_cost": cost,
	}, nil
}

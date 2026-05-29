package handler

import (
	"wails-ai-chat/internal/storage"
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

package handler

import "light-ai/internal/storage"

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

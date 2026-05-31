package storage

import "time"

func ListProviders() ([]LLMProvider, error) {
	var list []LLMProvider
	err := DB.Order("created_at ASC").Find(&list).Error
	return list, err
}

func GetProvider(id string) (*LLMProvider, error) {
	var p LLMProvider
	err := DB.First(&p, "id = ?", id).Error
	return &p, err
}

func SaveProvider(p *LLMProvider) error {
	now := time.Now().Format(time.RFC3339)
	if p.ID == "" {
		p.ID = NewID()
		p.CreatedAt = now
	}
	p.UpdatedAt = now
	return DB.Save(p).Error
}

func DeleteProvider(id string) error {
	DB.Where("provider_id = ?", id).Delete(&LLMModel{})
	return DB.Delete(&LLMProvider{}, "id = ?", id).Error
}

func ToggleProvider(id string, enabled bool) error {
	return DB.Model(&LLMProvider{}).Where("id = ?", id).
		Updates(map[string]any{"enabled": enabled, "updated_at": time.Now().Format(time.RFC3339)}).Error
}

func ListModels(providerID string) ([]LLMModel, error) {
	var list []LLMModel
	err := DB.Where("provider_id = ?", providerID).Order("created_at ASC").Find(&list).Error
	return list, err
}

func ListAllEnabledModels() ([]LLMModel, error) {
	var list []LLMModel
	err := DB.Joins("JOIN llm_providers ON llm_providers.id = llm_models.provider_id").
		Where("llm_providers.enabled = ?", true).
		Order("llm_models.created_at ASC").Find(&list).Error
	return list, err
}

func AddModel(m *LLMModel) error {
	if m.ID == "" {
		m.ID = NewID()
	}
	m.CreatedAt = time.Now().Format(time.RFC3339)
	return DB.Create(m).Error
}

func DeleteModel(id string) error {
	return DB.Delete(&LLMModel{}, "id = ?", id).Error
}

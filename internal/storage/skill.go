package storage

func ListSkills() ([]Skill, error) {
	var list []Skill
	err := DB.Order("sort_order ASC, name ASC").Find(&list).Error
	return list, err
}

func ListEnabledSkills() ([]Skill, error) {
	var list []Skill
	err := DB.Where("enabled = ?", true).Order("sort_order ASC").Find(&list).Error
	return list, err
}

func GetSkill(id string) (*Skill, error) {
	var s Skill
	err := DB.First(&s, "id = ?", id).Error
	return &s, err
}

func SaveSkill(s *Skill) error {
	if s.ID == "" {
		s.ID = NewID()
	}
	return DB.Save(s).Error
}

func ToggleSkill(id string, enabled bool) error {
	return DB.Model(&Skill{}).Where("id = ?", id).Update("enabled", enabled).Error
}

func DeleteSkill(id string) error {
	return DB.Delete(&Skill{}, "id = ?", id).Error
}

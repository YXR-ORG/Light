package storage

func GetSetting(key string) (string, error) {
	var s Setting
	err := DB.First(&s, "key = ?", key).Error
	return s.Value, err
}

func SetSetting(key, value string) error {
	return DB.Where("key = ?", key).Assign(Setting{Value: value}).
		FirstOrCreate(&Setting{Key: key, Value: value}).Error
}

func GetAllSettings() ([]Setting, error) {
	var list []Setting
	err := DB.Find(&list).Error
	return list, err
}

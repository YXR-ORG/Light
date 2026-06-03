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

// GetSettingWithDefault 读取 setting，若不存在或为空则返回 defaultVal。
func GetSettingWithDefault(key, defaultVal string) string {
	val, err := GetSetting(key)
	if err != nil || val == "" {
		return defaultVal
	}
	return val
}

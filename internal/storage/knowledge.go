package storage

import "time"

// ListKnowledgeBases 列出所有知识库
func ListKnowledgeBases() ([]KnowledgeBase, error) {
	var kbs []KnowledgeBase
	if err := DB.Order("created_at desc").Find(&kbs).Error; err != nil {
		return nil, err
	}
	return kbs, nil
}

// CreateKnowledgeBase 新建知识库
func CreateKnowledgeBase(name, description string) (*KnowledgeBase, error) {
	kb := &KnowledgeBase{
		ID:          NewID(),
		Name:        name,
		Description: description,
		DocCount:    0,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := DB.Create(kb).Error; err != nil {
		return nil, err
	}
	return kb, nil
}

// GetKnowledgeBase 按 ID 查询
func GetKnowledgeBase(id string) (*KnowledgeBase, error) {
	var kb KnowledgeBase
	if err := DB.First(&kb, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &kb, nil
}

// DeleteKnowledgeBase 从主库删除记录（文件系统清理由 handler 负责）
func DeleteKnowledgeBase(id string) error {
	return DB.Delete(&KnowledgeBase{}, "id = ?", id).Error
}

// IncrKBDocCount 增减知识库文档计数
func IncrKBDocCount(id string, delta int) error {
	return DB.Model(&KnowledgeBase{}).
		Where("id = ?", id).
		UpdateColumn("doc_count", DB.Raw("doc_count + ?", delta)).Error
}

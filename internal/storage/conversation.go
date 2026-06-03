package storage

import (
	"time"

	"gorm.io/gorm"
)

func CreateConversation(provider, model string) (*Conversation, error) {
	c := &Conversation{
		ID:       NewID(),
		Provider: provider,
		Model:    model,
	}
	err := DB.Create(c).Error
	return c, err
}

func GetConversation(id string) (*Conversation, error) {
	var c Conversation
	err := DB.First(&c, "id = ?", id).Error
	return &c, err
}

func ListConversations() ([]Conversation, error) {
	var list []Conversation
	err := DB.Order("created_at DESC").Find(&list).Error
	return list, err
}

func UpdateConversationTitle(id, title string) error {
	return DB.Model(&Conversation{}).Where("id = ?", id).
		UpdateColumn("title", title).Error
}

func SearchConversations(query string) ([]Conversation, error) {
	var list []Conversation
	err := DB.Where("title LIKE ?", "%"+query+"%").
		Order("created_at DESC").Find(&list).Error
	return list, err
}

func UpdateSystemPrompt(id, prompt string) error {
	return DB.Model(&Conversation{}).Where("id = ?", id).
		Updates(map[string]any{"system_prompt": prompt, "updated_at": time.Now()}).Error
}

// SetAgent 更新对话的智能体 ID 和 system_prompt
func SetAgent(convID, agentID, systemPrompt string) error {
	return DB.Model(&Conversation{}).Where("id = ?", convID).
		Updates(map[string]any{
			"agent_id":      agentID,
			"system_prompt": systemPrompt,
			"updated_at":    time.Now(),
		}).Error
}

func UpdateConversationModel(id, provider, model string) error {
	return DB.Model(&Conversation{}).Where("id = ?", id).
		Updates(map[string]any{"provider": provider, "model": model, "updated_at": time.Now()}).Error
}

// UpdateConversationMode 保存对话的输入模式和知识库选择
func UpdateConversationMode(id, mode, knowledgeBaseID string) error {
	return DB.Model(&Conversation{}).Where("id = ?", id).
		UpdateColumns(map[string]any{"mode": mode, "knowledge_base_id": knowledgeBaseID}).Error
}

// ToggleFavorite 切换对话收藏状态，返回切换后的值（不更新 updated_at，不影响排序）
func ToggleFavorite(id string) (bool, error) {
	var c Conversation
	if err := DB.First(&c, "id = ?", id).Error; err != nil {
		return false, err
	}
	newVal := !c.Starred
	err := DB.Model(&Conversation{}).Where("id = ?", id).
		UpdateColumn("starred", newVal).Error
	return newVal, err
}

// ListFavorites 返回所有已收藏的对话
func ListFavorites() ([]Conversation, error) {
	var list []Conversation
	err := DB.Where("starred = ?", true).Order("created_at DESC").Find(&list).Error
	return list, err
}

func DeleteConversation(id string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		tx.Where("conversation_id = ?", id).Delete(&Message{})
		tx.Delete(&Conversation{}, "id = ?", id)
		return nil
	})
}

func SaveMessage(convID, role, content, thinking, toolCalls, toolResult, agentID, mcpServerIDs string, attachments ...string) (*Message, error) {
	id := NewID()
	m := &Message{
		ID:                id,
		ConversationID:    convID,
		Role:              role,
		Content:           content,
		Thinking:          thinking,
		ToolCalls:         toolCalls,
		ToolResult:        toolResult,
		AgentID:           agentID,
		MCPServerIDs:      mcpServerIDs,
		GenerationGroupID: id, // 默认 group = 自身，首次生成
		GenIndex:          0,
	}
	if len(attachments) > 0 {
		m.Attachments = attachments[0]
	}
	err := DB.Create(m).Error
	if err == nil {
		DB.Model(&Conversation{}).Where("id = ?", convID).
			Update("updated_at", time.Now())
	}
	return m, err
}

// SaveRegeneratedMessage 保存重新生成的 assistant 消息，归入已有 group
func SaveRegeneratedMessage(convID, content, thinking, groupID string, genIndex int) (*Message, error) {
	m := &Message{
		ID:                NewID(),
		ConversationID:    convID,
		Role:              "assistant",
		Content:           content,
		Thinking:          thinking,
		GenerationGroupID: groupID,
		GenIndex:          genIndex,
	}
	err := DB.Create(m).Error
	if err == nil {
		DB.Model(&Conversation{}).Where("id = ?", convID).
			Update("updated_at", time.Now())
	}
	return m, err
}

// GetMessages 返回所有消息（含重生成历史版本），前端自行分组展示
func GetMessages(convID string) ([]Message, error) {
	var msgs []Message
	err := DB.Where("conversation_id = ?", convID).
		Order("created_at ASC").Find(&msgs).Error
	return msgs, err
}

// GetLatestMessages 返回对话消息，每个 generation group 只取最新版（gen_index 最大）
// 用于构建 einoMsgs 历史上下文，不把旧版本带入
func GetLatestMessages(convID string) ([]Message, error) {
	all, err := GetMessages(convID)
	if err != nil {
		return nil, err
	}
	// 按 group 去重：保留每组 gen_index 最大的一条
	type groupKey = string
	latest := make(map[groupKey]*Message)
	order := []groupKey{}
	for i := range all {
		m := &all[i]
		gid := m.GenerationGroupID
		if gid == "" {
			gid = m.ID // 旧数据兼容
		}
		if prev, ok := latest[gid]; !ok {
			latest[gid] = m
			order = append(order, gid)
		} else if m.GenIndex > prev.GenIndex {
			latest[gid] = m
		}
	}
	result := make([]Message, 0, len(order))
	for _, gid := range order {
		result = append(result, *latest[gid])
	}
	return result, nil
}

// UpdateConversationWorkDir 更新 task 模式工作目录，不影响 updated_at。
func UpdateConversationWorkDir(id, workDir string) error {
	return DB.Model(&Conversation{}).Where("id = ?", id).
		UpdateColumn("work_dir", workDir).Error
}

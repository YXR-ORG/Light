package storage

import (
	"fmt"
	"strings"
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

// SaveTaskMessage 保存 task 模式消息，自动标记 mode=task
func SaveTaskMessage(convID, role, content string) (*Message, error) {
	return SaveTaskMessageWithArtifacts(convID, role, content, "")
}

// SaveTaskMessageWithArtifacts 保存 task 消息，并附带产物 JSON（[]Artifact）。
func SaveTaskMessageWithArtifacts(convID, role, content, artifacts string) (*Message, error) {
	id := NewID()
	m := &Message{
		ID:                id,
		ConversationID:    convID,
		Role:              role,
		Content:           content,
		Artifacts:         artifacts,
		Mode:              "task",
		GenerationGroupID: id,
		GenIndex:          0,
	}
	err := DB.Create(m).Error
	if err == nil {
		DB.Model(&Conversation{}).Where("id = ?", convID).
			Update("updated_at", time.Now())
	}
	return m, err
}

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

// SaveTaskMessageWithAttachments 保存 task 模式用户消息，附带附件 meta JSON。
func SaveTaskMessageWithAttachments(convID, role, content, attachments string) (*Message, error) {
	id := NewID()
	m := &Message{
		ID:                id,
		ConversationID:    convID,
		Role:              role,
		Content:           content,
		Attachments:       attachments,
		Mode:              "task",
		GenerationGroupID: id,
		GenIndex:          0,
	}
	err := DB.Create(m).Error
	if err == nil {
		DB.Model(&Conversation{}).Where("id = ?", convID).
			Update("updated_at", time.Now())
	}
	return m, err
}

// ForkConversation 从 srcConvID 的 forkFromMsgID 处分叉创建新会话。
// 复制 forkFromMsgID 所在 group（含）之前的所有消息（仅最新版本），
// 新会话继承 provider/model/system_prompt/agent_id/mcp_server_ids/mode/knowledge_base_id。
// task 模式不支持分叉（调用方负责校验）。
func ForkConversation(srcConvID, forkFromMsgID string) (*Conversation, error) {
	var src Conversation
	if err := DB.First(&src, "id = ?", srcConvID).Error; err != nil {
		return nil, fmt.Errorf("source conversation not found: %w", err)
	}
	if src.Mode == "task" {
		return nil, fmt.Errorf("task 模式不支持分叉")
	}

	// 拉取源会话全部消息（按 created_at 升序）
	var allMsgs []Message
	if err := DB.Where("conversation_id = ?", srcConvID).
		Order("created_at ASC").Find(&allMsgs).Error; err != nil {
		return nil, fmt.Errorf("load source messages: %w", err)
	}

	// 定位 forkFromMsgID 所在的 GenerationGroupID
	var forkGroupID string
	forkFound := false
	for _, m := range allMsgs {
		if m.ID == forkFromMsgID {
			forkGroupID = m.GenerationGroupID
			if forkGroupID == "" {
				forkGroupID = m.ID
			}
			forkFound = true
			break
		}
	}
	if !forkFound {
		return nil, fmt.Errorf("fork message not found in conversation")
	}

	// 找到该 group 在消息序列中的最后位置（按 created_at）
	// group 内可能有多条（重生成版本），取 group 最后一条的位置作为分叉边界
	forkEndIdx := -1
	for i, m := range allMsgs {
		gid := m.GenerationGroupID
		if gid == "" {
			gid = m.ID
		}
		if gid == forkGroupID {
			forkEndIdx = i // 持续更新到该 group 的最后一条
		}
	}

	// 收集要复制的消息：到 forkEndIdx 为止，但 group 内只取最新版本
	// （与 GetLatestMessages 相同的 group 去重逻辑，但仅限 forkEndIdx 之前）
	type groupKey = string
	latest := make(map[groupKey]*Message)
	order := []groupKey{}
	for i := 0; i <= forkEndIdx; i++ {
		m := &allMsgs[i]
		gid := m.GenerationGroupID
		if gid == "" {
			gid = m.ID
		}
		if prev, ok := latest[gid]; !ok {
			latest[gid] = m
			order = append(order, gid)
		} else if m.GenIndex > prev.GenIndex {
			latest[gid] = m
		}
	}

	// 计算新标题：原标题 + " (分叉)"，已含则加序号
	newTitle := src.Title
	if !strings.Contains(newTitle, "(分叉)") {
		newTitle = newTitle + " (分叉)"
	} else {
		// 已有 (分叉) 后缀，追加序号
		base := strings.TrimSuffix(newTitle, " (分叉)")
		for n := 2; ; n++ {
			candidate := fmt.Sprintf("%s (分叉 %d)", base, n)
			var count int64
			DB.Model(&Conversation{}).Where("title = ?", candidate).Count(&count)
			if count == 0 {
				newTitle = candidate
				break
			}
		}
	}

	newID := NewID()
	newConv := &Conversation{
		ID:              newID,
		Title:           newTitle,
		Provider:        src.Provider,
		Model:           src.Model,
		SystemPrompt:    src.SystemPrompt,
		AgentID:         src.AgentID,
		MCPServerIDs:    src.MCPServerIDs,
		Mode:            src.Mode,
		KnowledgeBaseID: src.KnowledgeBaseID,
		ParentConvID:    srcConvID,
		ForkFromMsgID:   forkFromMsgID,
	}

	// 单事务：创建会话 + 复制消息
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(newConv).Error; err != nil {
			return fmt.Errorf("create forked conversation: %w", err)
		}
		for _, gid := range order {
			src := latest[gid]
			msgID := NewID()
			newMsg := Message{
				ID:                msgID,
				ConversationID:    newID,
				Role:              src.Role,
				Content:           src.Content,
				Thinking:          src.Thinking,
				ToolCalls:         src.ToolCalls,
				ToolResult:        src.ToolResult,
				Attachments:       src.Attachments,
				Artifacts:         src.Artifacts,
				AgentID:           src.AgentID,
				MCPServerIDs:      src.MCPServerIDs,
				Mode:              src.Mode,
				KnowledgeBaseID:   src.KnowledgeBaseID,
				GenerationGroupID: msgID, // 重置为自身，新会话从干净状态开始
				GenIndex:          0,
			}
			if err := tx.Create(&newMsg).Error; err != nil {
				return fmt.Errorf("copy message: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return newConv, nil
}

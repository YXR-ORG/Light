package handler

import (
	"fmt"
	"light-ai/internal/storage"
)

type ConversationHandler struct{}

func NewConversationHandler() *ConversationHandler {
	return &ConversationHandler{}
}

func (h *ConversationHandler) Create(provider, model string) (*storage.Conversation, error) {
	return storage.CreateConversation(provider, model)
}

func (h *ConversationHandler) List() ([]storage.Conversation, error) {
	return storage.ListConversations()
}

func (h *ConversationHandler) Get(id string) (*storage.Conversation, error) {
	return storage.GetConversation(id)
}

func (h *ConversationHandler) Rename(id, title string) error {
	return storage.UpdateConversationTitle(id, title)
}

func (h *ConversationHandler) Search(query string) ([]storage.Conversation, error) {
	if query == "" {
		return storage.ListConversations()
	}
	return storage.SearchConversations(query)
}

func (h *ConversationHandler) Delete(id string) error {
	return storage.DeleteConversation(id)
}

func (h *ConversationHandler) SetSystemPrompt(id, prompt string) error {
	return storage.UpdateSystemPrompt(id, prompt)
}

// SetAgent 设置对话的智能体（更新 system_prompt + agent_id）
func (h *ConversationHandler) SetAgent(convID, agentID string) error {
	agent, err := storage.GetAgent(agentID)
	if err != nil {
		return fmt.Errorf("agent not found: %w", err)
	}
	return storage.SetAgent(convID, agentID, agent.SystemPrompt)
}

func (h *ConversationHandler) SetModel(id, provider, model string) error {
	return storage.UpdateConversationModel(id, provider, model)
}

// SetMode 保存对话的输入模式和知识库选择
func (h *ConversationHandler) SetMode(id, mode, knowledgeBaseID string) error {
	return storage.UpdateConversationMode(id, mode, knowledgeBaseID)
}

// UpdateConversationWorkDir 更新对话的任务工作目录
func (h *ConversationHandler) UpdateConversationWorkDir(id, workDir string) error {
	return storage.UpdateConversationWorkDir(id, workDir)
}

// SetGoal 设定 task 模式的目标、验收标准和轮数上限。
// goal 为空时清除目标；acceptanceCriteria 为换行分隔的验收标准；maxTurns 为 0 表示用默认值。
func (h *ConversationHandler) SetGoal(id, goal, acceptanceCriteria string, maxTurns int) error {
	return storage.UpdateConversationGoal(id, goal, acceptanceCriteria, maxTurns)
}

func (h *ConversationHandler) GetMessages(convID string) ([]storage.Message, error) {
	return storage.GetMessages(convID)
}

// ToggleFavorite 切换收藏状态，返回新状态
func (h *ConversationHandler) ToggleFavorite(id string) (bool, error) {
	return storage.ToggleFavorite(id)
}

// ListFavorites 返回所有收藏对话
func (h *ConversationHandler) ListFavorites() ([]storage.Conversation, error) {
	return storage.ListFavorites()
}

// Fork 从指定消息处分叉创建新会话。task 模式不支持分叉。
// 复制到 forkFromMsgID 所在 group（含）为止的消息（仅最新版本），
// 新会话继承源会话的模型、智能体、模式等配置。
func (h *ConversationHandler) Fork(convID, msgID string) (*storage.Conversation, error) {
	conv, err := storage.GetConversation(convID)
	if err != nil {
		return nil, fmt.Errorf("conversation not found: %w", err)
	}
	if conv.Mode == "task" {
		return nil, fmt.Errorf("task 模式不支持分叉")
	}
	return storage.ForkConversation(convID, msgID)
}

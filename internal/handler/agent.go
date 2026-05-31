package handler

import "light-ai/internal/storage"

type AgentHandler struct{}

func NewAgentHandler() *AgentHandler { return &AgentHandler{} }

func (h *AgentHandler) ListAgents() ([]storage.Agent, error) {
	return storage.ListAgents()
}

func (h *AgentHandler) SaveAgent(a storage.Agent) (storage.Agent, error) {
	if err := storage.SaveAgent(&a); err != nil {
		return storage.Agent{}, err
	}
	return a, nil
}

func (h *AgentHandler) DeleteAgent(id string) error {
	return storage.DeleteAgent(id)
}

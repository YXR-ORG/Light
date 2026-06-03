package eino

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/cloudwego/eino-ext/components/model/claude"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type ChatService struct {
	mu       sync.RWMutex
	provider string
	model    string
	apiKey   string
	baseURL  string
	llm      model.ChatModel
	tools    []tool.BaseTool
}

func NewChatService() *ChatService {
	return &ChatService{}
}

func (s *ChatService) Configure(provider, modelName, apiKey, baseURL string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.provider == provider && s.model == modelName && s.apiKey == apiKey && s.baseURL == baseURL && s.llm != nil {
		return nil
	}

	s.provider = provider
	s.model = modelName
	s.apiKey = apiKey
	s.baseURL = baseURL
	// Reset tools when model changes
	s.tools = nil

	var err error
	switch provider {
	case "openai":
		s.llm, err = openai.NewChatModel(context.Background(), &openai.ChatModelConfig{
			Model:   modelName,
			APIKey:  apiKey,
			BaseURL: baseURL,
		})
	case "claude":
		claudeCfg := &claude.Config{
			Model:  modelName,
			APIKey: apiKey,
		}
		if baseURL != "" {
			claudeCfg.BaseURL = &baseURL
		}
		s.llm, err = claude.NewChatModel(context.Background(), claudeCfg)
	case "ollama":
		if baseURL == "" {
			baseURL = "http://localhost:11434"
		}
		if apiKey == "" {
			apiKey = "ollama"
		}
		s.llm, err = openai.NewChatModel(context.Background(), &openai.ChatModelConfig{
			Model:   modelName,
			APIKey:  apiKey,
			BaseURL: fmt.Sprintf("%s/v1", baseURL),
		})
	case "deepseek":
		if baseURL == "" {
			baseURL = "https://api.deepseek.com/v1"
		}
		s.llm, err = openai.NewChatModel(context.Background(), &openai.ChatModelConfig{
			Model:   modelName,
			APIKey:  apiKey,
			BaseURL: baseURL,
		})
	case "gemini":
		if baseURL == "" {
			baseURL = "https://generativelanguage.googleapis.com/v1beta/openai/"
		}
		s.llm, err = openai.NewChatModel(context.Background(), &openai.ChatModelConfig{
			Model:   modelName,
			APIKey:  apiKey,
			BaseURL: baseURL,
		})
	case "qwen":
		if baseURL == "" {
			baseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
		}
		s.llm, err = openai.NewChatModel(context.Background(), &openai.ChatModelConfig{
			Model:   modelName,
			APIKey:  apiKey,
			BaseURL: baseURL,
		})
	case "ark":
		if baseURL == "" {
			baseURL = "https://ark.cn-beijing.volces.com/api/v3"
		}
		s.llm, err = openai.NewChatModel(context.Background(), &openai.ChatModelConfig{
			Model:   modelName,
			APIKey:  apiKey,
			BaseURL: baseURL,
		})
	default:
		err = fmt.Errorf("不支持的供应商: %s", provider)
	}
	if err != nil {
		slog.Error("创建对话模型失败", "provider", provider, "error", err)
	}
	return err
}

// BindTools binds tools to the LLM for tool calling.
func (s *ChatService) BindTools(ctx context.Context, tools []tool.BaseTool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.llm == nil {
		return fmt.Errorf("对话模型未配置")
	}

	infos := make([]*schema.ToolInfo, 0, len(tools))
	for _, t := range tools {
		info, err := t.Info(ctx)
		if err != nil {
			slog.Warn("获取工具信息失败", "error", err)
			continue
		}
		infos = append(infos, info)
	}

	if err := s.llm.BindTools(infos); err != nil {
		return fmt.Errorf("绑定工具失败: %w", err)
	}
	s.tools = tools
	return nil
}

// RunTool executes a named tool with JSON arguments.
func (s *ChatService) RunTool(ctx context.Context, name, argsJSON string) (string, error) {
	s.mu.RLock()
	tools := s.tools
	s.mu.RUnlock()

	for _, t := range tools {
		info, err := t.Info(ctx)
		if err != nil {
			continue
		}
		if info.Name != name {
			continue
		}
		invokable, ok := t.(tool.InvokableTool)
		if !ok {
			return "", fmt.Errorf("工具 %s 不支持调用", name)
		}
		return invokable.InvokableRun(ctx, argsJSON)
	}
	return "", fmt.Errorf("未找到工具: %s", name)
}

func (s *ChatService) Chat(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
	s.mu.RLock()
	llm := s.llm
	s.mu.RUnlock()

	if llm == nil {
		return nil, fmt.Errorf("对话模型未配置")
	}
	result, err := llm.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("生成失败: %w", err)
	}
	return result, nil
}

func (s *ChatService) Stream(ctx context.Context, messages []*schema.Message) (*schema.StreamReader[*schema.Message], error) {
	s.mu.RLock()
	llm := s.llm
	s.mu.RUnlock()

	if llm == nil {
		return nil, fmt.Errorf("对话模型未配置")
	}
	return llm.Stream(ctx, messages)
}

// GetToolCallingModel 返回底层 ToolCallingChatModel，供 task 模式的 ReAct Agent 使用。
// 返回 nil 表示模型未配置或不支持 tool calling。
func (s *ChatService) GetToolCallingModel() model.ToolCallingChatModel {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.llm == nil {
		return nil
	}
	tcm, ok := s.llm.(model.ToolCallingChatModel)
	if !ok {
		return nil
	}
	return tcm
}

// Generate 非流式调用，用于摘要生成等后台任务
func (s *ChatService) Generate(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
	s.mu.RLock()
	llm := s.llm
	s.mu.RUnlock()

	if llm == nil {
		return nil, fmt.Errorf("对话模型未配置")
	}
	return llm.Generate(ctx, messages)
}

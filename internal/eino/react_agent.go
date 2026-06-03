package eino

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
	ub "github.com/cloudwego/eino/utils/callbacks"
)

// TaskStep 是 task 模式推理链中的一个步骤，对应前端 task:step event。
type TaskStep struct {
	ConvID     string `json:"conv_id"`     // 会话 ID，前端用于过滤
	Type       string `json:"type"`        // thinking|tool_call|tool_result|bash_confirm|bash_output|content|done|error
	Content    string `json:"content"`     // LLM 思考/回答片段
	ToolName   string `json:"tool_name"`   // tool_call / tool_result
	ToolArgs   string `json:"tool_args"`   // tool_call：JSON args
	ToolResult string `json:"tool_result"` // tool_result：执行结果
	ConfirmID  string `json:"confirm_id"`  // bash_confirm：唯一 ID
	Cmd        string `json:"cmd"`         // bash_confirm / bash_output
	Error      string `json:"error"`       // error
}

// taskSystemPrompt 构建 task 模式的 system prompt。
func taskSystemPrompt(workDir string) string {
	return fmt.Sprintf(`你是一个自主任务执行智能体。

工作目录：%s
当前时间：%s

你可以使用以下资源：
- bash_exec：执行 shell 命令（危险命令会请求用户确认）
- read_file / write_file / list_dir / make_dir：文件操作（仅限工作目录内）
- 知识库检索、技能、网络搜索、MCP 工具（已在工具列表中）

执行原则：
1. 分析任务，制定步骤，逐步执行
2. 优先使用现有工具，不要重复造轮子
3. 文件操作限制在工作目录内
4. bash 命令执行前思考是否必要，优先用文件工具替代简单操作
5. 任务完成后给出简洁的执行摘要（完成了什么、产生了哪些文件）
6. 如果任务无法完成，说明原因和建议`, workDir, time.Now().Format("2006-01-02 15:04"))
}

// RunTaskAgent 启动 eino ReAct Agent，返回 TaskStep channel（缓冲 64）。
// ctx cancel → agent 停止，channel close。
// tools 必须全部实现 tool.BaseTool（InvokableTool）。
// bashTool 引用用于 Confirm 回调注入。
func RunTaskAgent(
	ctx context.Context,
	llm model.ToolCallingChatModel,
	tools []tool.BaseTool,
	bashTool *BashTool,
	workDir string,
	history []*schema.Message,
	userMsg string,
) (<-chan TaskStep, error) {
	if llm == nil {
		return nil, fmt.Errorf("task agent: LLM 未配置")
	}

	ch := make(chan TaskStep, 64)

	// emitter：BashTool 推送 bash_confirm / bash_output 到 channel
	emitter := func(stepType, content, cmd, confirmID string) {
		select {
		case ch <- TaskStep{Type: stepType, Content: content, Cmd: cmd, ConfirmID: confirmID}:
		default:
			slog.Warn("TaskAgent: step channel full, dropping", "type", stepType)
		}
	}
	if bashTool != nil {
		bashTool.emitter = emitter
	}

	// 构建 MessageModifier：注入 system prompt
	sysPrompt := taskSystemPrompt(workDir)
	modifier := react.NewPersonaModifier(sysPrompt)

	// 构建 Callback：监听 model stream → 推送 thinking/content steps
	//                监听 tool start/end → 推送 tool_call/tool_result steps
	modelHandler := &ub.ModelCallbackHandler{}
	toolHandler := &ub.ToolCallbackHandler{
		OnStart: func(ctx context.Context, info *callbacks.RunInfo, input *tool.CallbackInput) context.Context {
			args := ""
			if input != nil {
				args = input.ArgumentsInJSON
			}
			name := ""
			if info != nil {
				name = info.Name
			}
			select {
			case ch <- TaskStep{Type: "tool_call", ToolName: name, ToolArgs: args}:
			default:
			}
			return ctx
		},
		OnEnd: func(ctx context.Context, info *callbacks.RunInfo, output *tool.CallbackOutput) context.Context {
			result := ""
			name := ""
			if output != nil {
				result = output.Response
			}
			if info != nil {
				name = info.Name
			}
			select {
			case ch <- TaskStep{Type: "tool_result", ToolName: name, ToolResult: result}:
			default:
			}
			return ctx
		},
	}

	cb := react.BuildAgentCallback(modelHandler, toolHandler)

	// 创建 ReAct agent
	agentInst, err := react.NewAgent(ctx, &react.AgentConfig{
		ToolCallingModel: llm,
		ToolsConfig: compose.ToolsNodeConfig{
			Tools: tools,
		},
		MessageModifier: modifier,
		MaxStep:         30,
	})
	if err != nil {
		return nil, fmt.Errorf("task agent: 创建 ReAct agent 失败: %w", err)
	}

	// 构建消息列表
	msgs := make([]*schema.Message, 0, len(history)+1)
	msgs = append(msgs, history...)
	msgs = append(msgs, &schema.Message{
		Role:    schema.User,
		Content: userMsg,
	})

	go func() {
		defer close(ch)
		defer func() {
			if r := recover(); r != nil {
				slog.Error("TaskAgent panic", "recover", r)
				select {
				case ch <- TaskStep{Type: "error", Error: fmt.Sprintf("agent 内部错误: %v", r)}:
				default:
				}
			}
		}()

		stream, err := agentInst.Stream(ctx, msgs,
			agent.WithComposeOptions(compose.WithCallbacks(cb)))
		if err != nil {
			slog.Error("TaskAgent stream error", "error", err)
			ch <- TaskStep{Type: "error", Error: err.Error()}
			return
		}
		defer stream.Close()

		for {
			msg, err := stream.Recv()
			if err != nil {
				break // io.EOF or ctx cancel
			}
			if msg == nil {
				continue
			}
			if msg.ReasoningContent != "" {
				ch <- TaskStep{Type: "thinking", Content: msg.ReasoningContent}
			}
			if msg.Content != "" {
				ch <- TaskStep{Type: "content", Content: sanitizeContent(msg.Content)}
			}
		}
		ch <- TaskStep{Type: "done"}
	}()

	return ch, nil
}

// sanitizeContent 清理 LLM 输出中的尾部空字节。
func sanitizeContent(s string) string {
	return strings.TrimRight(s, "\x00")
}

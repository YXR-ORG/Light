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
	return fmt.Sprintf(`你是 Light 的任务执行智能体。你的工作是**动手执行**，不是**口头描述**。

工作目录：%s
当前时间：%s

## 核心铁律（违反任何一条都算失败）

### 1. 绝对禁止"口头计划"
以下行为**严格禁止**：
- ❌ "让我先看看目录结构" / "我先查一下" / "我来分析一下" → 这些都是废话，直接调工具
- ❌ 在 content 中描述你将要做什么 → 你没有"将要"的选项，只有"正在做"
- ❌ 把工具调用计划写成文字 → 直接发送 tool_call

### 2. 必须调用工具，不能只说不做
- 需要查看文件？→ 调 read_file
- 需要查看目录？→ 调 list_dir
- 需要创建文件？→ 调 write_file
- 需要搜索？→ 调对应搜索工具
- 需要执行命令？→ 调 bash_exec
- **任何文件操作和信息获取都必须通过工具**

### 3. 你的第一个响应必须是工具调用
- 收到任务第一轮就应调用工具获取信息
- 不要先输出"好的，我来帮你..."然后停在那里
- 每个需要信息的步骤，都必须有对应的工具调用

### 4. 工具返回的是参考资料，不是最终答案
- **绝对禁止**把工具返回的原始内容当作你的回答输出
- 工具结果是你的"草稿纸"和"参考资料"，你需要消化吸收后再输出
- 你的最终回答必须是你自己的话，不是复制粘贴工具返回的文本
- 如果工具返回了完整的文档/手册/技能说明，请提炼其中关键信息，用你自己的话总结
- **技能文档≠最终输出**：技能告诉你"怎么做"，你要"做出结果"

### 5. 输出规范
- 创建的文件内容用 write_file 写入，不要在回答中贴出全文
- 最终总结：简短有力，告诉用户你做了什么、产出了什么
- 如果创建了多个文件，列出文件名和简要说明
- 回答长度控制在合理范围（过长的工具文档不要原样照抄）

## 可用工具
所有已配置的工具都在你的工具列表中，包括：
- bash_exec：执行 shell 命令（危险命令需用户确认）
- read_file / write_file / list_dir / make_dir：文件系统操作（限工作目录内）
- 知识库检索、技能、网络搜索、MCP 工具

## 工作流
1. 收到任务 → 直接调工具（不要先说话）
2. 工具返回结果 → 根据结果决定下一步工具
3. 信息充分后 → 输出简明总结

记住：**你不是一个只会说话的助手，你是一个能动手的智能体。用工具证明你的能力。**
`, workDir, time.Now().Format("2006-01-02 15:04"))
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

	ch := make(chan TaskStep, 512)

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
	modelHandler := &ub.ModelCallbackHandler{
		OnEnd: func(ctx context.Context, info *callbacks.RunInfo, output *model.CallbackOutput) context.Context {
			if output != nil && output.Message != nil {
				tcCount := len(output.Message.ToolCalls)
				contentLen := len(output.Message.Content)
				reasonLen := len(output.Message.ReasoningContent)
				slog.Info("TaskAgent model round end", "tool_calls", tcCount, "content_len", contentLen, "reasoning_len", reasonLen)
				if tcCount == 0 && contentLen > 0 {
					slog.Warn("TaskAgent model produced content without tool calls - agent may stop", "content_preview", contentLen)
				}
			}
			return ctx
		},
	}
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
			slog.Info("TaskAgent tool_call", "tool", name, "args_len", len(args))
			ch <- TaskStep{Type: "tool_call", ToolName: name, ToolArgs: args}
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
			slog.Info("TaskAgent tool_result", "tool", name, "result_len", len(result))
			ch <- TaskStep{Type: "tool_result", ToolName: name, ToolResult: result}
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

		// 使用 WithMessageFuture 获取每轮 LLM 的流式输出
		futureOpt, future := react.WithMessageFuture()

		// 启动 agent stream
		outputStream, err := agentInst.Stream(ctx, msgs,
			agent.WithComposeOptions(compose.WithCallbacks(cb)),
			futureOpt,
		)
		if err != nil {
			slog.Error("TaskAgent stream error", "error", err)
			ch <- TaskStep{Type: "error", Error: err.Error()}
			return
		}

		// 并发：goroutine 1 — 从 iterator 读 thinking delta（每轮 LLM 输出）
		iterDone := make(chan struct{})
		go func() {
			defer close(iterDone)
			streams := future.GetMessageStreams()
			for {
				sr, ok, err := streams.Next()
				if err != nil || !ok {
					break
				}
				for {
					msg, err := sr.Recv()
					if err != nil {
						break
					}
					if msg == nil {
						continue
					}
				// debug: dump raw tool_calls to diagnose deepseek-v4 format compatibility
				if len(msg.ToolCalls) > 0 {
					for i, tc := range msg.ToolCalls {
						slog.Info("TaskAgent raw tool_call in msg", "round_idx", i, "tool_id", tc.ID, "tool_name", tc.Function.Name, "args_len", len(tc.Function.Arguments))
					}
				}
				if msg.ReasoningContent != "" {
					ch <- TaskStep{Type: "thinking", Content: msg.ReasoningContent}
				}
				if msg.Content != "" {
					ch <- TaskStep{Type: "content", Content: sanitizeContent(msg.Content)}
				}
				}
				sr.Close()
			}
		}()

		// goroutine 2（主流程）— 消费 output stream，推进 agent 执行
		for {
			_, err := outputStream.Recv()
			if err != nil {
				break
			}
		}
		outputStream.Close()

		// 等 iterator goroutine 完成
		<-iterDone
		ch <- TaskStep{Type: "done"}
	}()

	return ch, nil
}

// sanitizeContent 清理 LLM 输出中的尾部空字节。
func sanitizeContent(s string) string {
	return strings.TrimRight(s, "\x00")
}

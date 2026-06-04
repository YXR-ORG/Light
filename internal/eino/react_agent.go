package eino

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
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
	Type       string `json:"type"`        // thinking|tool_call|tool_result|bash_confirm|bash_output|content|content_note|content_rollback|notice|done|error
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

### 5. 效率约束
- 同类搜索/查询最多 2-3 次，不要反复搜索同一个主题
- 信息到位后立即开始产出，不要无限研究
- 如果工具返回的内容足够完成任务，马上输出结果
- 最终总结：简短有力，告诉用户你做了什么、产出了什么
- 回答长度控制在合理范围（过长的工具文档不要原样照抄）

### 6. 默认不写文件（重要）
- **默认情况下，直接在回答中给出结果**，不要调用 write_file
- 只有当用户**明确要求**"保存到文件 / 写入文件 / 生成XX文件 / 导出 / 创建文件"等，才调用 write_file
- 用户说"帮我写一段代码""给我一个方案""分析一下" → 这些都是要你**在对话里回答**，不是写文件
- 用户说"把结果保存到 result.md""生成一个 config.json" → 这才调用 write_file
- 拿不准时，优先在回答中直接输出，而不是落地成文件


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

	// 内部可取消 ctx：用于死循环检测时主动中断 agent
	runCtx, runCancel := context.WithCancel(ctx)

	// 死循环检测：记录最近的工具调用签名（tool name + args），
	// 连续 loopDetectThreshold 次完全相同 → 判定原地打转，主动 cancel 止损。
	const loopDetectThreshold = 6
	var loopMu sync.Mutex
	lastSig := ""
	repeatCount := 0
	loopDetected := false

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

			// 死循环检测：同一工具+参数连续重复
			sig := name + "|" + args
			loopMu.Lock()
			if sig == lastSig {
				repeatCount++
			} else {
				lastSig = sig
				repeatCount = 1
			}
			triggered := repeatCount >= loopDetectThreshold && !loopDetected
			if triggered {
				loopDetected = true
			}
			loopMu.Unlock()

			if triggered {
				slog.Warn("TaskAgent loop detected, cancelling", "tool", name, "repeat", repeatCount)
				runCancel()
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
			slog.Info("TaskAgent tool_result", "tool", name, "result_len", len(result))
			ch <- TaskStep{Type: "tool_result", ToolName: name, ToolResult: result}
			return ctx
		},
	}

	cb := react.BuildAgentCallback(modelHandler, toolHandler)

	// 创建 ReAct agent。MaxStep=100 仅为天花板（模型输出无 tool_call 即自然终止），
	// 不强制跑满。配合死循环检测提前止损，避免弱模型浪费配额跑满 100 步。
	agentInst, err := react.NewAgent(runCtx, &react.AgentConfig{
		ToolCallingModel: llm,
		ToolsConfig: compose.ToolsNodeConfig{
			Tools: tools,
		},
		MessageModifier: modifier,
		MaxStep:         100,
	})
	if err != nil {
		runCancel()
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
		defer runCancel()
		defer func() {
			if r := recover(); r != nil {
				slog.Error("TaskAgent panic", "recover", r)
				select {
				case ch <- TaskStep{Type: "error", Error: fmt.Sprintf("agent 内部错误: %v", r)}:
				default:
				}
			}
		}()

		// 使用 WithMessageFuture 获取每轮 LLM 的流式输出。
		// 该消息流同时包含 assistant 消息（含 tool_call）与 tool 执行结果，
		// 我们顺序收集为 collectedMsgs，供撞限后的“补总结轮”复用上下文。
		futureOpt, future := react.WithMessageFuture()

		// 启动 agent stream
		outputStream, err := agentInst.Stream(runCtx, msgs,
			agent.WithComposeOptions(compose.WithCallbacks(cb)),
			futureOpt,
		)
		if err != nil {
			slog.Error("TaskAgent stream error", "error", err)
			ch <- TaskStep{Type: "error", Error: err.Error()}
			return
		}

		// hasFinalContent：是否已产出“最终答案”（不含 tool_call 轮次的 content）
		var hasFinalContent bool
		// collectedMsgs：累积的完整对话消息（assistant + tool），用于补总结轮
		var collectedMsgs []*schema.Message
		var collectMu sync.Mutex

		// 并发：goroutine 1 — 从 iterator 读每轮 LLM 输出
		// 关键：一轮 LLM 输出可能同时含 content 和 tool_call。
		//   - 含 tool_call 的轮次 → content 是“过程旁白/工具复述”，归入折叠链（content_note）
		//   - 不含 tool_call 的轮次（通常是最后一轮）→ content 才是“最终答案”，归入正文（content）
		iterDone := make(chan struct{})
		go func() {
			defer close(iterDone)
			streams := future.GetMessageStreams()
			for {
				sr, ok, err := streams.Next()
				if err != nil || !ok {
					break
				}
				var roundContent strings.Builder
				hasToolCall := false
				isToolMsg := false
				contentStreamed := false // 本轮是否已实时推送过 content delta
				var toolCalls []schema.ToolCall
				var toolMsgID, toolName string
				for {
					msg, err := sr.Recv()
					if err != nil {
						break
					}
					if msg == nil {
						continue
					}
					if len(msg.ToolCalls) > 0 {
						hasToolCall = true
						toolCalls = append(toolCalls, msg.ToolCalls...)
					}
					// tool 结果消息（WithMessageFuture 会把 tool result 也发进流）。
					// 其 Content 是工具执行结果，绝不能进正文——它已通过 ToolCallbackHandler
					// 以 tool_result step 展示在折叠链。这里仅记录用于补总结轮的上下文。
					if msg.Role == schema.Tool {
						isToolMsg = true
						toolMsgID = msg.ToolCallID
						toolName = msg.ToolName
						if msg.Content != "" {
							roundContent.WriteString(msg.Content)
						}
						continue
					}
					if msg.ReasoningContent != "" {
						ch <- TaskStep{Type: "thinking", Content: msg.ReasoningContent}
					}
					// assistant content：实时流式推送（逐 chunk）。
					// 本轮结束后若发现 hasToolCall，再发 content_rollback 把这些 delta
					// 从正文移入折叠链（说明是“过程旁白”而非最终答案）。
					if msg.Content != "" {
						roundContent.WriteString(msg.Content)
						contentStreamed = true
						ch <- TaskStep{Type: "content", Content: sanitizeContent(msg.Content)}
					}
				}
				sr.Close()

				text := sanitizeContent(roundContent.String())

				// 收集消息用于补总结轮：区分 assistant 与 tool
				collectMu.Lock()
				if isToolMsg {
					// tool 结果消息：仅入收集列表，不进正文/旁白
					collectedMsgs = append(collectedMsgs, &schema.Message{
						Role:       schema.Tool,
						Content:    text,
						ToolCallID: toolMsgID,
						ToolName:   toolName,
					})
					collectMu.Unlock()
					continue
				}
				// assistant 消息（可能含 tool_call）
				collectedMsgs = append(collectedMsgs, &schema.Message{
					Role:      schema.Assistant,
					Content:   text,
					ToolCalls: toolCalls,
				})
				if !hasToolCall && text != "" {
					hasFinalContent = true
				}
				collectMu.Unlock()

				// 本轮结束：若含 tool_call，说明刚才实时推送的 content 其实是“过程旁白”，
				// 发 content_rollback 让前端把本轮 delta 从正文撤回并改入折叠链。
				if hasToolCall && contentStreamed && text != "" {
					slog.Info("TaskAgent content_rollback (旁白)", "len", len(text))
					ch <- TaskStep{Type: "content_rollback", Content: text}
				}
			}
		}()

		// goroutine 2（主流程）— 消费 output stream，推进 agent 执行。
		// 关键：记录真实结束原因。io.EOF = agent 正常结束（模型已给最终答案）；
		// 其他 error（exceeds max steps / context canceled）= 异常结束，才需补总结轮。
		var finalErr error
		for {
			_, err := outputStream.Recv()
			if err != nil {
				finalErr = err
				break
			}
		}
		outputStream.Close()

		// 等 iterator goroutine 完成
		<-iterDone

		// 判断 agent 是否正常结束（EOF 视为正常）
		normalEnd := finalErr == nil || errors.Is(finalErr, io.EOF)
		isMaxSteps := finalErr != nil && strings.Contains(finalErr.Error(), "exceeds max steps")
		slog.Info("TaskAgent run ended", "normal", normalEnd, "max_steps", isMaxSteps, "err", finalErr)

		// 判断结束原因
		collectMu.Lock()
		produced := hasFinalContent
		msgsForSummary := make([]*schema.Message, len(collectedMsgs))
		copy(msgsForSummary, collectedMsgs)
		collectMu.Unlock()

		loopMu.Lock()
		wasLoop := loopDetected
		loopMu.Unlock()

		// 情况 1：agent 正常结束且已产出正文 → 直接完成（最常见路径）
		if normalEnd && produced && !wasLoop {
			ch <- TaskStep{Type: "done"}
			return
		}

		// 情况 2：agent 正常结束但未产出正文（流式时序导致 future 未推出最终答案）
		// → 静默补总结，不发"撞限"提示（因为并非异常）
		if normalEnd && !wasLoop {
			summary := runSummaryRound(ctx, llm, sysPrompt, userMsg, msgsForSummary, ch)
			if !summary && !produced {
				ch <- TaskStep{Type: "error", Error: "模型未返回有效回答，请重试"}
			}
			ch <- TaskStep{Type: "done"}
			return
		}

		// 情况 3：异常结束（撞 MaxStep / 死循环）→ 补总结 + 提示用户
		var notice string
		if wasLoop {
			notice = "⚠️ 检测到重复操作，已自动停止。以下是基于已收集信息的总结："
		} else {
			notice = "⚠️ 任务较复杂，已达执行步数上限。以下是基于已收集信息的总结："
		}

		summary := runSummaryRound(ctx, llm, sysPrompt, userMsg, msgsForSummary, ch)
		if summary || produced {
			ch <- TaskStep{Type: "notice", Content: notice}
			ch <- TaskStep{Type: "done"}
		} else {
			ch <- TaskStep{Type: "error", Error: "任务未能完成，且无法生成总结，请重试或拆分任务"}
		}
	}()

	return ch, nil
}

// runSummaryRound 在 agent 撞限/死循环后，用累积上下文发起一次“禁用工具”的总结请求，
// 让模型基于已收集信息直接产出最终答案。结果以 content step 流式推送。
// 返回是否成功产出非空总结。
func runSummaryRound(
	ctx context.Context,
	llm model.ToolCallingChatModel,
	sysPrompt, userMsg string,
	collected []*schema.Message,
	ch chan<- TaskStep,
) bool {
	// 构建总结请求：system + 原始任务 + 已收集的过程消息 + 总结指令
	msgs := make([]*schema.Message, 0, len(collected)+3)
	msgs = append(msgs, schema.SystemMessage(sysPrompt))
	msgs = append(msgs, &schema.Message{Role: schema.User, Content: userMsg})
	// 仅保留有内容的 assistant 消息作为上下文（去掉 tool_call 配对，避免 API 校验问题）
	for _, m := range collected {
		if m == nil {
			continue
		}
		if m.Role == schema.Assistant && m.Content != "" {
			msgs = append(msgs, schema.AssistantMessage(m.Content, nil))
		} else if m.Role == schema.Tool && m.Content != "" {
			// 把 tool 结果转成 assistant 旁白形式，避免 tool_call 配对校验
			msgs = append(msgs, schema.AssistantMessage("【工具结果】"+truncateRunes(m.Content, 2000), nil))
		}
	}
	msgs = append(msgs, &schema.Message{
		Role:    schema.User,
		Content: "请基于以上已收集的全部信息，直接给出最终答案或总结。不要再调用任何工具，用你自己的话组织成完整、清晰的回答。",
	})

	// 不绑定工具，直接 Stream
	sr, err := llm.Stream(ctx, msgs)
	if err != nil {
		slog.Warn("TaskAgent summary round failed", "error", err)
		return false
	}
	defer sr.Close()

	produced := false
	for {
		chunk, err := sr.Recv()
		if err != nil {
			break
		}
		if chunk == nil {
			continue
		}
		if chunk.ReasoningContent != "" {
			ch <- TaskStep{Type: "thinking", Content: chunk.ReasoningContent}
		}
		if chunk.Content != "" {
			produced = true
			ch <- TaskStep{Type: "content", Content: sanitizeContent(chunk.Content)}
		}
	}
	return produced
}

// truncateRunes 按 rune 截断字符串。
func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

// sanitizeContent 清理 LLM 输出中的尾部空字节。
func sanitizeContent(s string) string {
	return strings.TrimRight(s, "\x00")
}

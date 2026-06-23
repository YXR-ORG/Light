package eino

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

// TaskStep 是 task 模式推理链中的一个步骤，对应前端 task:step event。
type TaskStep struct {
	ConvID          string `json:"conv_id"`          // 会话 ID，前端用于过滤
	Type            string `json:"type"`             // thinking|tool_call|tool_result|bash_confirm|bash_output|content|content_note|content_rollback|notice|done|error
	Content         string `json:"content"`          // LLM 思考/回答片段
	ToolName        string `json:"tool_name"`        // tool_call / tool_result
	ToolArgs        string `json:"tool_args"`        // tool_call：JSON args
	ToolResult      string `json:"tool_result"`      // tool_result：执行结果
	ConfirmID       string `json:"confirm_id"`       // bash_confirm：唯一 ID
	Cmd             string `json:"cmd"`              // bash_confirm / bash_output
	Error           string `json:"error"`            // error
	AttachmentsMeta string `json:"attachments_meta"` // user_msg：附件 meta JSON
}

const taskContentRollbackMaxLen = 1200

func shouldRollbackTaskContent(content string, hasToolCall bool) bool {
	return hasToolCall && content != "" && len(content) <= taskContentRollbackMaxLen
}

// taskSystemPrompt 构建 task 模式的 system prompt。
// planEnabled 为 true 时注入 plan 指令（复杂任务先列计划）。
func taskSystemPrompt(workDir string, planEnabled bool) string {
	planHeader := ""
	planSection := ""
	planWorkflow := "1. 收到任务 → 直接调工具（不要先说话）"
	if planEnabled {
		planHeader = `
⚠️ **计划模式已开启：收到多步骤任务时，你的第一个工具调用必须是 update_plan，列出所有执行步骤。**
`
		planSection = `
### 7. 计划模式（强制要求）
- **收到任务后，第一步必须调用 update_plan**，列出完整的执行计划（步骤 >= 2 时）
- 每完成一步，立即再次调用 update_plan，把该步改为 done，下一步改为 in_progress
- 步骤描述要具体：不写"搜索信息"，要写"搜索XXX的YYY"；不写"处理数据"，要写"读取file.csv并筛选ZZZ字段"
- **只有真正的单步骤任务（一个工具调用就能完成）才跳过 update_plan**
`
		planWorkflow = "1. 收到任务 → **先调 update_plan 列出计划** → 再执行第一步工具"
	}
	return fmt.Sprintf(`你是 Light 的任务执行智能体。你的工作是**动手执行**，不是**口头描述**。
%s
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

### 6. 默认不操作本地文件（重要）
- **默认情况下，直接在回答中给出结果**，不要主动调用 read_file / write_file / list_dir
- 只有当用户**明确要求**操作文件，或**执行步骤本身需要**（如修复 bug、写代码到指定路径），才调用文件工具
- 用户说"帮我写一段代码""给我一个方案""分析一下" → 在对话里直接回答，不读也不写文件
- 用户说"把结果保存到 result.md" / "查看 config.json" → 这才调用 write_file / read_file
- **用户上传了附件**：附件内容已在消息里，直接使用，不要再调 read_file 读同名文件
- 拿不准时，优先在回答中直接输出，而不是主动触碰工作目录
%s

## 可用工具
所有已配置的工具都在你的工具列表中，包括：
- bash_exec：执行 shell 命令（危险命令需用户确认）
- read_file / write_file / list_dir / make_dir：文件系统操作（限工作目录内）
- 知识库检索、技能、网络搜索、MCP 工具

## 工作流
%s
2. 工具返回结果 → 根据结果决定下一步工具
3. 信息充分后 → 输出简明总结

记住：**你不是一个只会说话的助手，你是一个能动手的智能体。用工具证明你的能力。**
`, planHeader, workDir, time.Now().Format("2006-01-02 15:04"), planSection, planWorkflow)
}

func appendTaskPlanInstruction(msg *schema.Message) {
	const instruction = "\n\n[系统要求：这是多步骤任务，你的第一个工具调用必须是 update_plan，列出完整执行计划。]"
	if len(msg.UserInputMultiContent) > 0 {
		msg.UserInputMultiContent = append(msg.UserInputMultiContent, schema.MessageInputPart{
			Type: schema.ChatMessagePartTypeText,
			Text: instruction,
		})
		return
	}
	msg.Content += instruction
}

// RunTaskAgent 启动 eino adk ChatModelAgent（ReAct），返回 TaskStep channel（缓冲 512）。
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
	userInput *schema.Message,
	planEnabled bool,
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

	// System prompt 作为 agent Instruction（替代旧版 react.NewPersonaModifier）
	sysPrompt := taskSystemPrompt(workDir, planEnabled)

	// 构造压缩中间件链（reduction + summarization）
	// 两层防线：先机械裁剪大工具输出（零 LLM 成本），再 LLM 语义压缩长对话
	compressionHandlers := BuildCompressionMiddlewares(runCtx, llm, workDir)
	if len(compressionHandlers) > 0 {
		slog.Info("TaskAgent: compression middleware enabled", "count", len(compressionHandlers), "workDir", workDir)
	}

	// 创建 adk ChatModelAgent（ReAct）
	agent, err := adk.NewChatModelAgent(runCtx, &adk.ChatModelAgentConfig{
		Name:          "LightTaskAgent",
		Instruction:   sysPrompt,
		Model:         llm,
		ToolsConfig:   adk.ToolsConfig{ToolsNodeConfig: compose.ToolsNodeConfig{Tools: tools}},
		MaxIterations: 100,
		Handlers:      compressionHandlers, // 压缩中间件（可能为 nil，表示降级不压缩）
	})
	if err != nil {
		runCancel()
		return nil, fmt.Errorf("task agent: 创建 ChatModelAgent 失败: %w", err)
	}

	// 构建消息列表
	msgs := make([]*schema.Message, 0, len(history)+1)
	msgs = append(msgs, history...)
	if userInput == nil {
		userInput = &schema.Message{Role: schema.User, Content: userMsg}
	}
	if planEnabled {
		appendTaskPlanInstruction(userInput)
	}
	msgs = append(msgs, userInput)

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

		// 启动 agent（返回事件迭代器）
		iter := agent.Run(runCtx, &adk.AgentInput{
			Messages:        msgs,
			EnableStreaming: true,
		})

		// hasFinalContent：是否已产出"最终答案"（不含 tool_call 轮次的 content）
		var hasFinalContent bool
		// collectedMsgs：累积的完整对话消息（assistant + tool），用于补总结轮
		var collectedMsgs []*schema.Message
		var collectMu sync.Mutex

		// 单事件循环：逐轮处理模型输出（Assistant）和工具结果（Tool）
		var finalErr error
		for {
			event, ok := iter.Next()
			if !ok {
				break
			}

			// 错误事件
			if event.Err != nil {
				finalErr = event.Err
				slog.Error("TaskAgent event error", "error", event.Err)
				break
			}

			if event.Output == nil || event.Output.MessageOutput == nil {
				continue
			}

			mv := event.Output.MessageOutput

			switch mv.Role {
			case schema.Assistant:
				// —— Model output（一轮 LLM 输出）——
				var roundContent strings.Builder
				hasToolCall := false
				contentStreamed := false
				var toolCalls []schema.ToolCall

				slog.Info("TaskAgent model round start")

				if mv.IsStreaming && mv.MessageStream != nil {
					for {
						chunk, err := mv.MessageStream.Recv()
						if err != nil {
							break
						}
						if chunk == nil {
							continue
						}
						if len(chunk.ToolCalls) > 0 {
							hasToolCall = true
							toolCalls = append(toolCalls, chunk.ToolCalls...)
						}
						if chunk.ReasoningContent != "" {
							ch <- TaskStep{Type: "thinking", Content: chunk.ReasoningContent}
						}
						if chunk.Content != "" {
							roundContent.WriteString(chunk.Content)
							contentStreamed = true
							ch <- TaskStep{Type: "content", Content: sanitizeContent(chunk.Content)}
						}
					}
					// 流式 stream 消费完后无需单独 Close（框架负责）
				} else if mv.Message != nil {
					// 非流式输出（兜底）
					roundContent.WriteString(mv.Message.Content)
					if mv.Message.ReasoningContent != "" {
						ch <- TaskStep{Type: "thinking", Content: mv.Message.ReasoningContent}
					}
					if len(mv.Message.ToolCalls) > 0 {
						hasToolCall = true
						toolCalls = mv.Message.ToolCalls
					}
					if mv.Message.Content != "" {
						contentStreamed = true
						ch <- TaskStep{Type: "content", Content: sanitizeContent(mv.Message.Content)}
					}
				}

				text := sanitizeContent(roundContent.String())
				tcCount := len(toolCalls)
				slog.Info("TaskAgent model round end", "tool_calls", tcCount, "content_len", len(text), "reasoning_len", "...")

				// —— 发送 tool_call steps + 死循环检测 ——
				for _, tc := range toolCalls {
					ch <- TaskStep{Type: "tool_call", ToolName: tc.Function.Name, ToolArgs: tc.Function.Arguments}

					sig := tc.Function.Name + "|" + tc.Function.Arguments
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
						slog.Warn("TaskAgent loop detected, cancelling", "tool", tc.Function.Name, "repeat", repeatCount)
						runCancel()
					}
				}

				// —— 收集 assistant 消息用于补总结轮 ——
				collectMu.Lock()
				collectedMsgs = append(collectedMsgs, &schema.Message{
					Role:      schema.Assistant,
					Content:   text,
					ToolCalls: toolCalls,
				})
				if text != "" && !shouldRollbackTaskContent(text, hasToolCall) {
					hasFinalContent = true
				}
				collectMu.Unlock()

				// —— content_rollback：短 content + tool_call = 旁白 ——
				if contentStreamed && shouldRollbackTaskContent(text, hasToolCall) {
					slog.Info("TaskAgent content_rollback (旁白)", "len", len(text))
					ch <- TaskStep{Type: "content_rollback", Content: text}
				}

			case schema.Tool:
				// —— Tool result ——
				name := mv.ToolName
				result := ""
				toolMsgID := ""
				if mv.Message != nil {
					result = mv.Message.Content
					toolMsgID = mv.Message.ToolCallID
				}
				slog.Info("TaskAgent tool_result", "tool", name, "result_len", len(result))
				ch <- TaskStep{Type: "tool_result", ToolName: name, ToolResult: result}

				// 收集 tool 结果消息用于补总结轮
				collectMu.Lock()
				collectedMsgs = append(collectedMsgs, &schema.Message{
					Role:       schema.Tool,
					Content:    result,
					ToolCallID: toolMsgID,
					ToolName:   name,
				})
				collectMu.Unlock()
			}
		}

		// —— Agent 结束，判断结束原因 ——
		normalEnd := finalErr == nil
		isMaxSteps := finalErr != nil && strings.Contains(finalErr.Error(), "exceeds max iterations")
		slog.Info("TaskAgent run ended", "normal", normalEnd, "max_steps", isMaxSteps, "err", finalErr)

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

		// 情况 2：agent 正常结束但未产出正文（流式时序可能导致最终答案未正确收集）
		// → 静默补总结，不发"撞限"提示（因为并非异常）
		if normalEnd && !wasLoop {
			summary := runSummaryRound(ctx, llm, sysPrompt, userMsg, msgsForSummary, ch)
			if !summary && !produced {
				ch <- TaskStep{Type: "error", Error: "模型未返回有效回答，请重试"}
			}
			ch <- TaskStep{Type: "done"}
			return
		}

		// 情况 3：异常结束（撞 MaxStep / 死循环 / 其他错误）→ 补总结 + 提示用户
		var notice string
		if wasLoop {
			notice = "⚠️ 检测到重复操作，已自动停止。以下是基于已收集信息的总结："
		} else if isMaxSteps {
			notice = "⚠️ 任务较复杂，已达执行步数上限。以下是基于已收集信息的总结："
		} else if finalErr != nil {
			// 其他错误（如 context canceled 但非 loop 触发）
			slog.Info("TaskAgent: non-loop error, attempting summary", "err", finalErr)
			notice = "⚠️ 任务执行中断。以下是基于已收集信息的总结："
		} else {
			notice = "⚠️ 任务未能完成。以下是基于已收集信息的总结："
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

// runSummaryRound 在 agent 撞限/死循环后，用累积上下文发起一次"禁用工具"的总结请求，
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

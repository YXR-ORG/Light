package eino

import (
	"context"
	"encoding/json"
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
	ConvID          string     `json:"conv_id"`          // 会话 ID，前端用于过滤
	Type            string     `json:"type"`             // thinking|tool_call|tool_result|bash_confirm|bash_output|content|content_note|content_rollback|notice|done|error|usage
	Content         string     `json:"content"`          // LLM 思考/回答片段
	ToolName        string     `json:"tool_name"`        // tool_call / tool_result
	ToolArgs        string     `json:"tool_args"`        // tool_call：JSON args
	ToolResult      string     `json:"tool_result"`      // tool_result：执行结果
	ConfirmID       string     `json:"confirm_id"`       // bash_confirm：唯一 ID
	Cmd             string     `json:"cmd"`              // bash_confirm / bash_output
	Error           string     `json:"error"`            // error
	AttachmentsMeta string     `json:"attachments_meta"` // user_msg：附件 meta JSON
	Usage           *StepUsage `json:"usage,omitempty"`  // usage 步骤 / done 时的累计 token
}

// StepUsage 是推给前端的 token 用量快照。
type StepUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
	CumulativeTokens int `json:"cumulative_tokens"` // 本轮任务累计
}

const taskContentRollbackMaxLen = 1200

func shouldRollbackTaskContent(content string, hasToolCall bool) bool {
	return hasToolCall && content != "" && len(content) <= taskContentRollbackMaxLen
}

// defaultMaxEvalTurns 是验收打回的默认轮数上限（MaxTurns=0 时使用）。
const defaultMaxEvalTurns = 5

// parseCriteriaLines 把换行分隔的验收标准拆成非空行列表。
func parseCriteriaLines(raw string) []string {
	var out []string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// effectiveMaxTurns 返回有效的验收轮数上限。
func effectiveMaxTurns(maxTurns int) int {
	if maxTurns <= 0 {
		return defaultMaxEvalTurns
	}
	return maxTurns
}

// taskSystemPrompt 构建 task 模式的 system prompt。
// planEnabled 为 true 时注入 plan 指令（复杂任务先列计划）。
// goal 非空时注入 goal 模式指令（持续执行直到目标达成，调用 finish_goal 结束）。
// workflow 非空时注入工作流模式指令（5 阶段结构化流程），替代 goal 指令。
// criteria 非空时在 goal/workflow 指令后追加验收标准说明。
func taskSystemPrompt(workDir string, planEnabled bool, goal, workflow string, criteria []string) string {
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

	// 工作流模式注入（优先于 goal，二者互斥）
	modeSection := ""
	criteriaSection := ""
	if len(criteria) > 0 {
		var b strings.Builder
		for i, c := range criteria {
			fmt.Fprintf(&b, "  %d. %s\n", i+1, c)
		}
		criteriaSection = fmt.Sprintf(`
**用户定义的验收标准（硬性指标，全部满足后才能 finish_goal）：**
%s- 调用 finish_goal 后，系统会用独立裁判对照以上标准验收；不通过会被打回继续
`, b.String())
	}
	if workflow != "" {
		modeSection = fmt.Sprintf(`
### %d. 结构化工作流模式（已开启）
你正在执行一个结构化开发工作流，用户需求如下：
> %s
%s
**必须按以下 5 个阶段顺序执行，不可跳过任何阶段：**

1. **【需求分析】** 你的第一个工具调用必须是 **submit_spec**，明确需求描述和验收标准
   - 需求描述：用户想要什么、解决什么问题
   - 验收标准：可检查的具体条件，每条应能明确判断是否满足（至少 2 条）

2. **【方案设计】** submit_spec 之后，直接输出技术设计方案
   - 架构思路、技术选型、关键实现路径
   - 用你自己的话组织，不要调工具——这是你的设计思考

3. **【执行计划】** 调用 **update_plan** 列出编码步骤
   - 步骤要具体可执行，覆盖设计方案中的所有实现点

4. **【编码实现】** 使用 bash_exec / write_file 等工具逐步实现
   - 每完成一步，更新 update_plan 状态
   - 普通命令无需用户确认即可执行，但极危险命令仍需确认

5. **【完成确认】** 所有验收标准满足后，调用 **finish_goal** 结束任务
   - summary 中总结你做了什么、达成了哪些验收标准
   - 系统会自动发起独立审查 + 验收，对照标准逐项检查

**关键约束：**
- 验收标准是你的硬性指标，必须全部达成
- 不要在验收标准未满足时提前调用 finish_goal
- 遇到障碍尝试不同方案自行解决，不要停下来问用户
`, planSectionNum(planEnabled), workflow, criteriaSection)
	} else if goal != "" {
		modeSection = fmt.Sprintf(`
### %d. 目标驱动模式（已开启）
- 你正在为一个明确的目标工作，请持续执行直到目标**完全达成**：
  > %s
%s- **当且仅当**你确认目标已完全达成，调用 **finish_goal** 工具结束任务，并在 summary 中总结成果
- 不要在目标未达成时提前停下来问用户——遇到障碍尝试不同方案自行解决
- 目标确实无法达成时（遇到不可解决的障碍），调用 finish_goal 并将 success 设为 false，说明原因
- 普通命令无需用户确认即可执行，但极危险命令仍需确认
`, planSectionNum(planEnabled), goal, criteriaSection)
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
- 【同义词归一化】若 search_knowledge 返回中带有 synonym_mappings 字段（如"孙彤→孙小仙"），表示用户用词与文档标准词是同一实体。此时即使文档原文未出现用户用词，检索到的内容即为该实体的信息，应直接据此回答，不得以"原文未出现该词"为由拒绝作答或声称找不到。

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
%s%s

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
`, planHeader, workDir, time.Now().Format("2006-01-02 15:04"), planSection, modeSection, planWorkflow)
}

// planSectionNum 返回 plan 节在 system prompt 中的编号（plan 开启时为 7，goal 节紧随其后）。
func planSectionNum(planEnabled bool) int {
	if planEnabled {
		return 8
	}
	return 7
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
// goal 非空时启用 goal 模式（持续执行直到 finish_goal 调用）。
// workflow 非空时启用工作流模式（5 阶段结构化流程，内含 goal 逻辑）。
// acceptanceCriteria 非空时启用独立验收（换行分隔）；maxTurns 为验收打回上限（0=默认 5）。
// finishGoalTool 引用用于检测 agent 是否调用了 finish_goal（可为 nil）。
// specTool 引用用于工作流模式的需求规格追踪（可为 nil）。
//
// 验收打回采用方案 B：finish_goal 后若验收/审查不通过，带着历史+反馈重新启动一轮 Run。
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
	goal, workflow string,
	acceptanceCriteria string,
	maxTurns int,
	finishGoalTool *FinishGoalTool,
	specTool *SpecTool,
) (<-chan TaskStep, error) {
	if llm == nil {
		return nil, fmt.Errorf("task agent: LLM 未配置")
	}

	ch := make(chan TaskStep, 512)
	criteria := parseCriteriaLines(acceptanceCriteria)
	evalLimit := effectiveMaxTurns(maxTurns)

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

	// System prompt 作为 agent Instruction
	sysPrompt := taskSystemPrompt(workDir, planEnabled, goal, workflow, criteria)

	// 初始消息列表
	baseMsgs := make([]*schema.Message, 0, len(history)+1)
	baseMsgs = append(baseMsgs, history...)
	if userInput == nil {
		userInput = &schema.Message{Role: schema.User, Content: userMsg}
	}
	if planEnabled || specTool != nil {
		appendTaskPlanInstruction(userInput)
	}
	baseMsgs = append(baseMsgs, userInput)

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

		// 跨轮次累积的消息（用于补总结 / 验收打回后的下一轮）
		var allCollected []*schema.Message
		// 跨轮次收集 write_file 变更路径
		changedFiles := map[string]bool{}
		// 验收轮次计数（每次 finish_goal 算一轮）
		evalTurn := 0
		// 当前轮次输入
		currentMsgs := baseMsgs
		// 是否已产出过最终正文
		hasFinalContent := false
		// 累计 token（主 agent + 裁判/审查）
		var totalPrompt, totalCompletion, totalTokens int
		emitUsage := func(prompt, completion, total int) {
			if prompt == 0 && completion == 0 && total == 0 {
				return
			}
			if total == 0 {
				total = prompt + completion
			}
			totalPrompt += prompt
			totalCompletion += completion
			totalTokens += total
			ch <- TaskStep{
				Type: "usage",
				Usage: &StepUsage{
					PromptTokens:     prompt,
					CompletionTokens: completion,
					TotalTokens:      total,
					CumulativeTokens: totalTokens,
				},
			}
		}
		emitDone := func() {
			var usage *StepUsage
			if totalTokens > 0 || totalPrompt > 0 || totalCompletion > 0 {
				usage = &StepUsage{
					PromptTokens:     totalPrompt,
					CompletionTokens: totalCompletion,
					TotalTokens:      totalTokens,
					CumulativeTokens: totalTokens,
				}
			}
			ch <- TaskStep{Type: "done", Usage: usage}
		}

		for {
			if ctx.Err() != nil {
				ch <- TaskStep{Type: "error", Error: "任务已取消"}
				return
			}

			// 每轮独立 cancel：死循环/finish_goal 时中断本轮 agent
			runCtx, runCancel := context.WithCancel(ctx)

			// 死循环检测
			const loopDetectThreshold = 6
			var loopMu sync.Mutex
			lastSig := ""
			repeatCount := 0
			loopDetected := false

			// 压缩中间件（每轮重建，绑定本轮 runCtx）
			compressionHandlers := BuildCompressionMiddlewares(runCtx, llm, workDir)
			if len(compressionHandlers) > 0 {
				slog.Info("TaskAgent: compression middleware enabled", "count", len(compressionHandlers), "workDir", workDir)
			}

			agent, err := adk.NewChatModelAgent(runCtx, &adk.ChatModelAgentConfig{
				Name:          "LightTaskAgent",
				Instruction:   sysPrompt,
				Model:         llm,
				ToolsConfig:   adk.ToolsConfig{ToolsNodeConfig: compose.ToolsNodeConfig{Tools: tools}},
				MaxIterations: 100,
				Handlers:      compressionHandlers,
			})
			if err != nil {
				runCancel()
				ch <- TaskStep{Type: "error", Error: fmt.Sprintf("创建 ChatModelAgent 失败: %v", err)}
				return
			}

			// 本轮产出
			var roundCollected []*schema.Message
			var collectMu sync.Mutex
			goalFinished := false
			var finalErr error
			roundProduced := false

			iter := agent.Run(runCtx, &adk.AgentInput{
				Messages:        currentMsgs,
				EnableStreaming: true,
			})

			for {
				event, ok := iter.Next()
				if !ok {
					break
				}
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
					var roundContent strings.Builder
					hasToolCall := false
					contentStreamed := false
					var toolCalls []schema.ToolCall

					slog.Info("TaskAgent model round start")

					var roundPrompt, roundCompletion, roundTotal int
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
							// 流式 usage 通常在最后一块；取最大值避免重复累加同一轮
							if chunk.ResponseMeta != nil && chunk.ResponseMeta.Usage != nil {
								u := chunk.ResponseMeta.Usage
								if u.PromptTokens > roundPrompt {
									roundPrompt = u.PromptTokens
								}
								if u.CompletionTokens > roundCompletion {
									roundCompletion = u.CompletionTokens
								}
								if u.TotalTokens > roundTotal {
									roundTotal = u.TotalTokens
								}
							}
						}
					} else if mv.Message != nil {
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
						if mv.Message.ResponseMeta != nil && mv.Message.ResponseMeta.Usage != nil {
							u := mv.Message.ResponseMeta.Usage
							roundPrompt = u.PromptTokens
							roundCompletion = u.CompletionTokens
							roundTotal = u.TotalTokens
						}
					}
					emitUsage(roundPrompt, roundCompletion, roundTotal)

					text := sanitizeContent(roundContent.String())
					slog.Info("TaskAgent model round end", "tool_calls", len(toolCalls), "content_len", len(text),
						"tokens", roundTotal)

					for _, tc := range toolCalls {
						ch <- TaskStep{Type: "tool_call", ToolName: tc.Function.Name, ToolArgs: tc.Function.Arguments}

						// 收集 write_file 变更路径，供独立审查使用
						if tc.Function.Name == "write_file" {
							if p := extractWriteFilePath(tc.Function.Arguments); p != "" {
								changedFiles[p] = true
							}
						}

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

					collectMu.Lock()
					roundCollected = append(roundCollected, &schema.Message{
						Role:      schema.Assistant,
						Content:   text,
						ToolCalls: toolCalls,
					})
					if text != "" && !shouldRollbackTaskContent(text, hasToolCall) {
						roundProduced = true
						hasFinalContent = true
					}
					collectMu.Unlock()

					if contentStreamed && shouldRollbackTaskContent(text, hasToolCall) {
						slog.Info("TaskAgent content_rollback (旁白)", "len", len(text))
						ch <- TaskStep{Type: "content_rollback", Content: text}
					}

				case schema.Tool:
					name := mv.ToolName
					result := ""
					toolMsgID := ""
					if mv.Message != nil {
						result = mv.Message.Content
						toolMsgID = mv.Message.ToolCallID
					}
					slog.Info("TaskAgent tool_result", "tool", name, "result_len", len(result))
					ch <- TaskStep{Type: "tool_result", ToolName: name, ToolResult: result}

					if name == "finish_goal" && finishGoalTool != nil && finishGoalTool.IsFinished() {
						goalFinished = true
						slog.Info("TaskAgent: finish_goal called", "success", finishGoalTool.GetSuccess())
					}

					collectMu.Lock()
					roundCollected = append(roundCollected, &schema.Message{
						Role:       schema.Tool,
						Content:    result,
						ToolCallID: toolMsgID,
						ToolName:   name,
					})
					collectMu.Unlock()

					if goalFinished {
						runCancel()
					}
				}
			}

			runCancel() // 本轮结束，释放 cancel

			// 合并本轮消息到总累积
			collectMu.Lock()
			allCollected = append(allCollected, roundCollected...)
			collectMu.Unlock()

			normalEnd := finalErr == nil
			isMaxSteps := finalErr != nil && strings.Contains(finalErr.Error(), "exceeds max iterations")
			loopMu.Lock()
			wasLoop := loopDetected
			loopMu.Unlock()
			slog.Info("TaskAgent run ended", "normal", normalEnd, "max_steps", isMaxSteps,
				"goal_finished", goalFinished, "eval_turn", evalTurn, "err", finalErr)

			// —— 情况 0：finish_goal 被调用 ——
			if goalFinished && finishGoalTool != nil {
				// success=false：agent 主动认输，直接结束，不验收
				if !finishGoalTool.GetSuccess() {
					ch <- TaskStep{Type: "notice", Content: "⚠️ 目标未达成：\n\n" + finishGoalTool.GetSummary()}
					emitDone()
					return
				}

				evalTurn++
				feedback, passed := evaluateAndReview(
					ctx, llm, workDir, goal, workflow, criteria, evalLimit, evalTurn,
					finishGoalTool, specTool, changedFiles, allCollected, ch,
				)
				if passed {
					notice := "✅ 目标已达成！\n\n" + finishGoalTool.GetSummary()
					if workflow != "" {
						notice = "✅ 工作流已完成！\n\n" + finishGoalTool.GetSummary()
					}
					ch <- TaskStep{Type: "notice", Content: notice}
					emitDone()
					return
				}

				// 未通过且已达轮数上限 → 强制结束
				if evalTurn >= evalLimit {
					ch <- TaskStep{Type: "notice", Content: fmt.Sprintf(
						"⚠️ 已达验收轮数上限（%d 次），以下是基于已完成工作的总结：", evalLimit)}
					runSummaryRound(ctx, llm, sysPrompt, userMsg, allCollected, ch)
					emitDone()
					return
				}

				// 打回：带着历史 + 反馈，重新启动一轮
				ch <- TaskStep{Type: "notice", Content: fmt.Sprintf(
					"↩️ 验收未通过（第 %d/%d 轮），打回继续修改…", evalTurn, evalLimit)}
				finishGoalTool.Reset()
				// 构建下一轮输入：原 history + 用户任务 + 已收集过程（摘要化） + 反馈
				currentMsgs = buildRetryMessages(baseMsgs, allCollected, feedback)
				continue
			}

			// —— 情况 1：正常结束且已产出正文 ——
			if normalEnd && (roundProduced || hasFinalContent) && !wasLoop {
				emitDone()
				return
			}

			// —— 情况 2：正常结束但未产出正文 → 补总结 ——
			if normalEnd && !wasLoop {
				summary := runSummaryRound(ctx, llm, sysPrompt, userMsg, allCollected, ch)
				if !summary && !hasFinalContent {
					ch <- TaskStep{Type: "error", Error: "模型未返回有效回答，请重试"}
				}
				emitDone()
				return
			}

			// —— 情况 3：异常结束 → 补总结 + 提示 ——
			var notice string
			if wasLoop {
				notice = "⚠️ 检测到重复操作，已自动停止。以下是基于已收集信息的总结："
			} else if isMaxSteps {
				notice = "⚠️ 任务较复杂，已达执行步数上限。以下是基于已收集信息的总结："
			} else if finalErr != nil {
				slog.Info("TaskAgent: non-loop error, attempting summary", "err", finalErr)
				notice = "⚠️ 任务执行中断。以下是基于已收集信息的总结："
			} else {
				notice = "⚠️ 任务未能完成。以下是基于已收集信息的总结："
			}
			summary := runSummaryRound(ctx, llm, sysPrompt, userMsg, allCollected, ch)
			if summary || hasFinalContent {
				ch <- TaskStep{Type: "notice", Content: notice}
				emitDone()
			} else {
				ch <- TaskStep{Type: "error", Error: "任务未能完成，且无法生成总结，请重试或拆分任务"}
			}
			return
		}
	}()

	return ch, nil
}

// evaluateAndReview 在 finish_goal(success=true) 后执行验收 + 独立审查。
// 返回 (反馈文本, 是否通过)。通过时反馈为空。
//
// 流程：
//  1. workflow 模式且有 spec → 独立审查（可调只读工具）
//  2. 有用户验收标准 → 轻量裁判验收
//  3. 两者都没有 → 直接通过（保持向后兼容）
func evaluateAndReview(
	ctx context.Context,
	llm model.ToolCallingChatModel,
	workDir, goal, workflow string,
	criteria []string,
	evalLimit, evalTurn int,
	finishGoalTool *FinishGoalTool,
	specTool *SpecTool,
	changedFiles map[string]bool,
	collected []*schema.Message,
	ch chan<- TaskStep,
) (feedback string, passed bool) {
	var failReasons []string

	// 1) 工作流模式：独立审查
	if specTool != nil && specTool.HasSpec() {
		ch <- TaskStep{Type: "notice", Content: "🔍 正在进行独立验收审查…"}
		ok, issues := runIndependentReview(ctx, llm, workDir, specTool, changedFiles, ch)
		if !ok {
			failReasons = append(failReasons, issues...)
		}
	}

	// 2) 用户验收标准：轻量裁判
	if len(criteria) > 0 {
		ch <- TaskStep{Type: "notice", Content: fmt.Sprintf("⚖️ 正在验收目标（第 %d/%d 轮）…", evalTurn, evalLimit)}
		ok, issues := runEvaluatorRound(ctx, llm, goal, workflow, criteria, finishGoalTool.GetSummary(), ch)
		if !ok {
			failReasons = append(failReasons, issues...)
		}
	}

	// 两者都没有 → 直接通过
	if (specTool == nil || !specTool.HasSpec()) && len(criteria) == 0 {
		return "", true
	}

	if len(failReasons) == 0 {
		return "", true
	}

	var b strings.Builder
	b.WriteString("以下验收/审查项未通过，请继续修改后再次调用 finish_goal：\n")
	for i, r := range failReasons {
		fmt.Fprintf(&b, "%d. %s\n", i+1, r)
	}
	return b.String(), false
}

// buildRetryMessages 构造验收打回后的下一轮输入。
// 保留原始 history + 用户任务，附加过程摘要和反馈，避免消息无限膨胀。
func buildRetryMessages(baseMsgs, collected []*schema.Message, feedback string) []*schema.Message {
	msgs := make([]*schema.Message, 0, len(baseMsgs)+8)
	msgs = append(msgs, baseMsgs...)

	// 把已收集的 assistant 正文摘要化注入（最多取最近 6 条有内容的）
	var recent []string
	for i := len(collected) - 1; i >= 0 && len(recent) < 6; i-- {
		m := collected[i]
		if m == nil {
			continue
		}
		if m.Role == schema.Assistant && m.Content != "" {
			recent = append(recent, truncateRunes(m.Content, 800))
		}
	}
	// 反转为时间正序
	for i, j := 0, len(recent)-1; i < j; i, j = i+1, j-1 {
		recent[i], recent[j] = recent[j], recent[i]
	}
	if len(recent) > 0 {
		var hist strings.Builder
		hist.WriteString("【此前执行过程摘要】\n")
		for i, t := range recent {
			fmt.Fprintf(&hist, "— 步骤 %d —\n%s\n", i+1, t)
		}
		msgs = append(msgs, schema.AssistantMessage(hist.String(), nil))
	}

	msgs = append(msgs, &schema.Message{
		Role:    schema.User,
		Content: feedback + "\n请针对以上问题继续修改，完成后再次调用 finish_goal。",
	})
	return msgs
}

// extractWriteFilePath 从 write_file 工具参数 JSON 中提取 path 字段。
func extractWriteFilePath(argsJSON string) string {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ""
	}
	return strings.TrimSpace(args.Path)
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

// runEvaluatorRound 用独立 LLM 调用对照用户验收标准判断目标是否达成。
// 返回 (是否通过, fail 原因列表)。criteria 为空时直接通过。
func runEvaluatorRound(
	ctx context.Context,
	llm model.ToolCallingChatModel,
	goal, workflow string,
	criteria []string,
	agentSummary string,
	ch chan<- TaskStep,
) (passed bool, issues []string) {
	if len(criteria) == 0 {
		return true, nil
	}

	target := goal
	if workflow != "" {
		target = workflow
	}

	var criteriaText strings.Builder
	for i, c := range criteria {
		fmt.Fprintf(&criteriaText, "%d. %s\n", i+1, c)
	}

	sysPrompt := `你是目标验收裁判。你的唯一任务是对照验收标准，判断 Agent 是否真正完成了目标。
你必须客观严格——不要因为"看起来差不多"就通过。每条标准必须有明确依据。
最终只输出一个 JSON 对象，不要输出其他文字。`

	userPrompt := fmt.Sprintf(`## 目标
%s

## 验收标准
%s
## Agent 完成总结
%s

请逐条判断验收标准是否满足，严格按以下 JSON 格式输出（不要 markdown 代码块）：
{"items":[{"criterion":"标准原文","status":"pass或fail","reason":"判断依据"}],"overall":"pass或fail","issues":["未通过的原因1","未通过的原因2"]}`,
		target, criteriaText.String(), agentSummary)

	msgs := []*schema.Message{
		schema.SystemMessage(sysPrompt),
		{Role: schema.User, Content: userPrompt},
	}

	sr, err := llm.Stream(ctx, msgs)
	if err != nil {
		slog.Warn("TaskAgent evaluator round failed", "error", err)
		// 裁判失败时不阻塞，视为通过（避免因裁判故障卡住）
		ch <- TaskStep{Type: "notice", Content: "⚠️ 验收裁判调用失败，跳过验收"}
		return true, nil
	}
	defer sr.Close()

	var raw strings.Builder
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
			c := sanitizeContent(chunk.Content)
			raw.WriteString(c)
			ch <- TaskStep{Type: "content", Content: c}
		}
	}

	result := parseEvalJSON(raw.String(), criteria)
	// 推送 review artifact
	if result.artifact.Type != "" {
		ch <- TaskStep{Type: "tool_result", ToolName: "goal_evaluator", ToolResult: EmbedArtifact("目标验收完成", result.artifact)}
	}
	return result.passed, result.issues
}

// evalParseResult 是 parseEvalJSON 的返回值。
type evalParseResult struct {
	passed   bool
	issues   []string
	artifact Artifact
}

// parseEvalJSON 从裁判输出中解析 JSON 验收结果；解析失败时降级为文本匹配。
func parseEvalJSON(raw string, criteria []string) evalParseResult {
	type item struct {
		Criterion string `json:"criterion"`
		Status    string `json:"status"`
		Reason    string `json:"reason"`
	}
	type payload struct {
		Items   []item   `json:"items"`
		Overall string   `json:"overall"`
		Issues  []string `json:"issues"`
	}

	// 尝试提取 JSON 对象
	jsonStr := extractJSONObject(raw)
	var p payload
	if jsonStr != "" {
		if err := json.Unmarshal([]byte(jsonStr), &p); err != nil {
			slog.Warn("parseEvalJSON: unmarshal failed, fallback to text", "error", err)
			jsonStr = ""
		}
	}

	items := make([]AcceptItem, len(criteria))
	for i, c := range criteria {
		items[i] = AcceptItem{Content: c, Status: "pending"}
	}

	if jsonStr != "" && len(p.Items) > 0 {
		// 用 JSON 结果填充
		for i, c := range criteria {
			// 先按序号匹配，再按内容模糊匹配
			status := "pending"
			reason := ""
			if i < len(p.Items) {
				status = normalizePassFail(p.Items[i].Status)
				reason = p.Items[i].Reason
			}
			for _, it := range p.Items {
				if strings.Contains(it.Criterion, c) || strings.Contains(c, it.Criterion) {
					status = normalizePassFail(it.Status)
					reason = it.Reason
					break
				}
			}
			items[i] = AcceptItem{Content: c, Status: status, Detail: reason}
		}
	} else {
		// 降级：文本匹配
		for i, c := range criteria {
			items[i] = AcceptItem{
				Content: c,
				Status:  parseReviewStatus(raw, i, c),
			}
		}
	}

	passCount := 0
	var issues []string
	for _, it := range items {
		if it.Status == "pass" {
			passCount++
		} else {
			msg := it.Content
			if it.Detail != "" {
				msg = it.Content + "——" + it.Detail
			}
			issues = append(issues, msg)
		}
	}

	// overall 优先
	passed := passCount == len(items) && len(items) > 0
	if jsonStr != "" && p.Overall != "" {
		passed = normalizePassFail(p.Overall) == "pass"
	}
	if jsonStr != "" && len(p.Issues) > 0 && !passed {
		// 合并 JSON 里的 issues
		issues = append(issues, p.Issues...)
	}

	summary := fmt.Sprintf("验收标准 %d 条，通过 %d 条", len(items), passCount)
	if passed {
		summary += "（全部通过）"
	}

	return evalParseResult{
		passed: passed,
		issues: issues,
		artifact: Artifact{
			Type:               "review",
			Title:              "目标验收报告",
			AcceptanceCriteria: items,
			ReviewSummary:      summary,
		},
	}
}

// runIndependentReview 用全新 ChatModelAgent + 只读工具做代码审查。
// 只喂需求/验收标准/变更清单，不喂主 Agent 推理过程。
// 返回 (是否通过, fail 原因列表)。
func runIndependentReview(
	ctx context.Context,
	llm model.ToolCallingChatModel,
	workDir string,
	specTool *SpecTool,
	changedFiles map[string]bool,
	ch chan<- TaskStep,
) (passed bool, issues []string) {
	requirement := specTool.GetRequirement()
	criteria := specTool.GetAcceptanceCriteria()
	if requirement == "" || len(criteria) == 0 {
		return true, nil
	}

	var criteriaText strings.Builder
	for i, c := range criteria {
		fmt.Fprintf(&criteriaText, "%d. %s\n", i+1, c.Content)
	}

	var filesText strings.Builder
	if len(changedFiles) == 0 {
		filesText.WriteString("（Agent 未记录到 write_file 变更，请自行 list_dir / git status / git diff 排查）\n")
	} else {
		for p := range changedFiles {
			fmt.Fprintf(&filesText, "- %s\n", p)
		}
	}

	sysPrompt := fmt.Sprintf(`你是独立验收审查员。你没有参与前面的实现工作，请客观审查。

## 需求规格
%s

## 验收标准
%s
## 本次变更文件清单
%s
## 你的工作方式
1. 用工具自行核实：read_file 读改过的文件，bash_exec 跑测试 / git diff，list_dir 看目录
2. 逐条检查验收标准，判断实现是否真正满足
3. 客观严格——不要因为"看起来差不多"就 pass
4. 完成审查后，直接输出最终 JSON（不要再调工具），格式如下（不要 markdown 代码块）：
{"items":[{"criterion":"标准原文","status":"pass或fail","reason":"判断依据"}],"overall":"pass或fail","issues":["未通过原因"]}`,
		requirement, criteriaText.String(), filesText.String())

	reviewTools := BuildReviewTools(workDir)
	agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:          "LightReviewAgent",
		Instruction:   sysPrompt,
		Model:         llm,
		ToolsConfig:   adk.ToolsConfig{ToolsNodeConfig: compose.ToolsNodeConfig{Tools: reviewTools}},
		MaxIterations: 10,
	})
	if err != nil {
		slog.Warn("TaskAgent independent review agent create failed", "error", err)
		// 降级：不阻塞
		return true, nil
	}

	iter := agent.Run(ctx, &adk.AgentInput{
		Messages: []*schema.Message{
			{Role: schema.User, Content: "请审查本次实现是否满足验收标准。先用工具核实，最后输出 JSON 审查结果。"},
		},
		EnableStreaming: true,
	})

	var finalText strings.Builder
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			slog.Warn("TaskAgent independent review event error", "error", event.Err)
			break
		}
		if event.Output == nil || event.Output.MessageOutput == nil {
			continue
		}
		mv := event.Output.MessageOutput
		switch mv.Role {
		case schema.Assistant:
			var toolCalls []schema.ToolCall
			var roundContent strings.Builder
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
						toolCalls = append(toolCalls, chunk.ToolCalls...)
					}
					if chunk.ReasoningContent != "" {
						ch <- TaskStep{Type: "thinking", Content: chunk.ReasoningContent}
					}
					if chunk.Content != "" {
						c := sanitizeContent(chunk.Content)
						roundContent.WriteString(c)
						ch <- TaskStep{Type: "content", Content: c}
					}
				}
			} else if mv.Message != nil {
				if len(mv.Message.ToolCalls) > 0 {
					toolCalls = mv.Message.ToolCalls
				}
				if mv.Message.Content != "" {
					c := sanitizeContent(mv.Message.Content)
					roundContent.WriteString(c)
					ch <- TaskStep{Type: "content", Content: c}
				}
			}
			// 只有本轮没有 tool_call 的正文才计入最终 JSON（有 tool_call 的是旁白）
			if len(toolCalls) == 0 && roundContent.Len() > 0 {
				finalText.WriteString(roundContent.String())
			}
			for _, tc := range toolCalls {
				ch <- TaskStep{Type: "tool_call", ToolName: tc.Function.Name, ToolArgs: tc.Function.Arguments}
			}
		case schema.Tool:
			name := mv.ToolName
			result := ""
			if mv.Message != nil {
				result = mv.Message.Content
			}
			ch <- TaskStep{Type: "tool_result", ToolName: name, ToolResult: result}
		}
	}

	// 把 AcceptItem 的 Content 列表抽出，复用 parseEvalJSON
	critLines := make([]string, len(criteria))
	for i, c := range criteria {
		critLines[i] = c.Content
	}
	result := parseEvalJSON(finalText.String(), critLines)
	// 覆盖标题
	result.artifact.Title = "独立验收报告"
	if result.artifact.Type != "" {
		ch <- TaskStep{Type: "tool_result", ToolName: "review_acceptance", ToolResult: EmbedArtifact("独立验收审查完成", result.artifact)}
	}
	return result.passed, result.issues
}

// extractJSONObject 从文本中提取第一个完整 JSON 对象。
func extractJSONObject(s string) string {
	// 去掉可能的 markdown 代码块
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "```"); i >= 0 {
		rest := s[i+3:]
		if strings.HasPrefix(strings.ToLower(rest), "json") {
			rest = rest[4:]
		}
		if j := strings.Index(rest, "```"); j >= 0 {
			s = strings.TrimSpace(rest[:j])
		}
	}
	start := strings.Index(s, "{")
	if start < 0 {
		return ""
	}
	// 简单括号匹配
	depth := 0
	inStr := false
	escape := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if inStr {
			if escape {
				escape = false
				continue
			}
			if c == '\\' {
				escape = true
				continue
			}
			if c == '"' {
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}

func normalizePassFail(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if strings.Contains(s, "pass") || s == "ok" || s == "true" || s == "yes" {
		return "pass"
	}
	if strings.Contains(s, "fail") || s == "no" || s == "false" {
		return "fail"
	}
	return "pending"
}

// parseReviewStatus 从自由文本中判断第 idx 条验收标准的状态（降级兜底）。
func parseReviewStatus(reviewText string, idx int, criterion string) string {
	lower := strings.ToLower(reviewText)
	pos := strings.Index(lower, strings.ToLower(criterion))
	if pos < 0 {
		pos = strings.Index(lower, fmt.Sprintf("%d.", idx+1))
	}
	if pos < 0 {
		return "pending"
	}
	start := pos - 200
	if start < 0 {
		start = 0
	}
	end := pos + len(criterion) + 200
	if end > len(lower) {
		end = len(lower)
	}
	window := lower[start:end]
	if strings.Contains(window, "fail") || strings.Contains(window, "✗") || strings.Contains(window, "[x]") || strings.Contains(window, "❌") {
		return "fail"
	}
	if strings.Contains(window, "pass") || strings.Contains(window, "✓") || strings.Contains(window, "[v]") || strings.Contains(window, "✅") {
		return "pass"
	}
	return "pending"
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

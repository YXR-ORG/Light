# 三项功能规格：目标验收 / 独立审查 / 用量统计

> 版本：v1.1
> 日期：2026-07-08
> 状态：功能一、二已实现；功能三（用量统计）待实现
> 关联：
> - `docs/superpowers/specs/2026-06-24-goal-mode.md`（目标模式，本文档在此基础上增强）
> - `docs/superpowers/specs/2026-06-25-workflow-mode.md`（工作流模式，本文档改造其审查环节）

---

## 一句话先讲清楚

这份文档讲三件事：

1. **目标验收**：Agent 说"我做完了"之后，让一个独立的"裁判"来核对，而不是它自己说了算。
2. **独立审查**：工作流模式里，让一个没参与过前面工作的"新人"来审查代码，而不是写代码的人自己审。
3. **用量统计**：让用户看到每次对话烧了多少 token、花了多少钱，而不是一头雾水。

这三个功能互相独立，可以分开实现。下面逐个讲。

---

# 功能一：目标验收（Goal 评估器 + 轮数上限）

## 1. 要解决的问题

现在的目标模式是这样的：用户设一个目标，Agent 一路执行，直到它自己调用 `finish_goal` 工具说"我完成了"。

问题在于：**Agent 既是运动员又是裁判**。它经常在目标还没真正达成时就提前收工，或者反复兜圈子不知道该不该停。用户得自己检查结果，发现没做完，再催它继续——这就失去了"目标驱动、自主执行"的意义。

文章里的原话很到位：

> "当你定义了成功标准，Claude 就不用自己判断什么叫'差不多行了'然后提前结束。每次它想停下来，一个评估器会检查条件，没达到就打回去继续，直到目标达成或达到轮数上限。"

## 2. 核心思路

把"判断目标是否达成"这件事，从 Agent 自己拍脑袋，改成**一个独立裁判来核对**：

- 用户设目标时，同时写下"怎样才算完成"（验收标准）
- Agent 每次调用 `finish_goal` 想结束时，不直接结束，先把成果交给裁判
- 裁判对照验收标准判断：通过 → 真正结束；没通过 → 打回继续做
- 设一个轮数上限（比如 5 轮），达到上限强制停止，避免无限循环

这就好比：工人说"干完了"，监理来验收，合格才算完；不合格打回去返工；返工次数太多就停下来叫用户来看。

## 3. 数据模型变更

### 3.1 Conversation 表新增字段

```go
// 任务模式验收标准（仅 goal 模式用，空=不启用验收）
AcceptanceCriteria string `gorm:"type:text;default:''" json:"acceptance_criteria"`
// 轮数上限（用户设的"最多试几次"，默认 0=用系统默认值 5）
MaxTurns int `gorm:"default:0" json:"max_turns"`
```

- `AcceptanceCriteria`：JSON 数组字符串，如 `["首页 Lighthouse 分数 ≥ 90","控制台无报错"]`
- `MaxTurns`：0 表示用默认值 5；用户可自定义（建议范围 3~20）
- 两个字段都走 AutoMigrate 自动加列，旧数据无感升级

### 3.2 为什么不存到 goal 字段里

现在的 `Goal` 字段是纯文本目标描述。验收标准是结构化的多条目，塞在一起既不好解析也不好编辑。分开存，前端可以给两个独立输入框，逻辑也清晰。

## 4. 核心机制

### 4.1 验收标准输入

前端目标输入区改造：

- 原来：一个文本框，填目标
- 改后：上面目标文本框，下面"验收标准"输入区（可添加多条，每条一行）
- 每条验收标准应是**可明确判断的**，前端给占位提示，如"测试全部通过""文件存在于 xxx 路径""输出包含 yyy"

### 4.2 finish_goal 工具改造

`internal/eino/finish_goal_tool.go` 的 `finish_goal` 工具增加行为：

- Agent 调用时，照常存 `summary` + `success`
- **关键变化**：`react_agent.go` 检测到 `finish_goal` 调用后，不再立即结束，而是：
  - 若 `AcceptanceCriteria` 为空 → 维持原行为（直接结束，向后兼容）
  - 若 `AcceptanceCriteria` 非空 → 触发验收轮次

### 4.3 验收轮次

新增 `runEvaluatorRound` 函数（参照现有 `runReviewRound` 的结构）：

1. 取出 Agent 的工作成果总结（`finish_goal` 的 summary）和验收标准
2. 构造一个**独立的裁判 system prompt**，喂给一个独立的 LLM 调用（不绑定工具，单轮判断）
3. 裁判输出：逐条 pass/fail + 总体结论
4. 结果处理：
   - 全部 pass → 真正结束，发 `✅ 目标已达成（验收通过）` + 验收报告
   - 有 fail → 打回 Agent 继续，把 fail 的原因作为新的用户消息注入，提示"以下验收标准未满足，请继续：..."
   - 注意：打回不是新建 Agent，而是在同一个 Agent 上继续运行（需要调整 `runCancel` 逻辑）

### 4.4 轮数上限

`react_agent.go` 的 `MaxIterations` 现在硬编码 100。改造：

- 新增 `maxTurns` 参数传入 `RunTaskAgent`
- 目标模式有效值为：用户设的 `MaxTurns`（>0 时）或默认 5
- **注意区分两个概念**：
  - `MaxIterations`（内部迭代上限，保持 100 不变，是硬天花板，防止失控）
  - `maxTurns`（用户设的验收轮数上限，是软上限，控制"打回几次就停"）
- 达到 `maxTurns` 时：强制停止，发提示"已达轮数上限（N 次），以下是基于已完成的总结"，跑总结轮次

### 4.5 验收失败打回的循环

这是本功能最需要小心实现的地方。流程：

```
Agent 执行 → 调 finish_goal → 裁判验收
                                ├─ 通过 → 结束
                                └─ 不通过 → 把"未满足的标准"作为反馈注入 → Agent 继续
                                                                          → 再次 finish_goal → 裁判验收（第 2 轮）
                                                                          → ...直到通过或达到 maxTurns
```

技术要点：
- eino `adk.ChatModelAgent` 的 `Run` 是一次性的（返回事件迭代器，跑完即结束）。要实现"打回继续"，有两种方案：
  - **方案 A（推荐）**：验收不通过时，不结束当前 `Run`，而是往 agent 的输入里追加一条"用户反馈"消息，让它在同一个 Run 里继续。需要研究 adk 是否支持运行中追加消息——如果不支持，走方案 B。
  - **方案 B（保底）**：验收不通过时，结束当前 Run，把完整历史 + 裁判反馈作为新的 history，再起一次 `RunTaskAgent`。前端无感（都是 `task:step` 事件），但会丢失 adk 内部状态。实现简单，推荐先用这个跑通。

⚠️ 实现时需先验证 adk 的行为，再定方案。这是本功能的主要技术风险点。

## 5. 调用链

```
前端 InputArea.vue
  ├─ 设目标 + 验收标准 + 轮数上限
  │    └─ SetGoal(convID, goal, criteria, maxTurns) → 持久化
  └─ 发送任务 → StreamTask({ ..., goal, acceptanceCriteria, maxTurns })
       └─ task.go StreamTask
            └─ RunTaskAgent(ctx, llm, tools, ..., goal, workflow, acceptanceCriteria, maxTurns, finishGoalTool)
                 ├─ taskSystemPrompt 注入目标 + 验收标准说明
                 └─ 事件循环:
                      ├─ finish_goal 调用 → 触发 runEvaluatorRound
                      │    ├─ 裁判判定全部通过 → notice("✅ 验收通过") → done
                      │    ├─ 裁判判定未通过 → 注入反馈 → 继续（或起新 Run）
                      │    └─ 达到 maxTurns → notice("⚠️ 已达上限") → runSummaryRound → done
                      └─ (无验收标准时维持原逻辑)
```

## 6. 受影响文件

| 文件 | 变更 | 说明 |
|------|------|------|
| `internal/storage/models.go` | 修改 | Conversation 加 `AcceptanceCriteria`、`MaxTurns` 字段 |
| `internal/storage/conversation.go` | 修改 | `UpdateConversationGoal` 扩展为接收验收标准和轮数 |
| `internal/handler/conversation.go` | 修改 | `SetGoal` 方法扩展参数 |
| `internal/handler/task.go` | 修改 | `StreamTaskRequest` 加 `AcceptanceCriteria`、`MaxTurns` |
| `internal/eino/react_agent.go` | 修改 | 新增 `runEvaluatorRound`；`finish_goal` 检测后走验收逻辑；`maxTurns` 参数 + 上限判断 |
| `internal/eino/finish_goal_tool.go` | 可能修改 | 若需要在工具侧记录调用次数 |
| `frontend/src/components/InputArea.vue` | 修改 | 目标输入区增加验收标准多条目输入 + 轮数设置 |
| `frontend/wailsjs/go/models.ts` | 自动生成 | 新字段导出 |

## 7. 向后兼容

- `AcceptanceCriteria` 为空 → 完全维持现有行为，不触发验收
- `MaxTurns` 为 0 → 用默认值 5
- 普通任务模式（无 goal）→ 不受影响
- chat/knowledge 模式 → 不受影响

## 8. 技术约束与风险

- **adk 运行中追加消息的能力未知**：方案 A 的前提，需先验证。不通则走方案 B。
- **裁判的判断质量**：裁判也是 LLM，可能误判（该 pass 的判 fail，或该 fail 的判 pass）。缓解：裁判用与主 Agent 相同或更强的模型；验收标准尽量写成可机器验证的（如"文件存在""命令返回 0"），而非模糊的"做得好"。
- **轮数上限别设太小**：默认 5 适用于大多数任务，复杂任务建议用户设到 10+。前端给合理提示。
- **打回反馈要具体**：裁判 fail 时必须说明哪条标准没满足、缺了什么，不能只说"没通过"。否则 Agent 不知道该改什么。

---

# 功能二：独立第二代理审查（代码 Review 改造）

## 1. 要解决的问题

工作流模式现在有一个验收审查环节：Agent 写完代码调 `finish_goal` 后，会跑一轮 review。但这个 review 是**在同一个 Agent 进程里**，用同一个 LLM，喂入前面的完整对话历史，让它换个身份来审。

问题：**自己审自己，难免护短**。前面写代码时的思路、取舍、甚至偷懒，都会影响审查的客观性。文章说得很直白：

> "用第二个 Agent 做代码审查：一个拥有全新上下文的审查者偏见更少，不受主 Agent 推理过程的影响。"

现在虽然换了 system prompt（"你是验收审查员"），但喂进去的对话历史里全是主 Agent 的思路，等于让一个人写完代码再换个帽子审自己。

## 2. 核心思路

把审查环节从"同一个 Agent 换个身份"，改成**真正独立的第二次 Agent 调用**：

- 审查者只拿到：需求描述 + 验收标准 + **最终的代码改动**（diff），不拿前面的思考过程
- 用一个全新的 `ChatModelAgent` 实例跑审查
- 审查者可以调用工具自己核实（读文件、跑测试、看目录），不是只看文字描述
- 审查失败时，把问题回灌给主 Agent 修改

这就好比：代码写完后，不是让原作者自审，而是提一个 MR，让没参与过的同事来 review——他只看代码和需求，不知道你写的时候纠结过什么。

## 3. 数据模型变更

无需新增表或字段。审查结果复用现有的 `review` 类型 Artifact 结构（`ArtifactCard.vue` 已能渲染）。审查者产出的内容存入现有 Message 的 `Artifacts` 字段。

如果功能一（目标验收）也实现了，这里的审查可以和验收合并——都是"独立裁判核对成果"。区别在于：

- **目标验收**：判断"目标达没达到"，轻量，单轮 LLM 调用
- **代码审查**：判断"代码做得好不好"，重量，可调工具核实

两者可以共存：工作流模式先跑代码审查（重），再跑目标验收（轻）；普通 goal 模式只跑目标验收。

## 4. 核心机制

### 4.1 现有审查的问题（代码事实）

当前 `runReviewRound`（`react_agent.go:596-684`）的问题：

1. **喂入完整过程历史**：把 `collected`（主 Agent 的全部 assistant + tool 消息）原样塞给审查者。审查者看到了主 Agent 的每一步思考。
2. **不绑定工具**：审查者只能看文字描述判断，不能自己读文件、跑测试核实。
3. **文本解析 pass/fail**：靠字符串匹配（`parseReviewStatus` 在序号附近 200 字符窗口搜 pass/fail），脆弱。

### 4.2 改造方案

新增 `runIndependentReview` 函数，替代 `runReviewRound`：

**输入只给"结果"，不给"过程"**：
- 需求描述（从 `SpecTool` 取）
- 验收标准（从 `SpecTool` 取）
- 最终代码改动：收集本轮 Agent 所有 `write_file` 调用的文件路径和内容，整理成"本次变更清单"
- 不给：主 Agent 的 thinking、中间工具结果、推理过程

**审查者可调工具**：
- 给审查者一个独立的 `ChatModelAgent`，绑定 `read_file`、`list_dir`、`bash_exec`（只读命令，禁写）工具
- 审查者可以自己读改过的文件、跑测试、看 diff 来核实
- MaxIterations 设小一点（如 10），审查不需要太多轮

**输出结构化**：
- 审查者的 system prompt 要求输出 JSON 格式（而非自由文本），如：
  ```json
  {
    "items": [
      {"criterion": "验收标准1", "status": "pass", "reason": "..."},
      {"criterion": "验收标准2", "status": "fail", "reason": "缺少错误处理"}
    ],
    "overall": "fail",
    "issues": ["问题1", "问题2"]
  }
  ```
- 直接解析 JSON，不再靠字符串匹配，稳定可靠

### 4.3 审查失败回灌

审查 `overall = fail` 时：

- 把 `issues` 作为反馈消息，注入回主 Agent
- 主 Agent 继续修改，修完再调 `finish_goal`，再次触发审查
- 这和功能一的"验收打回"逻辑一致，可复用同一套"打回继续"机制

### 4.4 与功能一的关系

如果两个功能都实现，工作流模式的结束流程：

```
Agent 编码完成 → 调 finish_goal
  ├─ 有验收标准 + 工作流模式 → runIndependentReview（代码审查，可调工具）
  │    ├─ 通过 → runEvaluatorRound（目标验收，轻量单轮）→ 通过则结束
  │    └─ 不通过 → 回灌问题 → Agent 继续改
  └─ 只有验收标准（普通 goal）→ runEvaluatorRound（目标验收）→ ...
```

如果只实现功能二（不实现一），则审查通过即结束，不跑额外的目标验收。

## 5. 调用链

```
react_agent.go 事件循环
  └─ finish_goal 调用（workflow 模式 + success）
       ├─ 收集本次 write_file 的文件清单（变更范围）
       ├─ runIndependentReview(ctx, llm, specTool, changedFiles, ...)
       │    ├─ 构造独立 ChatModelAgent（绑定只读工具）
       │    │    └─ system prompt: 审查员身份 + 需求 + 验收标准 + 变更清单
       │    ├─ agent.Run → 审查者自己读文件/跑测试核实
       │    └─ 解析 JSON 结果 → 构建 review Artifact
       ├─ 通过 → notice("✅ 审查通过") → 推 review artifact → done
       └─ 不通过 → 注入 issues 反馈 → 主 Agent 继续
```

## 6. 受影响文件

| 文件 | 变更 | 说明 |
|------|------|------|
| `internal/eino/react_agent.go` | 修改 | 新增 `runIndependentReview`；替换 `runReviewRound` 调用点；`buildReviewArtifact` 改为解析 JSON |
| `internal/eino/tool_registry.go` | 修改 | 新增 `BuildReviewTools`（只读工具集：read_file/list_dir/bash_exec 受限） |
| `internal/eino/react_agent.go` | 修改 | 收集 `write_file` 变更清单的逻辑（可在事件循环里记录） |

前端基本不用改——review artifact 的渲染（`ArtifactCard.vue`）已经支持，只是数据来源换了。

## 7. 向后兼容

- 现有工作流模式的 review 会被新的独立审查**替代**，行为变化是预期的（就是要改得更好）
- 非工作流模式不受影响
- review artifact 结构不变，前端无感

## 8. 技术约束与风险

- **成本翻倍**：独立审查意味着多跑一个 Agent（可能 5-10 轮工具调用）。这是文章也提到的代价。缓解：审查者可用更便宜的模型（未来支持模型选择时）；MaxIterations 控小。
- **变更清单收集**：需要在事件循环里记录所有 `write_file` 的路径。现在没记，要加。也可以让审查者自己 `list_dir` + `git diff`（如果 workDir 是 git 仓库），更准确。
- **只读工具安全**：给审查者的 `bash_exec` 必须限制为只读命令（不能写、不能删），避免审查者改坏代码。可以复用现有危险命令黑名单，再叠加白名单（只允许 ls/cat/grep/test/git diff 等）。
- **JSON 输出稳定性**：LLM 可能不总是输出合法 JSON。需兜底解析（正则提取 + 失败时降级为文本匹配）。

---

# 功能三：用量统计（Token 与成本可观测性）

> ⚠️ 优先级低于功能一、二。可独立实现，不依赖前两者。

## 1. 要解决的问题

现在用户跑一个任务、聊一轮天，**完全不知道烧了多少 token**。长循环（目标模式、工作流）尤其费钱，但用户毫无感知，直到去服务商后台看账单才吓一跳。

文章专门强调可观测性的重要性：

> "`/usage` 按技能、子代理、MCP 拆分用量；`/goal` 不带参数时显示已用轮数和 token；`/workflows` 显示每个 Agent 的 token 用量，还能随时停掉某个 Agent。"

对桌面客户端来说，不需要做到 Claude Code 那么细，但至少要让用户知道：这一轮对话花了多少 token、这个会话累计花了多少、各个服务商各花了多少。

## 2. 核心思路

分三层：采集 → 存储 → 展示。

- **采集**：在流式输出时，读取每个 chunk 携带的 token 用量（eino 的 `chunk.ResponseMeta.Usage` 已有这个字段，只是现在没人读）
- **存储**：落库，按会话和按消息记录，方便汇总
- **展示**：对话界面实时显示当前会话用量；设置页给汇总统计

## 3. 数据模型变更

### 3.1 Message 表新增字段

```go
PromptTokens     int `gorm:"default:0" json:"prompt_tokens"`      // 输入 token
CompletionTokens int `gorm:"default:0" json:"completion_tokens"`  // 输出 token
TotalTokens      int `gorm:"default:0" json:"total_tokens"`       // 合计
```

每条 AI 回复消息记录其 token 消耗。走 AutoMigrate 加列。

### 3.2 新增 TokenUsage 表（可选，用于汇总统计）

```go
type TokenUsage struct {
    ID              string    `gorm:"primaryKey;size:36" json:"id"`
    ConversationID  string    `gorm:"index;size:36" json:"conversation_id"`
    MessageID       string    `gorm:"size:36" json:"message_id"`
    Provider        string    `gorm:"size:32" json:"provider"`
    Model           string    `gorm:"size:64" json:"model"`
    Mode            string    `gorm:"size:16" json:"mode"`        // chat/task/workflow
    PromptTokens    int       `json:"prompt_tokens"`
    CompletionTokens int      `json:"completion_tokens"`
    TotalTokens     int       `json:"total_tokens"`
    EstimatedCost   float64   `json:"estimated_cost"`             // 估算费用（美元）
    CreatedAt       time.Time `json:"created_at"`
}
```

为什么单独建表而不只靠 Message 字段：汇总统计（按天、按服务商、按模式）在 Message 表上做要扫全表 JOIN，独立表加索引更高效。且未来定时任务、多个子 Agent 的 token 都可以往这里记。

### 3.3 费用估算

新增 `internal/storage/pricing.go`，维护各模型的单价表（每百万 token 多少美元）：

- 内置常见模型的价格（OpenAI、Claude、DeepSeek 等）
- 用户可在设置里覆盖/补充价格
- `EstimatedCost = (PromptTokens/1M * 输入单价) + (CompletionTokens/1M * 输出单价)`
- 找不到价格时 `EstimatedCost = 0`，不阻塞统计

## 4. 核心机制

### 4.1 采集层

eino 的流式 chunk 已经携带 token 信息（`schema/message.go` 的 `ResponseMeta.Usage`，含 `PromptTokens`/`CompletionTokens`/`TotalTokens`）。现在没人读，只需在流式消费循环里加上读取。

**task 模式**（`react_agent.go` 流式循环，约 :334-355）：

```go
// 现在只读 Content 和 ReasoningContent
if chunk.Content != "" { ... }

// 新增：读取 token 用量（通常在最后一个 chunk）
if chunk.ResponseMeta != nil && chunk.ResponseMeta.Usage != nil {
    u := chunk.ResponseMeta.Usage
    promptTokens += u.PromptTokens
    completionTokens += u.CompletionTokens
}
```

**chat 模式**（`chat.go` 流式循环）：同理加读取。

**注意**：流式场景下 token 用量通常只在**最后一个 chunk** 返回，中间 chunk 的 Usage 为空。所以要累加而不是覆盖，或者只在非空时记录最后一次。

### 4.2 存储层

- task 模式：`task.go` 保存消息时（`SaveTaskMessageWithArtifacts`），把采集到的 token 写入 Message 的三个新字段，同时往 TokenUsage 表插一条
- chat 模式：`chat.go` 保存 AI 回复时同理
- 子 Agent 调用（功能二的独立审查、目标验收的裁判轮次）也记 TokenUsage，`Mode` 字段标为 `review`/`evaluator`

### 4.3 展示层

**对话内实时显示**：

- task 模式：每轮工具调用后，在推理链区域显示"本轮 N tokens"。goal 模式额外显示"第 X 轮 / 上限 Y 轮，累计 N tokens"。
- chat 模式：AI 回复消息底部小字显示"N tokens"。
- 需要通过 `task:step` 事件（新增 `usage` 类型）或 `chat:chunk` 事件把 token 推给前端。

**会话级汇总**：

- 对话头部（`ChatView.vue` 的 `.chat-header`）显示"本会话累计 X tokens / $Y"。
- 数据来源：该会话所有 Message 的 token 之和，或 TokenUsage 表聚合。

**设置页统计**：

- 设置 dialog 新增"用量统计"tab
- 按时间（今天/本周/本月）、按服务商、按模式汇总
- 简单柱状图或表格（Naive UI 有现成组件）

### 4.4 前端事件扩展

`TaskStep` 结构（`react_agent.go:19-30`）新增 usage 推送：

```go
type TaskStep struct {
    // ... 现有字段 ...
    Usage *StepUsage `json:"usage,omitempty"` // 新增
}

type StepUsage struct {
    PromptTokens     int     `json:"prompt_tokens"`
    CompletionTokens int     `json:"completion_tokens"`
    TotalTokens      int     `json:"total_tokens"`
    CumulativeTokens int     `json:"cumulative_tokens"` // 本会话累计
    Turn             int     `json:"turn"`              // 第几轮（goal 模式）
    MaxTurns         int     `json:"max_turns"`         // 上限（goal 模式）
}
```

前端 `ChatView.vue` 的 `applyStepToTaskState` 增加 `usage` 类型处理，更新会话级 token 状态。

## 5. 调用链

```
[采集]
react_agent.go / chat.go 流式循环
  └─ 读 chunk.ResponseMeta.Usage → 累加到本轮 token 计数

[存储]
task.go / chat.go 保存消息时
  ├─ Message.PromptTokens / CompletionTokens / TotalTokens 赋值
  └─ TokenUsage 表插入一条记录（含 provider/model/mode/cost）

[展示]
react_agent.go → task:step 事件（type: "usage"）→ 前端
  ├─ TaskMessageItem.vue：推理链显示本轮 token
  └─ ChatView.vue header：会话累计 token + 费用

chat.go → chat:chunk 事件（末尾带 usage）→ 前端
  └─ MessageItem.vue：消息底部小字 token

SettingsHandler.GetUsageStats() → 前端设置页
  └─ 按 天/服务商/模式 聚合 TokenUsage 表
```

## 6. 受影响文件

| 文件 | 变更 | 说明 |
|------|------|------|
| `internal/storage/models.go` | 修改 | Message 加 3 个 token 字段；新增 `TokenUsage` 表 |
| `internal/storage/conversation.go` | 修改 | `SaveMessage`/`SaveTaskMessageWithArtifacts` 写入 token；新增 token 汇总查询函数 |
| `internal/storage/pricing.go` | 新文件 | 模型单价表 + 费用估算 |
| `internal/eino/react_agent.go` | 修改 | 流式循环读 Usage；`TaskStep` 加 `Usage` 字段；推送 usage step |
| `internal/handler/chat.go` | 修改 | chat 流式读 Usage；chunk 事件带 token |
| `internal/handler/task.go` | 修改 | 保存消息时写 token + 插 TokenUsage |
| `internal/handler/settings.go` | 修改 | 新增 `GetUsageStats` 方法 |
| `frontend/src/components/TaskMessageItem.vue` | 修改 | 推理链显示本轮 token；goal 模式显示轮数 |
| `frontend/src/components/ChatView.vue` | 修改 | header 显示会话累计；处理 usage step |
| `frontend/src/components/MessageItem.vue` | 修改 | 消息底部显示 token |
| `frontend/src/components/SettingsDialog.vue` | 修改 | 新增"用量统计"tab |
| `frontend/wailsjs/go/models.ts` | 自动生成 | 新字段 + TokenUsage 类型 |

## 7. 向后兼容

- 旧消息的 token 字段为 0（AutoMigrate 默认值），不影响显示（前端判断 0 时不显示 token 小字）
- TokenUsage 表是新增的，不影响现有逻辑
- 流式读取 Usage 是纯增量——chunk 里没有 Usage 时不报错，只是不记

## 8. 技术约束与风险

- **不是所有服务商都返回 token**：有些 OpenAI 兼容接口（特别是本地 Ollama）可能不返回 Usage。需兼容空值，不报错。
- **费用估算是粗略的**：模型价格会变，用户自定义价格后才能准确。内置价格只作参考，UI 上标"估算"。
- **子 Agent 的 token 归属**：功能二的独立审查、功能一的裁判轮次，其 token 应记到同一个会话下，`Mode` 字段区分。别漏记。
- **性能**：TokenUsage 表加 `ConversationID` 索引；汇总统计走聚合查询，数据量大时考虑按天预聚合（暂不需要，个人使用数据量小）。

---

# 实现顺序建议

```
第一批（先做，互相独立）：
  1. 功能一：目标验收          ← 改动集中在 react_agent.go + finish_goal_tool，风险可控
  2. 功能二：独立审查          ← 替换 runReviewRound，独立可测

第二批（优先级低，可后做）：
  3. 功能三：用量统计          ← 横跨 chat/task 两条链路，改动面广但每处都浅
```

功能一和功能二在"验收打回"机制上有重叠（都是"裁判说不通过就回灌继续"），如果一起做，建议先抽一个共用的"打回继续"机制，两者复用。

功能三与功能一、二无依赖，任何时候都可以独立插入。但功能一、二引入的裁判轮次和独立审查 Agent 也会消耗 token，功能三的 `Mode` 字段把它们区分记录，能帮用户看清"钱花在哪了"——所以功能三晚做也有晚做的好处，能一次性覆盖所有场景。

---

# 附：三个功能的关系图

```
                        用户设目标 + 验收标准
                              │
                     ┌────────▼────────┐
                     │  主 Agent 执行   │
                     │  (task 模式)     │
                     └────────┬────────┘
                              │ 调 finish_goal
                     ┌────────▼─────────────────┐
                     │ 功能一：目标验收（裁判）   │
                     │ 独立 LLM 判断目标是否达成  │
                     └────┬──────────────┬──────┘
                     通过 │              │ 不通过
                          │              │ (回灌继续)
              ┌───────────▼───┐         │
              │ 工作流模式?    │         │
              │ 是→功能二审查  │         │
              └───────┬───────┘         │
                      │                 │
           ┌──────────▼──────────┐      │
           │ 功能二：独立审查     │      │
           │ 新 Agent + 只读工具  │──────┘
           │ 核实代码是否合格     │ (不通过也回灌)
           └──────────┬──────────┘
                      │
               全部通过 │
                      │
            ┌─────────▼──────────┐
            │ 功能三：记录本次    │
            │ 所有 Agent 的 token │
            │ (采集→存储→展示)    │
            └────────────────────┘
```

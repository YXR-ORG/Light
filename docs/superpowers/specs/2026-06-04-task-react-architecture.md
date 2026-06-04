# Task 模式 — ReAct 架构详解

> 版本：v1.0
> 日期：2026-06-04
> 状态：已实现（对应 v1.3.18+）
> 关联：`2026-06-03-task-mode-design.md`（功能设计）、本文档（机制/架构）

---

## 0. 为什么单独成章

Light 有三种对话模式，**底层执行机制完全不同**，不能混为一谈：

| 模式 | 中文 | 执行引擎 | tool 循环由谁驱动 | 终止条件 |
|------|------|----------|-------------------|----------|
| `chat` | 问答 | 手写 `runToolLoop` | 我们自己的 `for` 循环 | 模型不再返回 tool_call，或 `maxToolLoops` 上限 |
| `knowledge` | 知识库 | 手写 `runToolLoop`（强制挂 `search_knowledge`） | 同上 | 同上 |
| `task` | 任务 | **eino 框架 ReAct Agent** | **eino graph 编排** | 模型输出无 tool_call 的最终答案，或 `MaxStep` 上限 |

**问答 / 知识库** 是同一套手写循环（`internal/handler/chat.go:584` `runToolLoop`），区别只是知识库模式强制绑定了 `search_knowledge` 工具（`chat.go:316`）。

**任务模式** 是另一套东西：用的是 `github.com/cloudwego/eino/flow/agent/react` 提供的**标准 ReAct Agent**，循环、tool 注入、Observation 回灌全部由框架负责。这是本文档要讲清楚的核心。

> ⚠️ **不要误解：三种模式的 LLM 调用全部走 eino。**
>
> "手写 `runToolLoop`" 指的是**上层编排**自己写，**不是**说问答没用 eino。必须区分两个层面：
>
> | 层面 | 问答 / 知识库 | 任务 | 是否用 eino |
> |------|--------------|------|-------------|
> | **LLM 调用**（连模型、流式、tool 绑定） | `s.llm.Stream()` / `BindTools()` / `Generate()` | 同一个 `s.llm` | ✅ 三者全走 eino |
> | **编排层**（tool 循环怎么转） | 手写 `for` 循环 | eino ReAct Agent | 仅任务用 eino 编排 |
>
> 证据（均在 `internal/eino/chat.go`）：模型由 `openai.NewChatModel`/`claude.NewChatModel` 创建（:48/:61），类型是 eino 的 `model.ChatModel`（:22），流式走 `llm.Stream`（:192），工具绑定走 `llm.BindTools`（:139）。问答 `runToolLoop` 里那句 `h.chat.Stream()`（`chat.go:588`）底层就是 eino 的 `llm.Stream`。
>
> **一句话**：三者共用同一台 eino "发动机"（`ChatService.llm`）；问答/知识库**自己搭变速箱**（手写循环），任务**用 eino 配好的自动变速箱**（ReAct Agent）。发动机从没换过。

---

## 1. 结论先行：这是标准 ReAct，不是提示词模拟

很多"AI agent"其实是用 prompt 拼出来的伪 ReAct（让模型输出 `Action: xxx` 文本，再用正则解析）。**task 模式不是这样**，它是框架级真 ReAct，三个铁证：

### 1.1 框架级 ReAct 引擎

`internal/eino/react_agent.go:175`

```go
agentInst, err := react.NewAgent(ctx, &react.AgentConfig{
    ToolCallingModel: llm,
    ToolsConfig:      compose.ToolsNodeConfig{Tools: tools},
    MessageModifier:  modifier,
    MaxStep:          10,
})
```

Thought → Action → Observation 的循环由 eino 的 graph 编排驱动，**不是手写 for 循环**。

### 1.2 真正的 Function Calling，不是文本解析

- `llm` 类型是 `model.ToolCallingChatModel`（`react_agent.go:98`）
- 工具以 **JSON Schema** 通过 `BindTools(infos)` 注入模型（`chat.go:139`）
- tool_call 从结构化字段 `msg.ToolCalls` 读取（`react_agent.go:240`），走 OpenAI tool_calls 协议，**不是**从 content 正则抠出来的

### 1.3 Observation 自动回灌

`ToolCallbackHandler.OnEnd`（`react_agent.go:157`）拿到 tool result 后，eino **自动**把结果作为 tool message 塞回下一轮 LLM 输入 —— 这正是 ReAct 的 Observation 环节，框架代劳，我们没手写。

### 1.4 那一大段 system prompt 是干嘛的？

`taskSystemPrompt`（`react_agent.go:34`）里那堆"核心铁律"**不是在实现 ReAct，而是在矫正一个能力偏弱的模型**（deepseek-v4-flash）。两个层次要分清：

| 层次 | 谁负责 | 内容 |
|------|--------|------|
| ReAct 机制 | **eino 框架** | 循环编排、tool 注入、Observation 回灌、MaxStep |
| 行为矫正 | **我们的 prompt** | 防止小模型"口头计划"、防止照抄工具输出 |

一句话：**机制是标准 ReAct，提示词只是给一匹不太听话的马加的缰绳。**

---

## 2. 标准 ReAct 数据流

```
┌─────────────────────────────────────────────────────────────┐
│                     eino ReAct Agent                         │
│                  (react.NewAgent, MaxStep=10)                 │
│                                                              │
│   ┌──────────┐   tool_calls?   ┌──────────────┐             │
│   │  LLM     │ ───────yes────▶ │  ToolsNode   │             │
│   │ (Thought │                 │  (Action)    │             │
│   │ +Action) │ ◀──Observation──│  执行工具    │             │
│   └────┬─────┘   (tool message │  自动回灌    │             │
│        │          塞回输入)     └──────────────┘             │
│        │ no tool_calls                                       │
│        ▼                                                     │
│   最终答案 (content)                                          │
└─────────────────────────────────────────────────────────────┘
        │ Thought/Action/Observation 全程
        ▼
   TaskStep channel ──▶ task:step event ──▶ 前端 TaskMessageItem
```

教科书版 ReAct 到此为止：阻塞式一轮一轮跑，每轮拿到完整 LLM 输出后再决定。

---

## 3. 我们的改动：流式增强版 ReAct

标准 ReAct 是**阻塞式**的（一轮 LLM 跑完才有输出），但桌面 UI 需要**实时**看到模型思考。我们在不破坏 ReAct 语义的前提下做了一层并发增强。

### 3.1 核心手法：WithMessageFuture + 双 goroutine

`react_agent.go:208`

```go
futureOpt, future := react.WithMessageFuture()
outputStream, err := agentInst.Stream(ctx, msgs,
    agent.WithComposeOptions(compose.WithCallbacks(cb)),
    futureOpt,
)
```

`WithMessageFuture()` 让我们能在 agent 内部每一轮 LLM 产生 token 时就捞到流式 delta，而不必等整轮结束。

### 3.2 完整数据流（含我们的改动）

```
                    agentInst.Stream(ctx, msgs, futureOpt, callbacks)
                                      │
            ┌─────────────────────────┼─────────────────────────┐
            │                         │                         │
            ▼                         ▼                         ▼
┌───────────────────────┐  ┌──────────────────────┐  ┌────────────────────┐
│ goroutine 2 (主流程)   │  │ goroutine 1 (旁路)    │  │  Callbacks         │
│ react_agent.go:257    │  │ react_agent.go:223   │  │  (框架钩子)        │
│                       │  │                      │  │                    │
│ for outputStream.Recv │  │ future               │  │ ModelCallback.OnEnd│
│   推进 agent 执行      │  │  .GetMessageStreams()│  │  :130 记录每轮     │
│   (只 drain，不读内容) │  │  逐 token Recv():    │  │  tool_calls 数量   │
│                       │  │   ReasoningContent → │  │  + 无 tool_call 告警│
│                       │  │     thinking step    │  │                    │
│                       │  │   Content →          │  │ ToolCallback.OnStart│
│                       │  │     content step     │  │  :144 → tool_call  │
│                       │  │   ToolCalls →        │  │     step           │
│                       │  │     debug 日志 :240  │  │ ToolCallback.OnEnd │
│                       │  │                      │  │  :157 → tool_result│
└───────────┬───────────┘  └──────────┬───────────┘  └─────────┬──────────┘
            │                         │                        │
            │   都写入同一个          ▼                        │
            └──────────▶  ch (TaskStep, 缓冲 512) ◀────────────┘
                                      │
                         主 goroutine drain 完 +
                         <-iterDone (等旁路结束)
                                      │
                         ch <- TaskStep{Type:"done"}  :267
                                      │
                                      ▼
                    task.go: for step := range stepCh
                         runtime.EventsEmit("task:step")
                                      │
                                      ▼
                       前端 TaskMessageItem.vue 渲染推理链
```

### 3.3 改动清单（相对教科书 ReAct）

| # | 改动 | 位置 | 目的 | 是否破坏 ReAct 语义 |
|---|------|------|------|---------------------|
| 1 | `WithMessageFuture()` + 双 goroutine | `react_agent.go:208,223,257` | 流式实时推送 thinking/content 到 UI | 否，纯旁路观测 |
| 2 | `BuildAgentCallback` 三类钩子 | `react_agent.go:129-172` | 捕获 model round / tool start / tool end → 转 TaskStep | 否，框架原生钩子 |
| 3 | channel 缓冲 512 + 阻塞发送 | `react_agent.go:109` | 防止高频 token 丢 step | 否 |
| 4 | `MaxStep=10` | `react_agent.go:181` | 弱模型防失控（曾出现 30 步 web_search 死循环） | 弱化，见 §4 |
| 5 | `ReasoningEffort=Low`（deepseek） | `chat.go:82` | 深度思考会让 tool_call 意图变成纯文本输出 | 否，模型配置 |
| 6 | 反"口头计划" system prompt | `react_agent.go:34-89` | 矫正小模型不调工具只描述的毛病 | 否，prompt 层 |
| 7 | SkillTool 反照抄后缀 | `skill_tool.go:42` | 防止小模型把 144 行技能文档原样贴进回答 | 否，tool 输出层 |
| 8 | fallback `done` 事件 | `task.go` | ctx cancel 时 channel 静默关闭，前端不能依赖 agent 发 done | 否，健壮性 |
| 9 | BashTool emitter 注入 channel | `react_agent.go:112-121` | 危险命令确认/输出走同一 step 通道 | 否，工具能力 |

### 3.4 两层 goroutine 的分工（关键设计）

- **goroutine 2（主流程，`:257`）**：只负责 `outputStream.Recv()` 把 agent 推进到底，**不读内容**——它的作用是驱动 ReAct 循环跑完。
- **goroutine 1（旁路，`:223`）**：通过 `future.GetMessageStreams()` 实时捞每轮 LLM 的 streaming delta，转成 thinking/content step 喂前端。
- 主 goroutine drain 完后 `<-iterDone` 等旁路结束，再发 `done`，保证顺序正确、不漏 token。

---

## 4. 唯一不够"纯"的地方：MaxStep 兜底

标准 ReAct 理论上**只**靠模型自己输出"无 tool_call 的最终答案"来终止。我们额外加了 `MaxStep=10` 硬上限：

```
正常情况：模型信息够了 → 输出无 tool_call 的 content → ReAct 自然终止  ✅
异常情况：弱模型陷入重复搜索 → 撞 MaxStep=10 → 强制停止           🛡️ 兜底
```

这是对弱模型的**必要防失控措施**（v1.3.18 之前 `MaxStep=30` 出现过 9+ 次 web_search 死循环）。强模型（如 deepseek-chat V3）通常远不到 10 步就自然终止，兜底不会误伤。

---

## 5. 与问答/知识库的机制对比（再次强调）

```
┌─────────────── 问答 chat / 知识库 knowledge ───────────────┐
│  internal/handler/chat.go : runToolLoop (:584)            │
│                                                          │
│  for loopCount < maxToolLoops {        ← 我们手写的循环    │
│     stream = chat.Stream(messages)                       │
│     merged = ConcatMessages(chunks)    ← 我们手动合并      │
│     if len(toolCalls) == 0 { break }   ← 我们手动判终止    │
│     执行工具 → append tool message      ← 我们手动回灌      │
│  }                                                       │
└──────────────────────────────────────────────────────────┘

┌──────────────────────── 任务 task ────────────────────────┐
│  internal/eino/react_agent.go : react.NewAgent (:175)    │
│                                                          │
│  agentInst.Stream(...)                 ← 框架驱动整个循环  │
│    框架内部：Thought→Action→Observation 自动循环          │
│    框架自动：tool 注入 / 结果回灌 / 终止判断              │
│  我们只：旁路捞流式 token + 转 TaskStep                   │
└──────────────────────────────────────────────────────────┘
```

**核心区别**：
- 问答/知识库 = 我们**自己写**循环逻辑，框架只提供单次 `Stream`
- 任务 = 框架**自己跑** ReAct，我们只在旁边**观测 + 矫正**

---

## 6. 为什么问答/知识库不用 eino 框架驱动？

这是常被问到的问题。诚实回答：**主要是历史遗留，其次才有几条让它"不值得改"的合理性。** 事实与事后合理化分开陈述。

### 6.1 事实：时间线证明是历史遗留（git 实证）

| 时间 | commit | 事件 | 用什么 |
|------|--------|------|--------|
| 2026-05-29 | `ffcbef7` | chat 问答首次实现 | 手写 `runToolLoop` |
| 2026-05-31 | `ecd2ab7` | 加 skills/web search/MCP/agents | 仍是手写 `runToolLoop` |
| 2026-06-03 | `0eea59f` | task 模式从零实现 | eino `react.NewAgent` |

`git log -S "react.NewAgent" -- internal/handler/chat.go` **结果为空** —— chat.go **从未用过** eino ReAct Agent。

即：问答先写好跑通，5 天后做 task 时才第一次引入 eino ReAct。问答没换框架，纯粹是"能跑就不动它"。这是历史遗留，不是深思熟虑的设计选择。

### 6.2 合理性：但确实也"不值得改"

历史遗留之外，有三条让它继续保持现状是合理的：

1. **问答循环足够简单，框架收益小**
   `runToolLoop`（`chat.go:584`）仅 80 行，逻辑浅：Stream → 合并 chunk → 有 tool_call 就回灌 → 没有就结束。eino ReAct 的"自动编排"对这种简单循环价值不大，反而多一层抽象。

2. **问答需逐 token 直推前端，手写更直接**
   `chat.go:611` 每个 content chunk 直接 `EventsEmit("chat:chunk")`，手写循环里是一行的事。而 task 为了在框架内拿流式 token，被迫上 `WithMessageFuture()` + 双 goroutine（`react_agent.go:208-262`）。**问答用手写循环反而比框架更省心。**

3. **两者产物不同，数据通道天然分叉**
   - 问答：`chat:chunk` event → `MessageItem.vue`，追求打字机流畅，只要一股 content 流
   - 任务：`task:step` event → `TaskMessageItem.vue`，要展示 thinking/tool_call/tool_result 推理链，正是 ReAct callback 钩子擅长的

### 6.3 技术债认定

如果今天从零重写，**统一用 eino ReAct 是更干净的架构**（一套引擎，消除 `runToolLoop` 与 `react_agent` 重复）。两套并存的代价：

- tool 回灌逻辑写两遍（`chat.go:644` 手动 vs 框架自动）
- 维护两套终止判断、两套错误处理

**但现在不值得改**：问答那套已稳定数个版本，重构有回归风险，收益仅"架构统一"，ROI 不划算。**结论：可接受的技术债，记录在案，暂不重构。** 未来若问答循环需求变复杂（如并行 tool call、多轮 planning），再考虑统一到 eino ReAct。

---

## 7. 已知风险与根治方向

当前所有"矫正"都是 **prompt/输出层缓解**，没改变模型本身能力。面对 tool calling 偏弱的 deepseek-v4-flash，边缘场景仍可能出问题：

| 场景 | 风险 | 缓解现状 | 根治方向 |
|------|------|----------|----------|
| 10+ 步复杂多阶段任务 | 中途输出文本替代 tool_call | system prompt 铁律 | 换 deepseek-chat V3（tool calling 更强） |
| 超长工具返回（MCP/Web） | 照抄工具结果到回答 | SkillTool 反照抄后缀 | 工具结果 >3000 字符自动截断+摘要注入 |
| 多工具混合执行 | 选错执行顺序 | MaxStep 兜底 | 同上换模型 |
| 中文指令与 tool 描述歧义 | 误解意图 | prompt 明确化 | tool description 优化 |

**优先级最高的根治**：工具结果压缩（代码层防 regurgitation，不依赖模型自觉）+ 必要时切换强模型。`tool_choice: required` 强制首轮 tool_call 目前 eino 不支持。

---

## 8. 源码锚点速查

| 关注点 | 文件:行 |
|--------|---------|
| ReAct Agent 创建 | `internal/eino/react_agent.go:175` |
| system prompt（反口头计划） | `internal/eino/react_agent.go:34` |
| WithMessageFuture 流式 | `internal/eino/react_agent.go:208` |
| 旁路 goroutine（捞 token） | `internal/eino/react_agent.go:223` |
| 主 goroutine（推进循环） | `internal/eino/react_agent.go:257` |
| Callback 三钩子 | `internal/eino/react_agent.go:129-172` |
| ReasoningEffort=Low | `internal/eino/chat.go:82` |
| BindTools（JSON Schema 注入） | `internal/eino/chat.go:139` |
| SkillTool 反照抄 | `internal/eino/skill_tool.go:42` |
| StreamTask handler | `internal/handler/task.go:185` |
| fallback done 事件 | `internal/handler/task.go` |
| 对比：问答/知识库手写循环 | `internal/handler/chat.go:584` |
| 知识库强制挂 tool | `internal/handler/chat.go:316` |

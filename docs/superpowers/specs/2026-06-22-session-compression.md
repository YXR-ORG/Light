# 会话上下文压缩 — 设计与实现

> 版本：v1.0
> 日期：2026-06-22
> 状态：已实现（任务模式压缩已上线；问答/知识库模式待后续迭代）
> 关联：`docs/superpowers/specs/2026-06-04-task-react-architecture.md`（此文档已被本文部分替代）

---

## 0. 背景

Light 的任务模式（`task`）使用 eino ReAct Agent 执行多轮工具调用。长任务可能涉及大量工具输出（如 `bash_exec` 返回数千行、`read_file` 读取大文件、`web_search` 返回长文本），这些 `tool_result` 原样积累在对话历史中，几轮后就可能超出模型的上下文窗口，导致：

- 任务执行被 `MaxStep` 提前终止（撞限）
- 早期关键上下文（用户原始需求、计划步骤）被挤压
- LLM 因 token 限制丢失"记忆"

**解决方案**：利用 eino 框架 `adk/middlewares/` 提供的两个现成中间件——`reduction`（无损工具输出裁剪）和 `summarization`（LLM 语义压缩）——对任务模式的上下文进行自动管理。

与此同时，这两个中间件要求 agent 构造使用 eino 的 `adk.ChatModelAgent`（而非旧版 `flow/agent/react`），因此本次变更也包含一次 agent API 迁移。

---

## 1. 目标

1. **透明压缩**：在模型不可感知的情况下管理上下文大小，不改变最终回复质量
2. **零代码侵入**：利用 eino 框架中间件，我们不实现压缩逻辑，只做配置
3. **API 升级**：任务模式 agent 从 `flow/agent/react` 迁移到 `adk.ChatModelAgent`，为后续更多中间件铺路

---

## 2. 方案选择：eino 框架两个中间件

### 2.1 reduction — 工具输出卸载

**包路径**：`github.com/cloudwego/eino/adk/middlewares/reduction`

**机制**（两阶段）：

| 阶段 | 触发条件 | 行为 |
|------|---------|------|
| Truncation | 单次 tool output 超过 `MaxLengthForTrunc`（默认 50000 字符） | 完整内容写入文件，消息中只留截断提示 |
| Clear | 总 token 超过 `MaxTokensForClear`（默认 160k） | 遍历历史，将旧的 tool_call/tool_result 对卸载到文件，替换为占位符。保留最近 `ClearRetentionSuffixLimit` 条消息不动 |

**关键特性**：
- **无损**：内容落盘到文件（`RootDir/{trunc,clear}/{tool_call_id}`），agent 可通过 `read_file` 工具回读
- **零 LLM 成本**：纯字符串裁剪 + 文件 I/O，不消耗 token
- **tool result 专用**：只处理 tool call/result 对，不碰用户消息和 assistant 文本回复
- 我们已有的 `read_file` 工具天然满足 "卸载后用 read_file 回读" 的设计要求

**适用场景**：工具输出过大的情况——`bash_exec` 返回完整构建日志、`read_file` 读大文件等。

### 2.2 summarization — LLM 语义压缩

**包路径**：`github.com/cloudwego/eino/adk/middlewares/summarization`

**机制**：
- 当消息总 token 超过 `Trigger.ContextTokens`（默认 160k）时触发
- 调一个 LLM（可与主模型相同或不同）将历史对话总结为结构化摘要
- 摘要替换原有消息，保留 system prompt + 摘要 + 最近若干条用户消息原文（`preserveUserMsgsMaxTokens=30000`）
- 支持 retry（3 次）和 failover（降级模型）

**关键特性**：
- **有损但保语义**：丢失具体措辞但保留关键信息（文件、决策、用户反馈）
- **消耗一次 LLM 调用**：需要配置单独的模型（建议用便宜模型如 deepseek-flash）
- 内置中/英双语 prompt
- 可通过 `TranscriptFilePath` 保存完整对话原文，agent 需要细节时可引导其回读

**适用场景**：长对话 → 语义层面需要压缩，如多步任务执行到一半上下文就快满了。

### 2.3 两者配合策略

```
reduction（先触发）→ 机械裁剪大工具输出，零成本 "瘦身"
         ↓
summarization（后触发）→ token 仍超标时，LLM 语义压缩
```

reduction 处理的是局部膨胀（单个大工具输出），summarization 处理的是整体膨胀（整个对话历史太长了）。两者配合形成两层防线。

**配置参数规划**：

| 中间件 | 参数 | 建议值 | 理由 |
|--------|------|--------|------|
| reduction | `MaxLengthForTrunc` | 50000 | 单次输出>50KB 才截断 |
| reduction | `MaxTokensForClear` | 120000 | 为大模型（如Claude 200k）留足空间 |
| reduction | `ClearRetentionSuffixLimit` | 3 | 保留最近 3 条消息不清理 |
| reduction | `ReadFileToolName` | `"read_file"` | 匹配我们已有的文件工具 |
| summarization | `Trigger.ContextTokens` | 120000 | 与 reduction 一致 |
| summarization | `Model` | 主模型（可选降级） | 首次用主模型，后续可优化为便宜模型 |

---

## 3. 架构变更：agent API 迁移

### 3.1 为什么必须迁移

`flow/agent/react`（旧版）的 `AgentConfig` **没有中间件挂载点**。`adk.ChatModelAgent` 通过 `Handlers []ChatModelAgentMiddleware` 字段支持中间件链，`reduction` 和 `summarization` 都实现了 `ChatModelAgentMiddleware` 接口。

```go
// 旧版：不能挂中间件
react.NewAgent(ctx, &react.AgentConfig{...})

// 新版：可以挂中间件
adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
    Handlers: []adk.ChatModelAgentMiddleware{
        reductionMW,
        summarizationMW,
    },
})
```

> ⚠️ 注意：`flow/agent/react` 的 `AgentMiddleware`（`chatmodel.go:236`）已被 eino 官方标注为 **Deprecated**。`adk.ChatModelAgent` 是当前推荐方式。

### 3.2 API 映射

| 旧版（`flow/agent/react`） | 新版（`adk`） | 说明 |
|---------------------------|--------------|------|
| `react.NewAgent(ctx, config)` | `adk.NewChatModelAgent(ctx, config)` | 构造 |
| `config.ToolCallingModel` | `config.Model` | LLM 模型字段重命名 |
| `config.ToolsConfig` | `config.ToolsConfig.ToolsNodeConfig` | 嵌套一层 |
| `config.MessageModifier` | `config.Instruction` | 直接传 system prompt 字符串 |
| `config.MaxStep` | `config.MaxIterations` | 语义重命名（默认 20→100） |
| `agent.Stream(ctx, msgs, opts...)` | `agent.Run(ctx, input)` 或 `runner.Run(ctx, msgs)` | 流式 → 事件迭代器 |
| `react.WithMessageFuture()` | 事件迭代器内置，无需额外 option | 每轮 model output 本身就是 `AgentEvent` |
| `react.BuildAgentCallback(mh, th)` | 事件流用 `Output.MessageOutput.Role` 区分 assistant/tool | 不再需要 callback，直接读事件 |

### 3.3 事件模型迁移（关键）

**旧版流程**（双 goroutine + MessageFuture）：
```
goroutine 1: future.GetMessageStreams() → 逐轮读 LLM 输出流
goroutine 2: outputStream.Recv() → 推进 agent 执行
→ 双路并发，手动同步
```

**新版流程**（单事件循环）：
```go
iter := agent.Run(ctx, &adk.AgentInput{
    Messages:        msgs,
    EnableStreaming: true,
})

for {
    event, ok := iter.Next()
    if !ok { break }
    if event.Err != nil { /* 错误处理 */; break }

    mv := event.Output.MessageOutput
    switch mv.Role {
    case schema.Assistant:
        // model output（含 content/thinking/tool_call）
        // 读 mv.MessageStream 逐 chunk
        for {
            chunk, err := mv.MessageStream.Recv()
            if err != nil { break }
            // 推送 content/thinking delta 到前端
        }
    case schema.Tool:
        // tool result
        // 推送 tool_result step 到前端
    }
}
```

**保留的逻辑**（迁移但不变）：
- `content_rollback`（短 content + tool_call = 旁白回滚）：在 Assistant event 的 stream 读完一轮后，检查是否有 tool_call，决定是 `content` 还是 `content_rollback`。等价于旧版逻辑，只是数据来源从 MessageFuture 变成 event。
- `content_note`（折叠链显示过程旁白）：同上。
- 补总结轮（MaxStep 撞限后用 accumulated messages 再调一次 LLM）：累积方式从 MessageFuture 改为从事件流收集。
- 死循环检测：从 `ToolCallbackHandler.OnStart` 改为自定义 `ChatModelAgentMiddleware.WrapToolCall`。
- BashConfirm 机制：BashTool 的 `emitter` 通道独立于 eino 事件系统，完全不受影响。

---

## 3.P0 重要的技术约束：禁止混用 API

> ⚠️ **编码时注意：eino 框架有两种 ReAct agent API，必须严格隔离。**

由于 eino 的 `adk` 包内也定义了 `react.go`（用于内部 ReAct 逻辑编排），且 `adk.NewChatModelAgent()` 返回的类型与 `flow/agent/react.NewAgent()` 完全不同，**禁止在同一个文件中同时 import 两个包**。

所有新开发的 task agent 逻辑都将使用以下的 API：

```go
import "github.com/cloudwego/eino/adk"

agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{...})
```

相关的步骤：
- 禁止 import `github.com/cloudwego/eino/flow/agent/react` 或 `github.com/cloudwego/eino/flow/agent`，确保不会意外调用旧版 API。
- 不要试图在 `adk` 的 agent 上使用 `flow/agent/react` 的任何工具方法。
- 查询资料时，必须只看 `adk/` 目录下的接口和文档。

## 4. 实施计划

### 第零步（prerequisite）：熟悉 eino adk 的事件模型和中间件 API（已完成 ✅）

### 第一步（1-2 天）：迁移 agent API，不挂中间件

**目标**：`react_agent.go` 用 `adk.ChatModelAgent` 重写，**行为与旧版完全等价**。

改动范围：
- `internal/eino/react_agent.go`：重写 agent 构造（约 30 行）和事件流读取（约 100 行），保留 content_rollback、content_note、补总结轮、死循环检测、BashEmitter 全部逻辑
- `internal/handler/task.go`：适配新的事件迭代器（约 20 行调整）

验证方法：启动应用，跑同一个任务，确认前端推理链展示、最终回复完全一致。

### 第二步（0.5 天）：挂载 reduction + summarization

**目标**：在第一步成功的基础上，加入压缩中间件。

改动范围：
- `internal/eino/react_agent.go`：在 agent config 的 `Handlers` 里增加 reductionMW 和 summarizationMW（约 10 行）
- 新增 `internal/eino/compress.go`：封装中间件构造、token 计数等（约 50 行）

验证方法：构造一个长任务（如"总结这篇 5000 行代码"），观察日志确认压缩触发，确认前端无异常。

### 后续（优先级中）：问答/知识模式的压缩

问答/知识模式使用手写 `runToolLoop`（`internal/handler/chat.go:584`），不在 adk runner 内。方案：
- 在 `history` 加载后、构建 `einoMsgs` 前，手动调用 `summarization.Middleware.Summarize()` 进行压缩
- 或在 `runToolLoop` 每轮循环前检查 token 数，超阈值时触发压缩
- **暂缓**，优先保证任务模式压缩稳定后再铺开

---

## 5. 受影响文件

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `internal/eino/react_agent.go` | 重写 | 主要改动，约 180 行 |
| `internal/eino/compress.go` | 新增 | 中间件构造封装 |
| `internal/handler/task.go` | 适配 | 事件消费方式调整，约 20 行 |
| `docs/superpowers/specs/2026-06-04-task-react-architecture.md` | 过时标记 | 标注旧版 API 已废弃，指向本文档 |

---

## 6. 技术约束

- eino v0.9.2 已包含 `adk/middlewares/reduction` 和 `summarization`，无需升级版本
- reduction 的文件存储路径：`workDir/.light/reduction/`，纳入项目 `.gitignore`
- summarization 的 transcript 存储路径：`workDir/.light/transcript.md`
- 中间件的 `TokenCounter` 默认用 4 字符≈1 token 估算，对中文略偏高但可接受；后续可接 `tiktoken-go` 精确计数
- `flow/agent/react` 已被 eino 标记为 Deprecated，迁移是规范要求的
- **`summarization` 和 `reduction` 中间件仅在任务（task）模式下加载，问答/知识库模式暂不使用**

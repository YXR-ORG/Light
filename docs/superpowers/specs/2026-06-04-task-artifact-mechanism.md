# Task 模式 — 通用产物机制 & 自适应执行

> 版本：v1.1
> 日期：2026-06-04
> 状态：已实现（v1.4.0），v1.5.2 补充 plan/file 历史分桶约束
> 关联：`2026-06-03-task-mode-design.md`（功能设计）、`2026-06-04-task-react-architecture.md`（ReAct 架构）

本文档记录 task 模式在 v1.4.0 引入的两大机制：**通用产物（Artifact）机制** 与 **自适应执行（步数 + 总结兜底）**，以及配套的正文/旁白分离、清除上下文交互。

---

## 1. 通用产物（Artifact）机制

### 1.1 解决的问题

Agent 执行任务时会产生「产物」：生成的文件、读取的文件、未来可能的图片/链接等。这些产物需要在 UI 上**醒目、可交互地展示**（点击打开文件等），而不是埋在折叠的推理链里。

早期实现用两种写死的标记（`WRITE_FILE_RESULT` + `FILE_REF`），各写一套解析逻辑，每加一种产物就要改前后端，且用 `lastIndexOf('-->')` 定位 JSON，**当产物内容本身含 `-->`（如读取 HTML）时解析错位**。

### 1.2 核心思想：带外数据通道

工具在返回给 LLM 的文本里夹带一段**对人类不可见、对前端可解析**的标记。一份返回值同时服务两个消费者：

```
工具返回值 = 人类可读文本 + <!--ARTIFACT:base64(json)-->
                  ↓                      ↓
            LLM 读这部分            前端解析这部分 → 渲染产物卡片
```

- LLM 看到的是可读文本（HTML 注释通常被模型忽略）
- 前端解析标记，自动在「产物区」渲染
- **Agent 框架（react_agent）只透传，不感知产物语义**

### 1.3 统一标记格式

```
<!--ARTIFACT:base64(json)-->
```

- **单一标记类型**，取代旧的两种
- meta 用 **base64 编码**，彻底规避产物内容含 `-->` 导致的解析错位
- JSON 结构（`Artifact`）：

```go
type Artifact struct {
    Type    string // file | image | url | ...（可扩展）
    Action  string // write | read（file 专用）
    Title   string // 展示标题
    Path    string // 相对路径（file）
    AbsPath string // 绝对路径（file，用于打开/定位）
    URL     string // 链接（url/image）
    Bytes   int    // 字节大小
    Mime    string // MIME 类型
}
```

### 1.4 后端 API（`internal/eino/artifact.go`）

```go
EmbedArtifact(humanText string, a Artifact) string   // 追加单个产物标记
EmbedArtifacts(humanText string, arts ...Artifact)   // 追加多个
StripArtifacts(s string) string                       // 移除所有标记（纯文本展示/存储）
ParseArtifacts(s string) []Artifact                   // 提取所有产物
```

任意工具接入只需一行：

```go
return EmbedArtifact("文件已写入: report.md（1234 字节）", Artifact{
    Type: "file", Action: "write", Title: "report.md",
    Path: "report.md", AbsPath: abs, Bytes: 1234,
}), nil
```

**新工具零成本接入**：埋了标记 = 自动在产物区出现，无需改 agent 或前端任何代码。

### 1.5 前端

| 文件 | 职责 |
|------|------|
| `utils/artifacts.ts` | `parseArtifacts` / `stripArtifacts` / `collectArtifacts` / `splitTaskArtifacts`，UTF-8 安全的 base64 解码与 task artifact 分桶 |
| `components/ArtifactCard.vue` | 按 `type` 渲染卡片：file 点击打开、url/image 浏览器打开，带类型标签和「定位」按钮 |
| `components/TaskMessageItem.vue` | 调 `collectArtifacts(所有 tool_result)` → 渲染产物区 |

「自动显示」的关键：`collectArtifacts` 扫描所有 `tool_result`，**不关心是哪个工具产生的**，只要有 ARTIFACT 标记就收集。

渲染前必须调用 `splitTaskArtifacts` 做显式分桶：

| Artifact type | UI 区域 | 约束 |
|---------------|---------|------|
| `plan` | 回复顶部「执行计划」卡片 | 历史和实时任务渲染一致 |
| `file` | 「本次涉及的文件」区域 | 文件区只允许 `type === 'file'` |
| 其他类型 | 预留「相关产物」区域 | 不得放进文件区 |

禁止使用 `type !== 'plan'` 作为文件区判断条件。这个负向判断会把未来新增的 `url` / `image` / `chart`，以及任何异常 type 的 artifact 误归入「本次涉及的文件」。

### 1.6 持久化（历史会话也能看）

产物存入数据库，跨会话/重启后仍可见：

- `Message` 表新增 `Artifacts string` 字段（JSON `[]Artifact`），AutoMigrate 无损加列
- `task.go` 消费 step 流时累积所有 `tool_result` 的产物，按 `abs_path/url` 去重（write 优先 read），done 时序列化存入消息
- 前端 `TaskMessageItem` 的 `artifacts` 计算属性：历史模式优先用持久化的 `artifactsJson`，实时流式则从 steps 收集
- 历史模式模板也渲染产物区
- plan 按 `type:"plan"` 持久化；历史恢复时仍显示为执行计划卡片，不得计入「本次涉及的文件」数量

### 1.7 边界正确性

`artifact_test.go` 覆盖：
- 往返一致（embed → parse）
- **产物内容含 `-->` 不破坏解析**（base64 的核心收益）
- 多产物提取

---

## 2. 自适应执行（步数 + 总结兜底）

### 2.1 解决的问题

旧实现 `MaxStep=10` 太小，复杂多轮任务（多次搜索）撞上限被框架直接中断、报错、正文空。用户体验是「达到上限，什么也没拿到」。

### 2.2 关键事实：MaxStep 是天花板不是固定步数

读 eino 源码确认：ReAct Agent 在「模型输出无 tool_call 的最终答案」时**自然终止**，`MaxStep`（`compose.WithMaxRunSteps`）只是上限。简单任务设 100 也只跑几步就停。

因此「指数退避加步数」对真实任务无意义（做完就停，撞不到上限），还要承担撞限后重建消息历史重放的 400 风险。最终采用**等价的简化方案**。

### 2.3 方案

```
MaxStep = 100（天花板，不强制跑满）
+ 死循环早停：连续 6 轮相同 tool+参数 → 主动 cancel 止损（比跑满更省配额）
+ 补总结轮：结束时若无正文 → 追加一次"禁用工具"请求，让模型基于已收集信息产出
+ 分类提示：正常完成 / 撞上限 / 死循环，给用户不同 notice
```

### 2.4 实现要点（`react_agent.go`）

- **死循环检测**：`ToolCallbackHandler.OnStart` 记录工具调用签名（name+args），连续 6 次相同 → `runCancel()`
- **真实结束原因**：消费 `outputStream.Recv()` 的 err。`io.EOF` = 正常结束；`exceeds max steps` = 撞限。**不再只靠不可靠的 `hasFinalContent` 判断**（这曾导致正常完成被误判为撞限、弹假提示）
- **补总结轮** `runSummaryRound`：不绑工具，把累积的 assistant/tool 消息作为上下文，让模型直接产出最终答案；tool 结果转「【工具结果】」旁白形式避免 tool_call 配对校验
- **三种结束分支**：
  1. 正常结束 + 有正文 → 直接完成
  2. 正常结束 + 无正文 → 静默补总结（不弹"撞限"假提示）
  3. 撞限/死循环 → 补总结 + notice 提示

---

## 3. 正文 / 旁白 / 工具结果 分离

task 模式的「正文」必须只含最终答案，不能混入工具结果或过程旁白。

### 3.1 三类内容

| 来源 | 去向 | 判定 |
|------|------|------|
| 最终答案（无 tool_call 轮的 content） | 正文（流式渲染 markdown） | 该轮无 tool_call |
| 过程旁白（含 tool_call 轮的 content） | 折叠链（content_note） | 该轮有 tool_call |
| 工具结果（Role=Tool 消息） | 折叠链（tool_result） + 产物区 | `msg.Role == schema.Tool` |

### 3.2 流式 + 乐观回滚

矛盾：收到 content delta 当下，还不知本轮最终是否含 tool_call，无法立即分类。

解法（恢复实时流式的同时保证分离）：
- content **逐 chunk 实时推送**到正文（和 thinking 一样即时）
- 本轮结束若发现 `hasToolCall` → 发 `content_rollback`，前端把本轮 delta 从正文末尾撤回、改入折叠链
- 工具结果消息（Role=Tool）的 content **绝不进正文**，只入收集列表 + 产物区

deepseek 通常 tool_call 在早期 chunk 出现，旁白很短，回滚几乎无感。最终答案轮（最常见）纯流式无回滚。

### 3.3 markdown 实时渲染

与 chat 模式一致，正文**始终用 marked 渲染**（不再「流式纯文本、完成才 markdown」）。marked 对不完整语法容错，下一帧自动修正。

---

## 4. 清除上下文（一次性交互）

- 按钮**无选中高亮**（一次性动作，非持续开关）
- 点击 → 显示「上下文从此处清除」分割线 + 本次不带历史；再点 → 取消
- 发送后状态复位、分割线消失
- task 模式分割线基于 `store.taskCutoffActive` 实时渲染，支持历史会话（不再依赖 `completedRounds` 标记）

---

## 5. 源码锚点

| 关注点 | 文件:符号 |
|--------|-----------|
| 产物结构与 API | `internal/eino/artifact.go` |
| 产物往返测试 | `internal/eino/artifact_test.go` |
| file 工具埋产物 | `internal/eino/file_tool.go`（read/write_file） |
| 前端产物解析 | `frontend/src/utils/artifacts.ts` |
| 产物卡片 | `frontend/src/components/ArtifactCard.vue` |
| 自适应步数/总结/死循环 | `internal/eino/react_agent.go`（RunTaskAgent, runSummaryRound） |
| 正文/旁白/回滚分流 | `internal/eino/react_agent.go`（iterator goroutine） |
| 产物持久化 | `internal/storage/models.go`（Message.Artifacts）、`conversation.go`（SaveTaskMessageWithArtifacts）、`internal/handler/task.go`（采集） |
| 清除上下文交互 | `frontend/src/components/InputArea.vue`、`ChatView.vue`、`stores/chat.ts` |

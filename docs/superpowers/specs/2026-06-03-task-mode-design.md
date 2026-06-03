# Task Mode — Design Spec

> 版本：v0.1  
> 日期：2026-06-03  
> 状态：待实现

---

## 1. 背景与目标

Light 现有两种对话模式：

| 模式 | 行为 |
|------|------|
| `chat` | 普通 LLM 对话，可挂 MCP/skill/web search，手写 ReAct 循环 |
| `knowledge` | 强制挂 `search_knowledge` tool，知识库问答 |

**Task 模式**是第三种模式，核心差异：

1. 用户只描述目标，**agent 自主决定**调用哪些工具、查哪些资源
2. 自动挂载**所有已配置的启用资源**（MCP、skill、知识库、web search）
3. 新增 `bash_exec` 工具，agent 可执行 shell 命令（危险命令需前端弹窗确认）
4. 新增文件操作工具（限制在工作目录内）
5. 使用 **eino ReAct Agent** 替代手写循环，具备并行 tool call 能力
6. 推理链全程可见（思考 / tool call / tool result / 最终答案）
7. 支持用户随时中断，已输出内容保留

---

## 2. 整体架构

```
前端 InputArea / TaskInputBar
  ├── mode: chat      → StreamChat (现有 runToolLoop，不动)
  ├── mode: knowledge → StreamChat (现有 KB 路径，不动)
  └── mode: task      → StreamTask (新 handler)
                            ↓
                      tool_registry.go (自动发现所有工具)
                            ↓
                      react_agent.go (eino ReAct Agent)
                            ↓
                      task:step events → 前端 TaskMessageItem
```

chat / knowledge 模式完全不变。task 模式走独立路径。

---

## 3. 后端新增文件

```
internal/
  handler/
    task.go            StreamTask() handler，管理生命周期
  eino/
    react_agent.go     封装 eino react agent，产出 TaskStep channel
    bash_tool.go       BashTool，危险命令阻塞等待前端确认
    file_tool.go       FileTool (read/write/list_dir/make_dir)
    tool_registry.go   自动发现所有启用资源，组装 []tool.BaseTool
```

---

## 4. 工具集与注册

### 4.1 自动发现（BuildTaskTools）

每次 `StreamTask` 调用时执行，无需用户手动选择：

| 来源 | 工具 | 实现 |
|------|------|------|
| 已启用 MCP servers | 每个 server 的所有 tool | eino-ext/tool/mcp（复用现有） |
| 所有 skills | 每个 skill 一个 tool | SkillTool（复用现有） |
| 所有知识库 | 每个 KB 一个 `search_<kbName>` tool | KnowledgeSearchTool（复用现有） |
| Web search（已配置） | `web_search` | WebSearchTool（复用现有） |
| 内置 | `bash_exec` | BashTool（新增） |
| 内置 | `read_file` / `write_file` / `list_dir` / `make_dir` | FileTool（新增） |

### 4.2 BashTool

```go
// Info
name: "bash_exec"
desc: "在工作目录中执行 shell 命令。危险命令会请求用户确认后执行。"
params:
  cmd (string, required): shell 命令
  timeout_sec (int, optional): 超时秒数，默认 30，最大 300

// 执行流程
InvokableRun(cmd):
  1. 检查黑名单（见 4.3）
  2. 若匹配黑名单 → 发 task:step{type:bash_confirm, id, cmd}
                   → 阻塞 channel，超时 120s
                   → 用户确认 → 继续
                   → 用户拒绝 → 返回 "用户已拒绝执行此命令"
                   → 超时    → 返回 "等待确认超时，命令未执行"
  3. exec.CommandContext(ctx, "sh", "-c", cmd)，CWD=workDir
  4. 合并 stdout+stderr，实时推送 task:step{type:bash_output}
  5. 超时 kill 进程，返回超时错误
  6. 返回完整输出（最多 50KB，超出截断并提示）
```

### 4.3 危险命令黑名单

初始内置规则（存 `settings` 表，key=`task_bash_blacklist`，可在设置页修改）：

```
rm -rf
sudo
mkfs
dd if=
curl * | sh
curl * | bash
wget * | sh
wget * | bash
> /dev/
chmod 777 /
```

匹配方式：glob 通配符，大小写不敏感。命中任意一条 → 触发确认弹窗。

### 4.4 FileTool

所有操作路径必须在 `workDir` 下，否则返回错误（防止越权读写）。

| tool name | 参数 | 说明 |
|-----------|------|------|
| `read_file` | path | 读文件，返回内容（最多 100KB） |
| `write_file` | path, content | 写文件（自动创建父目录） |
| `list_dir` | path | 列目录，返回文件名+大小+修改时间 |
| `make_dir` | path | 创建目录（含父目录） |

---

## 5. eino ReAct Agent 封装

### 5.1 TaskAgent

```go
// internal/eino/react_agent.go

type TaskStep struct {
    Type       string // thinking / tool_call / tool_result / bash_confirm
                      // bash_output / content / done / error
    Content    string // LLM 思考文本 或 最终回答片段
    ToolName   string // tool_call / tool_result 时有值
    ToolArgs   string // tool_call 时：JSON args（格式化后）
    ToolResult string // tool_result 时：执行结果
    ConfirmID  string // bash_confirm 时：唯一 ID，前端用于回调
    Cmd        string // bash_confirm / bash_output 时：命令文本
    Error      string // error 时有值
}

type TaskAgent struct {
    workDir string
    llm     model.ChatModel
    tools   []tool.BaseTool
}

func NewTaskAgent(llm model.ChatModel, tools []tool.BaseTool, workDir string) *TaskAgent

// Run 启动 agent，返回 step channel（缓冲 32）
// ctx cancel → agent 停止，channel 关闭
func (a *TaskAgent) Run(ctx context.Context, systemPrompt, userMsg string, history []*schema.Message) (<-chan TaskStep, error)
```

### 5.2 System Prompt（task 模式）

```
你是一个自主任务执行智能体。

工作目录：{workDir}
当前时间：{time}

你可以使用以下资源：
- bash_exec：执行 shell 命令（危险操作会请求用户确认）
- read_file / write_file / list_dir / make_dir：文件操作（限工作目录内）
- 知识库检索、技能、网络搜索、MCP 工具（已在工具列表中）

执行原则：
1. 分析任务，制定步骤，逐步执行
2. 优先使用现有工具，不要重复造轮子
3. 文件操作限制在工作目录内
4. bash 命令执行前思考是否必要
5. 任务完成后给出简洁的执行摘要（完成了什么、产生了哪些文件）
6. 如果任务无法完成，说明原因和建议
```

### 5.3 中断机制

```
用户点停止
  → 前端调 StopTask() handler
  → cancelFunc() 取消 context
  → eino agent stream 感知 ctx.Done() 停止
  → BashTool 正在执行的子进程：cmd.Process.Kill()
  → BashTool 正在等待确认的 channel：关闭，返回"已取消"
  → 已推送的 step 内容保留
  → 发送 TaskStep{Type: "done"} 结束
```

---

## 6. Handler 设计

### 6.1 StreamTask

```go
// internal/handler/task.go

type StreamTaskRequest struct {
    ConversationID string   `json:"conversationId"`
    Content        string   `json:"content"`
    WorkDir        string   `json:"workDir"`        // 工作目录
    ModelID        string   `json:"modelId"`        // 选择的模型
    AgentID        string   `json:"agentId"`        // 可选，覆盖 system prompt
    RegenerateGroupID string `json:"regenerateGroupId"` // 重新生成时有值
}

func (h *TaskHandler) StreamTask(req StreamTaskRequest) error
```

流程：

```
1. 校验 workDir 存在（不存在则返回错误）
2. BuildTaskTools(ctx, workDir)  // 自动发现所有工具
3. 加载 conv history（GetLatestMessages）
4. NewTaskAgent(llm, tools, workDir)
5. agent.Run(ctx, systemPrompt, req.Content, history)
6. for step := range stepCh:
     runtime.EventsEmit(ctx, "task:step", step)
     if step.Type == "done" || step.Type == "error": break
7. 保存消息到数据库（SaveMessage）
```

### 6.2 ConfirmBash

```go
func (h *TaskHandler) ConfirmBash(confirmID string, approved bool) error
```

前端点击确认/拒绝后调用，通过 `confirmID` 找到对应的 BashTool 等待 channel，发送结果。

### 6.3 StopTask

```go
func (h *TaskHandler) StopTask(conversationID string) error
```

调用对应 conversation 的 cancelFunc()。

---

## 7. 数据模型变更

### 7.1 Conversation 新增字段

```go
WorkDir string `gorm:"column:work_dir;default:''"` // task 模式工作目录
```

AutoMigrate 自动加列，旧数据库无感升级。

### 7.2 Settings 新增 Key

| key | 类型 | 默认值 | 说明 |
|-----|------|--------|------|
| `task_bash_blacklist` | string（换行分隔） | 内置规则列表 | 危险命令黑名单 |
| `task_default_work_dir` | string | `~/Documents` | 默认工作目录 |

---

## 8. 前端组件

### 8.1 TaskInputBar.vue

task 模式下替换 InputArea 底部工具栏，新增：

- **工作目录选择器**：显示当前路径，点击触发 `runtime.OpenDirectoryDialog`，持久化到 conv
- **模型选择器**：独立于全局 agent 配置，可单独为 task 选模型

### 8.2 TaskMessageItem.vue

展示一条 task 消息的完整推理链：

```
[折叠] 🤔 思考         ← thinking steps，默认折叠
[展开] 🔧 bash_exec    ← tool_call，显示命令
       └ 输出: ...     ← tool_result
[展开] 🔧 read_file    ← tool_call
       └ 内容: ...     ← tool_result
[展开] 💬 最终回答     ← content，流式输出，默认展开
```

每个 step 卡片可独立折叠/展开。tool_result 超过 500 字符自动截断，显示"展开全部"。

### 8.3 BashConfirmDialog.vue

全局单例弹窗，收到 `task:step{type:bash_confirm}` 时显示：

```
⚠️ Agent 请求执行以下命令

  $ rm -rf ./dist

工作目录：~/Documents/my-project

                          [拒绝]  [执行]
```

点击后调用 `ConfirmBash(id, bool)` handler，弹窗关闭。

### 8.4 设置页扩展

`设置 → 安全` 新增 tab（或独立区域）：

- **默认工作目录**：文本框 + 选择文件夹按钮
- **Bash 黑名单**：多行文本框，每行一条规则，支持 glob 通配符
- 保存后立即生效（下次 task 执行时读取）

---

## 9. Event 协议

task 模式使用独立的 `task:step` event，不与 `chat:chunk` 混用。

```typescript
interface TaskStep {
  type: 'thinking' | 'tool_call' | 'tool_result' | 'bash_confirm'
        | 'bash_output' | 'content' | 'done' | 'error'
  content?: string      // thinking / content
  toolName?: string     // tool_call / tool_result
  toolArgs?: string     // tool_call（JSON 字符串，已格式化）
  toolResult?: string   // tool_result
  confirmId?: string    // bash_confirm
  cmd?: string          // bash_confirm / bash_output
  error?: string        // error
}
```

---

## 10. 不在本期范围内

- **Planning 阶段**：任务分解为子任务列表，逐步执行（方案 C 的能力，待后续版本）
- **多 agent 协作**：主 agent 派发子 agent
- **task 历史持久化**：推理链 step 存数据库（本期只存最终回答）
- **知识库多选挂载**（TODO-APP-3，独立需求）
- **RRF 参数调优**

---

## 11. 成功标准

| 场景 | 验收条件 |
|------|----------|
| 基本任务 | 用户描述"帮我在工作目录下创建一个 README"，agent 自主调用 write_file 完成 |
| bash 安全 | agent 调用 `rm -rf ./dist`，前端弹窗，用户确认后执行；用户拒绝则不执行 |
| 安装 skill | agent 执行 `npm install` 等安全命令无需确认，直接执行 |
| 知识库自动挂载 | task 模式下 agent 自动调用知识库 tool，无需用户手动选 |
| 中断 | 用户点停止，agent 停止，已输出内容保留 |
| 黑名单可配置 | 设置页修改黑名单后，下次 task 执行时生效 |
| 工作目录隔离 | FileTool 访问 workDir 外的路径返回错误 |
| 模式切换 | 切回 chat 模式，runToolLoop 行为不变 |

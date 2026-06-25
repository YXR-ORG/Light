# 工作流模式（Workflow Mode）— v1.9.0

> 日期：2026-06-25
> 状态：已实现

## 概述

在 task 模式基础上新增第四种对话模式 **workflow（工作流）**，实现结构化的"需求→设计→计划→编码→验收"开发工作流。
workflow 模式内含 goal 模式的全部自主执行逻辑（autoApprove + finish_goal + 持续执行），并叠加 5 阶段结构化 prompt 和 2 个新工具。

## 设计决策

1. **workflow 是独立模式**，与 goal 互斥。Conversation.Mode 增加 `workflow` 值。
2. **workflow 内含 goal 逻辑**：后端判断 `mode==workflow` 时自动启用 autoApprove + finish_goal + 持续执行，叠加 5 阶段 prompt + 新工具。
3. **产物仅 artifact 卡片展示**，不落盘文件。
4. **独立 review 轮次**：编码阶段 finish_goal 后，后端发起独立 LLM 调用（禁用写工具，只读检查），对照验收标准逐项审查。

## 5 阶段流程

```
① submit_spec      → 产出 requirement 卡片（需求描述 + 验收标准 checklist）
② 设计方案          → agent 直接输出 markdown 正文（content step）
③ update_plan       → 产出 plan 卡片（复用现有 plan 工具，强制启用）
④ 编码实现          → bash/file 工具（复用现有，autoApprove=true）
⑤ finish_goal       → 触发独立 review 轮次 → 产出 review 卡片（对照①的验收标准逐项 ✓/✗）→ 结束
```

## 核心组件

### SpecTool（submit_spec 工具）

- **文件**：`internal/eino/spec_tool.go`
- **功能**：agent 提交需求规格和验收标准，结构体持有供后续 review 轮次引用
- **参数**：`requirement`（需求描述）+ `acceptance_criteria`（验收标准数组）
- **产物**：`Artifact{Type:"requirement", Title:"需求规格", Requirement:..., AcceptanceCriteria:...}`
- **状态追踪**：`GetRequirement()` / `GetAcceptanceCriteria()` / `HasSpec()`，线程安全（sync.Mutex）

### runReviewRound（独立验收审查轮次）

- **文件**：`internal/eino/react_agent.go`
- **触发**：workflow 模式下 finish_goal 调用后，在"情况0"分支中执行
- **机制**：仿 `runSummaryRound`——构建 review 请求消息（验收审查员 system prompt + 需求 + 验收标准 + 过程消息）→ 不绑定工具 → Stream 输出
- **输出**：review 文本以 content step 流式推送，结束后解析为 `Artifact{Type:"review", Title:"验收报告", AcceptanceCriteria:[{pass/fail/pending}], ReviewSummary:...}`
- **解析**：`buildReviewArtifact` 从 review 文本中按 pass/fail/✓/✗ 标记匹配验收标准项

### workflow system prompt 注入

- **位置**：`taskSystemPrompt` 函数，`workflow` 参数非空时注入
- **内容**：5 阶段强制顺序指令，验收标准是硬性指标，完成后调 finish_goal
- **与 goal 互斥**：workflow 非空时替代 goalSection

### 工具注册

- **BuildTaskTools** 增加 `workflow string` 参数，返回 `*SpecTool`
- workflow 非空时：
  - `autoApprove = true`（复用 goal 逻辑）
  - 创建 `SpecTool` 并加入工具列表
  - 创建 `FinishGoalTool` 并加入工具列表
  - `PlanTool` 强制启用（不受全局 `task_plan_enabled` 开关限制）

### Artifact 类型扩展

- `Artifact` struct 新增字段：`Requirement`、`AcceptanceCriteria []AcceptItem`、`ReviewSummary`
- `AcceptItem` struct：`{ Content, Status (pass|fail|pending), Detail }`
- 前端 `ArtifactCard.vue` 新增 `requirement` 和 `review` 两种卡片渲染

## 数据模型

- **不新增字段**：workflow 模式复用 `Conversation.Goal` 字段存需求描述
- `Conversation.Mode` 新增 `workflow` 值
- `StreamTaskRequest` 新增 `Workflow string` 字段

## 前端交互

- InputArea mode-picker 增加第四个选项"工作流"（🎯 图标）
- workflow 模式下显示工作目录 + 需求输入按钮（复用 goal 按钮的 popover UI）
- 发送时：`workflow: goal.value, goal: ''`（复用 goal ref，字段名区分）
- ChatView header badge 显示"工作流模式"
- workflow 复用 task 渲染路径（TaskMessageItem），requirement/review 卡片在 plan 卡片之后渲染

## 权限策略

复用 goal 模式策略：
- `autoApprove=true`：跳过普通危险命令确认
- `criticalBashPatterns`：极危险命令（rm -rf /、sudo、mkfs、dd、chmod 777 /、fork bomb）始终确认

## 涉及文件

### 后端
| 文件 | 变更 |
|------|------|
| `internal/eino/artifact.go` | Artifact struct 扩展 + AcceptItem 类型 |
| `internal/eino/spec_tool.go` | **新文件** — submit_spec 工具 |
| `internal/eino/react_agent.go` | taskSystemPrompt 增加 workflow 参数 + runReviewRound + buildReviewArtifact + parseReviewStatus + RunTaskAgent 增加 specTool 参数 |
| `internal/eino/tool_registry.go` | BuildTaskTools 增加 workflow 参数 + 返回 SpecTool |
| `internal/handler/task.go` | StreamTaskRequest 增加 Workflow + 传透 + collectArtifact 支持 requirement/review |

### 前端
| 文件 | 变更 |
|------|------|
| `frontend/src/components/InputArea.vue` | ChatMode 增加 workflow + mode-picker + 需求按钮 + sendTask/stop 适配 |
| `frontend/src/components/ArtifactCard.vue` | requirement + review 卡片渲染 + 样式 |
| `frontend/src/components/TaskMessageItem.vue` | specArtifacts computed + 渲染位置 |
| `frontend/src/components/ChatView.vue` | isWorkflowMode + header badge + 空状态 |
| `frontend/src/utils/artifacts.ts` | AcceptItem 类型 + Artifact 扩展 + 去重 + splitTaskArtifacts specs 分类 |
| `frontend/wailsjs/go/models.ts` | StreamTaskRequest 增加 workflow |

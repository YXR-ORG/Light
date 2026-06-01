# Agent Sidebar + MCP Input + Param Snapshot Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将智能体选择移至侧边栏，输入栏新增 MCP 多选，并在 Message 中记录每次发送时的智能体和 MCP 快照。

**Architecture:**
- DB: `Conversation` 新增 `agent_id`；`Message` 新增 `agent_id` + `mcp_server_ids`（JSON 数组字符串）
- 后端: `SendMessageRequest` 携带 `agent_id` + `mcp_server_ids`，`StreamChat` 写入 Message 快照；新增 `SetAgent` handler
- 前端: Sidebar 搜索框下方加智能体横向滚动条；InputArea 移除星形按钮，加 MCP 多选弹出面板

**Tech Stack:** Go/GORM/SQLite, Vue 3 + TypeScript, Wails v2

---

### Task 1: DB Schema + Migration

**Files:**
- Modify: `internal/storage/models.go`
- Modify: `internal/storage/conversation.go`

- [ ] **Step 1: 在 Conversation 加 AgentID 字段**

在 `internal/storage/models.go` 的 `Conversation` struct 中，`SystemPrompt` 字段后加：

```go
AgentID      string    `gorm:"size:36;default:''" json:"agent_id"`
MCPServerIDs string    `gorm:"type:text;default:''" json:"mcp_server_ids"` // JSON []string
```

- [ ] **Step 2: 在 Message 加快照字段**

在 `Message` struct 的 `Attachments` 字段后加：

```go
AgentID      string    `gorm:"size:36;default:''" json:"agent_id"`
MCPServerIDs string    `gorm:"type:text;default:''" json:"mcp_server_ids"` // JSON []string
```

- [ ] **Step 3: 确认 AutoMigrate 覆盖新字段**

`AutoMigrate` 已包含 `&Conversation{}` 和 `&Message{}`，GORM 会自动 ALTER TABLE 加列，无需额外改动。

- [ ] **Step 4: 在 conversation.go 加 SetAgent 存储函数**

在 `internal/storage/conversation.go` 中新增：

```go
// SetAgent 更新对话的智能体和 system_prompt
func SetAgent(convID, agentID, systemPrompt string) error {
    return db.Model(&Conversation{}).
        Where("id = ?", convID).
        Updates(map[string]interface{}{
            "agent_id":      agentID,
            "system_prompt": systemPrompt,
        }).Error
}
```

- [ ] **Step 5: 构建确认无编译错误**

```bash
cd /Volumes/samsungssd/code/temp/wails客户端 && go build ./...
```
Expected: 无输出（编译成功）

- [ ] **Step 6: Commit**

```bash
git add internal/storage/models.go internal/storage/conversation.go
git commit -m "feat: add agent_id and mcp_server_ids snapshot fields to Conversation and Message"
```

---

### Task 2: 后端 Handler 变更

**Files:**
- Modify: `internal/handler/chat.go`
- Modify: `internal/handler/conversation.go`（新增 SetAgent）

- [ ] **Step 1: SendMessageRequest 加字段**

在 `internal/handler/chat.go` 的 `SendMessageRequest` struct 中加：

```go
AgentID      string   `json:"agent_id"`
MCPServerIDs []string `json:"mcp_server_ids"`
```

- [ ] **Step 2: StreamChat 中按 mcp_server_ids 过滤 MCP 工具**

在 `StreamChat` 的 `loadMCPTools` 调用处，将现有的"加载全部启用 MCP"改为"只加载请求中指定的 MCP"。

找到 `allTools := h.loadMCPTools(ctx)` 这一行，替换为：

```go
allTools := h.loadSelectedMCPTools(ctx, req.MCPServerIDs)
```

新增方法（放在 `loadMCPTools` 下方）：

```go
// loadSelectedMCPTools 只加载 selectedIDs 中指定的 MCP 服务器工具。
// 若 selectedIDs 为空，不加载任何 MCP 工具。
func (h *ChatHandler) loadSelectedMCPTools(ctx context.Context, selectedIDs []string) []tool.BaseTool {
    if len(selectedIDs) == 0 {
        return nil
    }
    servers, err := storage.ListMCPServers()
    if err != nil {
        slog.Warn("loadSelectedMCPTools: list servers failed", "error", err)
        return nil
    }
    idSet := make(map[string]bool, len(selectedIDs))
    for _, id := range selectedIDs {
        idSet[id] = true
    }
    var allTools []tool.BaseTool
    for _, srv := range servers {
        if !srv.Enabled || !idSet[srv.ID] {
            continue
        }
        allTools = append(allTools, h.connectAndGetTools(ctx, srv)...)
    }
    return allTools
}
```

- [ ] **Step 3: StreamChat 保存 Message 快照**

找到 `storage.SaveMessage(req.ConversationID, "user", ...)` 调用，在其后加：

```go
// 更新对话的 agent_id 快照（每次发送时同步）
if req.AgentID != "" {
    mcpJSON, _ := json.Marshal(req.MCPServerIDs)
    _ = db.Model(&storage.Conversation{}).
        Where("id = ?", req.ConversationID).
        Updates(map[string]interface{}{"mcp_server_ids": string(mcpJSON)}).Error
}
```

实际上更简洁的做法是在 `SaveMessage` 签名里加字段。修改 `internal/storage/conversation.go` 中的 `SaveMessage`：

找到现有签名：
```go
func SaveMessage(convID, role, content, thinking, toolResult string, attachments ...string) (*Message, error) {
```

改为：
```go
func SaveMessage(convID, role, content, thinking, toolResult, agentID, mcpServerIDs string, attachments ...string) (*Message, error) {
```

在函数体内 `msg := &Message{...}` 中加：
```go
AgentID:      agentID,
MCPServerIDs: mcpServerIDs,
```

- [ ] **Step 4: 修复所有 SaveMessage 调用处**

`chat.go` 中有两处调用：

用户消息调用改为：
```go
mcpJSON, _ := json.Marshal(req.MCPServerIDs)
storage.SaveMessage(req.ConversationID, "user", req.Content, "", "", req.AgentID, string(mcpJSON), attachmentsMeta)
```

助手消息调用改为：
```go
storage.SaveMessage(req.ConversationID, "assistant", fullContent, fullThinking, "", "", "", "")
```

- [ ] **Step 5: 在 conversation.go 的 ConversationHandler 加 SetAgent**

在 `internal/handler/conversation.go` 中新增：

```go
// SetAgent 设置对话的智能体（更新 system_prompt + agent_id）
func (h *ConversationHandler) SetAgent(convID, agentID string) error {
    agent, err := storage.GetAgent(agentID)
    if err != nil {
        return fmt.Errorf("agent not found: %w", err)
    }
    return storage.SetAgent(convID, agentID, agent.SystemPrompt)
}
```

需要在 `internal/storage/agent.go`（或 models.go）确认有 `GetAgent(id string)` 函数，若无则新增：

```go
func GetAgent(id string) (*Agent, error) {
    var a Agent
    return &a, db.First(&a, "id = ?", id).Error
}
```

- [ ] **Step 6: 构建确认**

```bash
cd /Volumes/samsungssd/code/temp/wails客户端 && go build ./...
```
Expected: 无输出

- [ ] **Step 7: 重新生成 Wails bindings**

```bash
cd /Volumes/samsungssd/code/temp/wails客户端 && wails generate module
```

- [ ] **Step 8: Commit**

```bash
git add internal/storage/ internal/handler/
git commit -m "feat: SendMessageRequest carries agent_id/mcp_server_ids, snapshot saved to Message"
```

---

### Task 3: Sidebar 智能体选择条

**Files:**
- Modify: `frontend/src/components/Sidebar.vue`
- Modify: `frontend/src/stores/chat.ts`（加 activeAgentId）

- [ ] **Step 1: chat store 加 activeAgentId**

在 `frontend/src/stores/chat.ts` 中加状态和 action：

```ts
activeAgentId: null as string | null,

setActiveAgent(id: string | null) {
  this.activeAgentId = id
},
```

- [ ] **Step 2: Sidebar script 加智能体加载逻辑**

在 `Sidebar.vue` `<script setup>` 中加：

```ts
import { ListAgents } from '../../wailsjs/go/handler/AgentHandler'
import { SetAgent } from '../../wailsjs/go/handler/ConversationHandler'
import type { storage } from '../../wailsjs/go/models'

const agents = ref<storage.Agent[]>([])

onMounted(async () => {
  agents.value = await ListAgents().catch(() => [])
})

async function selectAgent(agent: storage.Agent) {
  store.setActiveAgent(agent.id)
  if (store.currentConvId) {
    await SetAgent(store.currentConvId, agent.id).catch(console.error)
  }
}
```

- [ ] **Step 3: Sidebar template 加智能体选择条**

在 `.search-wrap` 之后、`.conv-list` 之前插入：

```html
<!-- 智能体选择条 -->
<div v-if="agents.length > 0" class="agent-bar">
  <button
    v-for="a in agents"
    :key="a.id"
    class="agent-chip"
    :class="{ active: store.activeAgentId === a.id }"
    :title="a.description"
    @click="selectAgent(a)"
  >
    <span class="agent-chip-icon">{{ a.icon }}</span>
    <span class="agent-chip-name">{{ a.name }}</span>
  </button>
</div>
```

- [ ] **Step 4: Sidebar style 加样式**

```css
.agent-bar {
  display: flex;
  gap: var(--space-1);
  padding: 0 var(--space-3) var(--space-2);
  overflow-x: auto;
  scrollbar-width: none;
  flex-shrink: 0;
}
.agent-bar::-webkit-scrollbar { display: none; }

.agent-chip {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 3px var(--space-2);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-full);
  background: transparent;
  cursor: pointer;
  white-space: nowrap;
  font-size: var(--text-xs);
  color: var(--color-text-2);
  font-family: var(--font-body);
  transition: border-color var(--duration-fast) var(--ease-out),
              background var(--duration-fast) var(--ease-out),
              color var(--duration-fast) var(--ease-out);
}
.agent-chip:hover {
  border-color: var(--color-accent);
  color: var(--color-accent);
  background: var(--color-accent-soft);
}
.agent-chip.active {
  border-color: var(--color-accent);
  background: var(--color-accent-soft);
  color: var(--color-accent);
  font-weight: 500;
}
.agent-chip-icon { font-size: 13px; line-height: 1; }
.agent-chip-name { max-width: 64px; overflow: hidden; text-overflow: ellipsis; }
```

- [ ] **Step 5: 切换对话时同步 activeAgentId**

在 `selectConv` 函数中，加载消息后同步智能体状态：

```ts
async function selectConv(id: string) {
  store.setCurrentConv(id)
  const msgs = await GetMessages(id)
  store.setMessages(msgs)
  // 同步当前对话的 agent_id
  const conv = store.conversations.find(c => c.id === id)
  store.setActiveAgent((conv as any)?.agent_id || null)
}
```

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/Sidebar.vue frontend/src/stores/chat.ts
git commit -m "feat: add agent selector bar to sidebar"
```

---

### Task 4: InputArea MCP 多选 + 移除智能体按钮

**Files:**
- Modify: `frontend/src/components/InputArea.vue`

- [ ] **Step 1: 移除 SkillsPanel 相关代码（script）**

删除以下 import 和变量：
```ts
// 删除这行 import
import SkillsPanel from './SkillsPanel.vue'
// 删除这个变量
const showSkills = ref(false)
// 删除 toggleSkills 函数
function toggleSkills() { ... }
```

在 `onClickOutside` 中删除 `showSkills` 相关逻辑。
在 `send()` 中删除 `showSkills.value = false`。

- [ ] **Step 2: 加 MCP 多选状态和加载逻辑**

```ts
import { List as ListMCPServers } from '../../wailsjs/go/handler/MCPHandler'

const showMCPPicker = ref(false)
const availableMCPs = ref<storage.MCPServer[]>([])
const selectedMCPIDs = ref<string[]>([])

async function loadMCPs() {
  availableMCPs.value = await ListMCPServers().catch(() => [])
}

function toggleMCPID(id: string) {
  const idx = selectedMCPIDs.value.indexOf(id)
  if (idx >= 0) selectedMCPIDs.value.splice(idx, 1)
  else selectedMCPIDs.value.push(id)
}

function toggleMCPPicker() {
  showMCPPicker.value = !showMCPPicker.value
  showModelPicker.value = false
  showSkillPicker.value = false
  if (showMCPPicker.value) loadMCPs()
}
```

- [ ] **Step 3: send() 携带 agent_id 和 mcp_server_ids**

在 `StreamChat(...)` 调用的 request 对象中加：

```ts
agent_id: store.activeAgentId ?? '',
mcp_server_ids: selectedMCPIDs.value,
```

需要从 chat store import `activeAgentId`（已在 Task 3 加入）。

- [ ] **Step 4: template 移除星形按钮，加 MCP 按钮**

删除：
```html
<!-- 技能按钮 -->
<button class="btn-icon" :class="{ 'btn-icon--active': showSkills }" @click="toggleSkills" title="技能">
  <svg ...星形图标... />
</button>
```

删除：
```html
<!-- Skills 面板 -->
<transition name="slide-up">
  <div v-if="showSkills" class="popup-panel">
    <SkillsPanel />
  </div>
</transition>
```

在 skill-picker 按钮之前加 MCP 按钮：

```html
<!-- MCP 选择按钮 -->
<button
  class="btn-mcp-picker"
  :class="{ active: showMCPPicker || selectedMCPIDs.length > 0 }"
  @click="toggleMCPPicker"
  title="选择 MCP 工具"
>
  <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
    <path d="M12 2L2 7l10 5 10-5-10-5z"/>
    <path d="M2 17l10 5 10-5"/>
    <path d="M2 12l10 5 10-5"/>
  </svg>
  <span v-if="selectedMCPIDs.length > 0" class="mcp-count">{{ selectedMCPIDs.length }}</span>
</button>
```

- [ ] **Step 5: template 加 MCP 选择面板**

在 skill-picker transition 之后加：

```html
<!-- MCP 选择面板 -->
<transition name="slide-up">
  <div v-if="showMCPPicker" class="mcp-picker">
    <div class="mcp-picker-header">
      <span class="mcp-picker-title">选择 MCP 工具</span>
      <span class="mcp-picker-hint">多选，发送时启用</span>
    </div>
    <div v-if="availableMCPs.filter(m => m.enabled).length === 0" class="model-empty">
      请先在「设置 → MCP」中添加并启用 MCP 服务器
    </div>
    <div v-else class="mcp-picker-list">
      <button
        v-for="m in availableMCPs.filter(s => s.enabled)"
        :key="m.id"
        class="mcp-picker-item"
        :class="{ active: selectedMCPIDs.includes(m.id) }"
        @click="toggleMCPID(m.id)"
      >
        <span class="mcp-picker-type">{{ m.type.toUpperCase() }}</span>
        <div class="mcp-picker-info">
          <span class="mcp-picker-name">{{ m.name }}</span>
          <span class="mcp-picker-addr">{{ m.type === 'sse' ? m.url : m.command }}</span>
        </div>
        <span v-if="selectedMCPIDs.includes(m.id)" class="mcp-picker-check">
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><polyline points="20 6 9 17 4 12"/></svg>
        </span>
      </button>
    </div>
    <div v-if="selectedMCPIDs.length > 0" class="mcp-picker-footer">
      <button class="skill-clear-btn" @click="selectedMCPIDs = []">清除选择</button>
    </div>
  </div>
</transition>
```

- [ ] **Step 6: style 加 MCP picker 样式**

```css
.btn-mcp-picker {
  display: flex; align-items: center; gap: 3px;
  height: 28px; padding: 0 var(--space-2);
  border: 1px solid var(--color-border); border-radius: var(--radius-md);
  background: transparent; cursor: pointer;
  color: var(--color-text-3);
  transition: border-color var(--duration-fast) var(--ease-out),
              color var(--duration-fast) var(--ease-out),
              background var(--duration-fast) var(--ease-out);
}
.btn-mcp-picker:hover, .btn-mcp-picker.active {
  border-color: var(--color-accent); color: var(--color-accent);
  background: var(--color-accent-soft);
}
.mcp-count {
  font-size: 10px; font-weight: 700;
  background: var(--color-accent); color: #fff;
  border-radius: var(--radius-full); padding: 0 4px;
  min-width: 14px; text-align: center;
}

.mcp-picker {
  position: absolute;
  bottom: calc(100% - var(--space-5));
  right: calc(var(--space-6) + 160px);
  z-index: 100;
  margin-bottom: var(--space-2);
  width: 240px;
  background: var(--color-paper);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-lg);
  overflow: hidden;
}
.mcp-picker-header {
  padding: var(--space-2) var(--space-3);
  font-size: var(--text-xs); font-weight: 600; color: var(--color-text-2);
  background: var(--color-paper-2); border-bottom: 1px solid var(--color-border);
  display: flex; align-items: baseline; gap: var(--space-2);
}
.mcp-picker-hint { font-weight: 400; color: var(--color-text-3); }
.mcp-picker-list {
  max-height: 240px; overflow-y: auto;
  padding: var(--space-1); display: flex; flex-direction: column; gap: 2px;
}
.mcp-picker-item {
  display: flex; align-items: center; gap: var(--space-2);
  padding: var(--space-2); border: none; border-radius: var(--radius-sm);
  background: transparent; cursor: pointer; text-align: left; width: 100%;
  transition: background var(--duration-fast) var(--ease-out);
}
.mcp-picker-item:hover { background: var(--color-paper-3); }
.mcp-picker-item.active { background: var(--color-accent-soft); }
.mcp-picker-type {
  font-size: 10px; font-weight: 700; padding: 1px 5px;
  background: var(--color-accent-soft); color: var(--color-accent);
  border-radius: var(--radius-sm); flex-shrink: 0;
}
.mcp-picker-info { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 1px; }
.mcp-picker-name { font-size: var(--text-xs); font-weight: 500; color: var(--color-text); }
.mcp-picker-addr {
  font-size: 10px; color: var(--color-text-3);
  overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
}
.mcp-picker-check {
  width: 16px; height: 16px; border: 1px solid var(--color-border);
  border-radius: var(--radius-sm); display: flex; align-items: center;
  justify-content: center; flex-shrink: 0; color: var(--color-accent);
}
.mcp-picker-item.active .mcp-picker-check {
  background: var(--color-accent); border-color: var(--color-accent); color: #fff;
}
.mcp-picker-footer { padding: var(--space-2) var(--space-3); border-top: 1px solid var(--color-border); }
```

- [ ] **Step 7: onClickOutside 加 MCP picker 关闭逻辑**

在 `onClickOutside` 中加：

```ts
const mcpPicker = document.querySelector('.mcp-picker')
const mcpBtn = document.querySelector('.btn-mcp-picker')
if (showMCPPicker.value && mcpPicker && !mcpPicker.contains(e.target as Node) && !mcpBtn?.contains(e.target as Node)) {
  showMCPPicker.value = false
}
```

- [ ] **Step 8: Commit**

```bash
git add frontend/src/components/InputArea.vue
git commit -m "feat: replace agent button with MCP multi-select in input area"
```

---

### Task 5: 构建验证

- [ ] **Step 1: 全量构建**

```bash
cd /Volumes/samsungssd/code/temp/wails客户端 && wails build -ldflags "-X main.Version=1.0.1" 2>&1
```
Expected: `Built '...Light.app...' in XX.XXXs.`

- [ ] **Step 2: 打开应用验证**

```bash
open /Volumes/samsungssd/code/temp/wails客户端/build/bin/Light.app
```

验证清单：
- [ ] 侧边栏搜索框下方出现智能体横向选择条
- [ ] 点击智能体 chip 高亮，切换对话后状态同步
- [ ] 输入栏无星形"技能"按钮
- [ ] 输入栏有 MCP 按钮，点击弹出 MCP 列表（多选）
- [ ] 发送消息后，数据库 messages 表中 agent_id / mcp_server_ids 有值

- [ ] **Step 3: Final commit**

```bash
git add -A && git commit -m "feat: agent sidebar + MCP input + param snapshot complete"
```

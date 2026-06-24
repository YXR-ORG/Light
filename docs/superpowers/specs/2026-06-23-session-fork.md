# 会话分叉（Fork） — 设计与实现

> 版本：v1.0
> 日期：2026-06-23
> 状态：已实现
> 关联：`docs/superpowers/specs/2026-06-22-session-compression.md`（会话高级管理：压缩 + 分叉）

---

## 0. 背景

会话分叉是会话高级管理的第二个功能。当用户在对话进行到一半时，想从某个回答处「另走一条路」探索不同方向，但又不想丢失当前对话进展，就需要分叉能力：**从指定消息处复制一份新会话，原会话不动，新会话从分叉点继续**。

典型场景：
- AI 给了一个方案 A，用户想同时探索方案 B，又不想让对话变成 A→B 杂糅
- 长对话走到岔路口，想保留两条路径分别继续
- 某个回答后想换个 system prompt / agent 重新展开

---

## 1. 设计决策

| 决策点 | 选择 | 理由 |
|--------|------|------|
| 触发位置 | 仅 assistant 消息处 | 复用现有 `MessageItem.vue` hover 操作区，user 消息无需改动 UI；语义清晰（在回答结束后分叉） |
| task 模式 | 禁用分叉 | task 模式有独立 `work_dir`（真实文件目录），分叉需处理目录复制，复杂度过高且语义不清。task 模式走独立 `TaskMessageItem.vue`，天然不显示分叉按钮 |
| 来源记录 | 记录 `parent_conv_id` + `fork_from_msg_id` | 支持追溯谱系，会话列表用角标标识分叉会话 |
| 复制范围 | 到分叉点为止（含该 assistant group） | 语义正确：分叉点之后的对话不带入新会话 |
| 重生成版本 | 仅当前查看的版本 | 新会话从干净状态开始，`gen_index` 重置为 0；用户不需要在新会话继续比较旧版本 |

---

## 2. 数据模型变更

### Conversation 表新增字段

```go
ParentConvID  string `gorm:"size:36;default:''" json:"parent_conv_id"`    // 分叉来源会话 ID（空=原始会话）
ForkFromMsgID string `gorm:"size:36;default:''" json:"fork_from_msg_id"`  // 分叉来源消息 ID
```

- 通过 GORM AutoMigrate 自动加列，无需手动迁移
- 原始会话两字段为空；分叉会话记录来源

### 消息复制规则

新会话的消息由 `ForkConversation()` 在单事务内创建：

1. 拉取源会话全部消息（`created_at` 升序）
2. 定位 `forkFromMsgID` 所在的 `GenerationGroupID`
3. 找到该 group 在消息序列中的最后位置（group 内可能有多条重生成版本）
4. 到该位置为止，按 group 去重（仅保留每组 `gen_index` 最大的一条），与 `GetLatestMessages` 逻辑一致
5. 为每条消息 INSERT 新行：新 message ID，新 conversation ID，`GenerationGroupID` 重置为自身 ID，`GenIndex`=0
6. 完整复制 Content/Thinking/ToolCalls/ToolResult/Attachments/Artifacts/AgentID/MCPServerIDs/Mode/KnowledgeBaseID

### 标题生成

- 原标题 + `" (分叉)"`
- 若原标题已含 `"(分叉)"`，则加序号：`"原标题 (分叉 2)"`、`"(分叉 3)"`…（查 DB 去重）

---

## 3. API

### 后端

**`internal/storage/conversation.go`**
```go
func ForkConversation(srcConvID, forkFromMsgID string) (*Conversation, error)
```
单事务完成会话创建 + 消息复制。调用方负责校验 task 模式。

**`internal/handler/conversation.go`**
```go
func (h *ConversationHandler) Fork(convID, msgID string) (*storage.Conversation, error)
```
- 校验源会话存在且 `mode != "task"`
- 调用 `storage.ForkConversation`
- 返回新会话对象

### 前端绑定

`wails dev` 自动重新生成 `ConversationHandler.d.ts/.js`。前端：
```ts
import { Fork, GetMessages } from '../../wailsjs/go/handler/ConversationHandler'
```

---

## 4. 前端实现

### MessageItem.vue — 分叉按钮

在 assistant 消息 hover 操作区（复制、重新生成按钮旁）增加「从此分叉」按钮：
- git-fork 图标（两个圆点 + 连线）
- emit `fork` 事件
- 复用 `showActions && !streaming` 条件，流式进行中不显示

### MessageList.vue — 分叉逻辑

```ts
async function handleFork(msg: storage.Message) {
  if (store.streaming) return
  const conv = store.conversations.find(c => c.id === store.currentConvId)
  if (!conv || conv.mode === 'task') return  // 双保险
  const newConv = await Fork(store.currentConvId, msg.id)
  store.setConversations([newConv, ...store.conversations])
  store.setCurrentConv(newConv.id)
  store.setMessages(await GetMessages(newConv.id))
}
```
- 分叉后新会话插入列表顶部并自动选中
- `activeGenIndex` 版本切换状态随 `store.currentConvId` watch 自动重置

### ConversationItem.vue — 来源角标

分叉会话（`parent_conv_id` 非空）在标题旁显示小分支图标角标，title="分叉会话"。轻量标识，不展开谱系树。

---

## 5. 受影响文件

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `internal/storage/models.go` | 修改 | Conversation 增加 2 字段 |
| `internal/storage/conversation.go` | 新增函数 | `ForkConversation` |
| `internal/handler/conversation.go` | 新增方法 | `Fork` |
| `frontend/src/components/MessageItem.vue` | 修改 | 分叉按钮 + emit |
| `frontend/src/components/MessageList.vue` | 修改 | `handleFork` 逻辑 |
| `frontend/src/components/ConversationItem.vue` | 修改 | 来源角标 |
| `frontend/wailsjs/go/models.ts` | 自动生成 | Conversation 字段 |
| `frontend/wailsjs/go/handler/ConversationHandler.{d.ts,js}` | 自动生成 | Fork 导出 |

---

## 6. 技术约束

- **task 模式不支持分叉**：后端 `Fork` 方法返回错误，前端双保险（`conv.mode === 'task'` 判断 + task 模式走独立 `TaskMessageItem.vue` 天然不显示按钮）
- **流式进行中禁用分叉**：复用 `showActions && !streaming` 条件
- **原会话不受影响**：分叉是纯新增操作（INSERT），不修改源会话任何数据
- **压缩中间件兼容**：分叉复制的是原始消息（未经中间件处理），新会话首次发送时中间件重新生效，行为正确
- **GORM AutoMigrate**：新增字段有 `default:''`，存量数据自动补空值，无需手动迁移

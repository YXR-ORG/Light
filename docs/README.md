# Light 文档中心

> 这是项目技术文档的唯一入口。新增或查阅文档时先从这里开始，避免 README、规格、设计记录和待办列表互相重复。

---

## 从这里开始

| 你要做什么 | 先看 | 说明 |
|------------|------|------|
| 了解当前产品和架构 | [`SPECS.md`](SPECS.md) | 当前权威规格：三种对话模式、数据库实体关系、核心约束 |
| 开发另一个 Wails 桌面软件 | [`WAILS_DEV_GUIDE.md`](WAILS_DEV_GUIDE.md) | 跨项目开发手册：Wails、图标、打包、CI、常见坑 |
| 维护知识库功能 | [`KNOWLEDGE_BASE_IMPL.md`](KNOWLEDGE_BASE_IMPL.md) | 知识库实现：SQLite、FTS5、向量、RRF、摘要层 |
| 排查知识库历史问题 | [`KNOWLEDGE_BASE_POSTMORTEM.md`](KNOWLEDGE_BASE_POSTMORTEM.md) | 知识库问题复盘与根因记录 |
| 看任务模式设计 | [`superpowers/specs/2026-06-03-task-mode-design.md`](superpowers/specs/2026-06-03-task-mode-design.md) | task 模式功能边界、工具集、安全执行 |
| 看 task ReAct 架构 | [`superpowers/specs/2026-06-04-task-react-architecture.md`](superpowers/specs/2026-06-04-task-react-architecture.md) | eino ReAct Agent、流式事件、与 chat/knowledge 的区别 |
| 看 task 产物机制 | [`superpowers/specs/2026-06-04-task-artifact-mechanism.md`](superpowers/specs/2026-06-04-task-artifact-mechanism.md) | Artifact、plan/file 分桶、历史恢复、自适应执行 |
| 看待办和未来规划 | [`TODO.md`](TODO.md) | 已完成、暂缓和未来规划；不是架构权威来源 |

---

## 权威关系

### 当前事实优先级

1. 源码和数据库模型是最终事实。
2. [`SPECS.md`](SPECS.md) 是当前产品/架构的权威文字说明。
3. 专题实现文档说明具体子系统，如知识库、task 模式、Wails 打包。
4. `docs/superpowers/specs/` 与 `docs/superpowers/plans/` 是历史设计与执行记录，不一定代表最新实现。

### 文档边界

| 文档 | 应该写什么 | 不应该写什么 |
|------|------------|--------------|
| `README.md` | 面向用户：功能、下载、快速开始、Changelog | 深层架构、长篇设计细节 |
| `docs/README.md` | 技术文档入口、目录、权威关系 | 具体实现细节 |
| `docs/SPECS.md` | 当前产品规格、架构边界、ER、关键约束 | 每次调试日志、历史计划过程 |
| `docs/WAILS_DEV_GUIDE.md` | 可复用到新 Wails 项目的工程经验 | Light 业务规则 |
| `docs/KNOWLEDGE_BASE_IMPL.md` | 知识库实现细节 | task 模式和普通 chat 细节 |
| `docs/TODO.md` | 规划、暂缓项、完成状态 | 当前架构事实的唯一来源 |
| `docs/superpowers/specs/` | 阶段性设计记录、专题深挖 | 替代 `SPECS.md` 成为入口 |

---

## 当前核心架构索引

### 三种对话模式

Light 有三种执行机制不同的模式：

| 模式 | 引擎 | 主要文档 |
|------|------|----------|
| `chat` | 手写 tool loop | [`SPECS.md`](SPECS.md) |
| `knowledge` | 手写 tool loop + `search_knowledge` | [`KNOWLEDGE_BASE_IMPL.md`](KNOWLEDGE_BASE_IMPL.md) |
| `task` | eino ReAct Agent | [`task ReAct 架构`](superpowers/specs/2026-06-04-task-react-architecture.md) |

不要把 task 模式和 chat/knowledge 的工具循环混为一谈。task 由 eino ReAct Agent 管理 tool call、observation 回灌和执行步数。

### 数据存储

| 数据 | 位置 | 说明 |
|------|------|------|
| 对话、消息、模型、MCP、技能、设置、知识库元数据 | `~/.wails-chat/chat.db` | 主库，GORM AutoMigrate |
| 知识库文档、chunks、FTS5、vectors、summaries | `~/.wails-chat/knowledgebases/{id}/kb.db` | 每个知识库独立 SQLite |
| task 附件元数据 | `messages.attachments` | JSON，不保存大文件本体 |
| task 产物 | `messages.artifacts` | JSON `[]Artifact`，plan 和 file 必须按 type 分桶 |

详细 ER 图见 [`SPECS.md#五数据库实体关系er`](SPECS.md#五数据库实体关系er)。

### Task Artifact 规则

历史和实时 task 消息必须使用同一套分桶规则：

| Artifact type | UI 区域 |
|---------------|---------|
| `plan` | 回复顶部「执行计划」卡片 |
| `file` | 「本次涉及的文件」区域 |
| 其他类型 | 预留「相关产物」区域 |

禁止用 `type !== 'plan'` 判断文件区。文件区只能用 `type === 'file'`。

---

## 文档维护规则

1. 新增长期有效的架构事实，优先更新 [`SPECS.md`](SPECS.md)。
2. 新增某个子系统的实现细节，更新对应专题文档。
3. 新增阶段性设计方案，可以放入 `docs/superpowers/specs/`，但必须在本入口登记。
4. 完成或废弃 TODO，更新 [`TODO.md`](TODO.md) 的状态和日期。
5. 根目录 `README.md` 只写用户视角内容和简短 changelog，复杂技术内容链接到本入口。
6. 修改数据库实体、artifact 类型、对话模式边界时，必须同步更新 `SPECS.md`。

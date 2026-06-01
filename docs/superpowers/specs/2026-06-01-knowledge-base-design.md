# Knowledge Base (知识库) Feature Spec — v1.0.3

## Overview

在 Light 客户端中引入知识库功能，允许用户将本地文档（PDF、Word、Excel、文本、代码等）导入知识库，并在对话中以"知识模式"进行基于文档的问答。LLM 通过 function call（`search_knowledge` tool）主动检索知识库，回复下方展示参考来源。

---

## 1. 对话模式切换

### 1.1 输入框左侧布局变更

当前输入框左侧只有附件按钮。1.0.3 改为三个元素从左到右排列：

```
[模式选择器] [模型选择器] [附件按钮]   →   [文本输入区]   →   [搜索][Skills][MCP][停止/发送]
```

### 1.2 模式选择器

一个紧凑的下拉按钮，显示当前模式图标+名称，点击展开：

| 模式 | 图标 | 说明 | 状态 |
|------|------|------|------|
| 问答 | 💬 | 默认对话模式，当前行为不变 | 可用 |
| 知识 | 📚 | 挂载知识库，LLM 通过 tool call 检索 | 可用 |
| 任务 | ⚡ | 自主 Agent 模式 | 灰显，标注"即将推出" |

### 1.3 知识库选择器

当模式切换为"知识"时，模式选择器右侧紧跟一个知识库单选下拉（与智能体选择器样式一致）：

- 列出所有已创建的知识库（名称 + 文档数角标）
- 单选，选中后高亮
- 若无知识库，显示"请先在设置中创建知识库"并链接到设置

### 1.4 状态持久化

`SendMessageRequest` 新增字段：
```go
Mode          string `json:"mode"`           // "chat" | "knowledge"
KnowledgeBase string `json:"knowledge_base"` // kb_id，mode=knowledge 时有效
```

`Message` 表新增快照字段：
```go
Mode          string `gorm:"size:16;default:'chat'" json:"mode"`
KnowledgeBase string `gorm:"size:36;default:''" json:"knowledge_base"`
```

---

## 2. 知识库数据架构

### 2.1 文件系统布局

```
~/Library/Application Support/Light/   (macOS)
%APPDATA%\Light\                        (Windows)
  knowledgebases/
    {kb_id}/
      kb.db          ← 文档元数据 + 分块 + FTS5 + 向量（预留）
      files/
        {doc_id}_{filename}  ← 原始文件备份
```

### 2.2 主数据库新增表

```sql
CREATE TABLE knowledge_bases (
  id          TEXT PRIMARY KEY,
  name        TEXT NOT NULL,
  description TEXT DEFAULT '',
  doc_count   INTEGER DEFAULT 0,
  created_at  DATETIME,
  updated_at  DATETIME
);
```

GORM model 加入 `AutoMigrate`。

### 2.3 每个 kb.db 的表结构

```sql
-- 文档元数据
CREATE TABLE documents (
  id          TEXT PRIMARY KEY,
  name        TEXT NOT NULL,        -- 原始文件名
  mime_type   TEXT NOT NULL,
  size        INTEGER DEFAULT 0,    -- 字节数
  chunk_count INTEGER DEFAULT 0,
  status      TEXT DEFAULT 'pending', -- pending|processing|ready|error
  error       TEXT DEFAULT '',
  created_at  DATETIME
);

-- 文本分块
CREATE TABLE chunks (
  id          TEXT PRIMARY KEY,
  doc_id      TEXT NOT NULL REFERENCES documents(id),
  content     TEXT NOT NULL,
  chunk_index INTEGER NOT NULL,     -- 块在文档中的顺序
  page        INTEGER DEFAULT 0,    -- PDF 页码，其他文件为 0
  created_at  DATETIME
);

-- FTS5 全文索引（content 表，不重复存储）
CREATE VIRTUAL TABLE chunks_fts USING fts5(
  content,
  content='chunks',
  content_rowid='rowid',
  tokenize='unicode61'
);

-- 向量表（1.0.3 建表但不填充，预留给 1.0.4）
CREATE TABLE vectors (
  id       TEXT PRIMARY KEY,
  chunk_id TEXT NOT NULL REFERENCES chunks(id),
  embedding BLOB  -- sqlite-vec float32[384]
);
```

---

## 3. 文档处理 Pipeline

### 3.1 支持的文件类型与解析库

| 类型 | 扩展名 | 解析方式 |
|------|--------|----------|
| 纯文本 | .txt .md .csv .json .yaml .xml .html | 直接读取 UTF-8 |
| 代码 | .go .py .js .ts .java .sql .sh 等 | 直接读取 |
| PDF | .pdf | `pdfcpu`（纯 Go，无 CGO）提取文本层；扫描版 PDF 跳过图片页，标注"部分内容不可读" |
| Word | .docx | `go-docx`（纯 Go）提取段落文本 |
| Excel | .xlsx .xls | `excelize`（纯 Go）逐 sheet 逐行转为 TSV 文本 |
| PowerPoint | .pptx | 1.0.3 不支持，推迟到 1.0.4 |

不支持的格式返回错误，前端显示"不支持的文件类型"。

### 3.2 分块策略

- 分块大小：512 token（按字符估算：中文 ≈ 1 char/token，英文 ≈ 4 chars/token，取保守值 400 字符/块）
- 重叠：64 字符（保留上下文连续性）
- 段落优先：优先在段落边界（`\n\n`）分块，避免在句子中间截断
- 最小块：< 50 字符的块合并到前一块

### 3.3 处理流程

```
上传文件
  → 保存原始文件到 files/{doc_id}_{filename}
  → 插入 documents 记录（status=pending）
  → 异步 goroutine：
      解析文本
      → 分块
      → 批量写入 chunks
      → 重建 FTS5 索引（INSERT INTO chunks_fts(chunks_fts) VALUES('rebuild')）
      → 更新 documents.status=ready, chunk_count=N
      → 更新主库 knowledge_bases.doc_count
  → 前端轮询文档状态（每 2 秒，直到 ready 或 error）
```

### 3.4 Embedding（1.0.3 实现）

- 模型：`all-MiniLM-L6-v2`，ONNX 格式，384 维，文件约 22MB
- 运行时：`onnxruntime-go`（CGO，需要 onnxruntime 动态库随 app 打包）
- 模型文件位置：`build/models/all-MiniLM-L6-v2.onnx`，打包进 app bundle
- 向量化时机：文档 status=ready 后，后台异步向量化所有 chunks，写入 vectors 表
- 向量化进度：`documents` 表加 `vec_count` 字段，前端可选展示

**打包要求（macOS）**：
- `libonnxruntime.dylib` 放入 `Light.app/Contents/Frameworks/`
- `all-MiniLM-L6-v2.onnx` 放入 `Light.app/Contents/Resources/models/`

**打包要求（Windows）**：
- `onnxruntime.dll` 放入 exe 同目录
- 模型文件放入 `resources/models/`

---

## 4. 知识检索：KnowledgeSearchTool

### 4.1 Tool 定义

实现 eino `InvokableTool` 接口：

```go
// internal/eino/knowledge_tool.go
type KnowledgeSearchTool struct {
    kbID   string
    kbPath string // kb.db 路径
}

// Info 返回 tool schema
func (t *KnowledgeSearchTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
    return &schema.ToolInfo{
        Name: "search_knowledge",
        Desc: "在知识库中搜索与问题相关的文档片段。当需要查找资料、回答基于文档的问题时，必须先调用此工具。",
        ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
            "query": {Type: schema.String, Desc: "搜索查询词，用自然语言描述要查找的内容", Required: true},
            "top_k": {Type: schema.Integer, Desc: "返回结果数量，默认 5，最大 10", Required: false},
        }),
    }, nil
}

// InvokableRun 执行检索，返回 JSON 格式的片段列表
func (t *KnowledgeSearchTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error)
```

### 4.2 检索逻辑

1.0.3 只做 FTS5 检索（向量检索 1.0.4 加入）：

```sql
SELECT c.id, c.content, c.chunk_index, c.page, d.name as doc_name
FROM chunks_fts
JOIN chunks c ON chunks_fts.rowid = c.rowid
JOIN documents d ON c.doc_id = d.id
WHERE chunks_fts MATCH ?
ORDER BY rank
LIMIT ?
```

返回 JSON：
```json
{
  "results": [
    {
      "doc_name": "高数.pdf",
      "page": 12,
      "chunk_index": 3,
      "content": "...相关片段内容..."
    }
  ],
  "total": 3
}
```

### 4.3 与 StreamChat 集成

当 `req.Mode == "knowledge"` 且 `req.KnowledgeBase != ""` 时，在 `StreamChat` 中：

```go
if req.Mode == "knowledge" && req.KnowledgeBase != "" {
    kbPath := getKBPath(req.KnowledgeBase)
    allTools = append(allTools, eino.NewKnowledgeSearchTool(req.KnowledgeBase, kbPath))
}
```

System prompt 追加：
```
You have access to a knowledge base search tool. When answering questions that require document knowledge, you MUST call search_knowledge first. Always cite your sources.
```

### 4.4 来源展示

Tool call 结果存入 `Message.ToolResult`（已有字段）。前端 `MessageItem.vue` 检测 `tool_result` 中 `search_knowledge` 的调用结果，在助手回复下方渲染来源卡片：

```
📄 参考来源
  高数.pdf · 第12页  "...片段预览..."
  线代.pdf · 第3页   "...片段预览..."
```

---

## 5. 知识库管理 UI

### 5.1 设置新增"知识库"标签

在 `SettingsDialog.vue` 的 `MainTab` 中加入 `'knowledge'`，图标 📚，位置在 `'skills'` 之后。

### 5.2 知识库广场（列表视图）

- 知识库卡片列表：名称、描述、文档数、创建时间
- "新建知识库"按钮（弹出表单：名称必填，描述可选）
- 点击知识库卡片进入详情视图

### 5.3 知识库详情视图

- 顶部：知识库名称 + 返回按钮
- 文档列表：文件名、大小、状态（处理中/就绪/错误）、分块数、操作（删除）
- "上传文件"按钮：点击后由**后端调用 `runtime.OpenMultipleFilesDialog`** 弹出系统原生文件选择框，返回路径列表后后端直接读取，无需经过前端传输
- 状态轮询：处理中的文档每 2 秒刷新状态

### 5.4 文件上传方式（Wails 原生）

**不使用 base64 / HTML `<input type="file">`**，改为 Wails 原生对话框方案：

```
前端点击"上传文件"
  → 调用 KnowledgeHandler.PickAndUploadDocuments(kbID)
  → 后端 runtime.OpenMultipleFilesDialog(ctx, OpenDialogOptions{Filters: [...]})
  → 返回 []string 文件路径
  → 后端逐个 os.ReadFile(path) 处理
  → 返回 []KBDocument（含初始状态）
  → 前端开始轮询状态
```

优点：零 IPC 传输开销，50MB 文件无压力，是 Wails 客户端标准做法。

Handler 签名：
```go
// PickAndUploadDocuments 弹出系统文件选择框，选中后直接处理，无需前端传文件内容
func (h *KnowledgeHandler) PickAndUploadDocuments(kbID string) ([]KBDocument, error)
```

支持的文件类型过滤器（传给 OpenMultipleFilesDialog）：
```go
Filters: []runtime.FileFilter{
    {DisplayName: "文档文件", Pattern: "*.pdf;*.docx;*.xlsx;*.txt;*.md;*.csv;*.json;*.yaml;*.xml;*.html"},
    {DisplayName: "代码文件", Pattern: "*.go;*.py;*.js;*.ts;*.java;*.sql;*.sh;*.rs;*.cpp;*.c"},
    {DisplayName: "所有文件", Pattern: "*"},
}
```

### 5.5 文件大小限制

单文件最大 50MB，后端读取后检查 `len(data)` 超出则跳过并记录错误，不中断其他文件处理。

---

## 6. 后端 Handler 接口

新增 `KnowledgeHandler`（`internal/handler/knowledge.go`）：

```go
type KnowledgeHandler struct{}

// ListKnowledgeBases 列出所有知识库
func (h *KnowledgeHandler) ListKnowledgeBases() ([]storage.KnowledgeBase, error)

// CreateKnowledgeBase 新建知识库
func (h *KnowledgeHandler) CreateKnowledgeBase(name, description string) (*storage.KnowledgeBase, error)

// DeleteKnowledgeBase 删除知识库（含 kb.db 和 files/）
func (h *KnowledgeHandler) DeleteKnowledgeBase(id string) error

// ListDocuments 列出知识库内的文档
func (h *KnowledgeHandler) ListDocuments(kbID string) ([]KBDocument, error)

// PickAndUploadDocuments 弹出系统文件选择框，选中后直接读取处理，无需前端传文件内容
func (h *KnowledgeHandler) PickAndUploadDocuments(kbID string) ([]KBDocument, error)

// DeleteDocument 删除文档（含原始文件和所有 chunks）
func (h *KnowledgeHandler) DeleteDocument(kbID, docID string) error

// GetDocumentStatus 查询文档处理状态（前端轮询用）
func (h *KnowledgeHandler) GetDocumentStatus(kbID, docID string) (string, error)
```

`KBDocument` DTO：
```go
type KBDocument struct {
    ID         string `json:"id"`
    Name       string `json:"name"`
    MimeType   string `json:"mime_type"`
    Size       int64  `json:"size"`
    ChunkCount int    `json:"chunk_count"`
    Status     string `json:"status"`  // pending|processing|ready|error
    Error      string `json:"error"`
    CreatedAt  string `json:"created_at"`
}
```

---

## 7. 降级与错误处理

| 场景 | 处理方式 |
|------|----------|
| 文档解析失败（加密 PDF、损坏文件） | status=error，error 字段记录原因，前端显示错误图标 |
| FTS5 检索无结果 | tool 返回 `{"results":[],"total":0}`，LLM 自行告知用户未找到相关内容 |
| kb.db 文件损坏 | 检索返回错误，StreamChat 降级为普通问答模式，前端提示"知识库暂时不可用" |
| onnxruntime 加载失败 | 向量化跳过，仅使用 FTS5 检索，不影响基本功能 |
| 上传文件超过 50MB | 前端拦截，不发送请求 |
| 知识库被删除时有对话引用 | 检索返回空结果，不报错 |

---

## 8. 不在 1.0.3 范围内

- 向量检索（sqlite-vec）：表结构预留，1.0.4 实现
- 混合检索 RRF 融合：依赖向量检索，1.0.4 实现
- PowerPoint (.pptx) 解析：视依赖复杂度，可能推迟
- 知识库导出/导入
- 知识库与 WebDAV 备份集成
- 任务模式（自主 Agent）
- 多知识库同时挂载（当前单选）

---

## 9. 实现步骤（分阶段）

**阶段一：数据层**
1. 主库 `knowledge_bases` 表 + GORM model
2. `KBStore`：管理 kb.db 的创建/打开/关闭，封装 chunks/FTS5 操作
3. 文档解析器（文本、PDF、Word、Excel）

**阶段二：处理 Pipeline**
4. `UploadDocument` handler：保存文件 → 异步解析分块 → 写 FTS5
5. 文档状态轮询接口

**阶段三：检索与集成**
6. `KnowledgeSearchTool` 实现 eino `InvokableTool`
7. `StreamChat` 集成：mode=knowledge 时绑定 tool
8. Embedding pipeline（onnxruntime-go + all-MiniLM-L6-v2）

**阶段四：前端**
9. 输入框模式选择器 + 知识库单选器
10. 设置"知识库"标签 + 列表/详情视图
11. `MessageItem.vue` 来源卡片渲染

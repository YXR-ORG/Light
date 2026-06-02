# 知识库功能实现文档

> 版本：v1.0.4 | 更新：2026-06-02

---

## 一、架构总览

```
用户操作（前端）
      ↓
KnowledgeHandler（handler/knowledge.go）
      ↓
kb.Store（kb/store.go）         ← 每个知识库一个独立 kb.db（SQLite）
  ├── FTS5 trigram 全文检索
  ├── vectors 向量表（float32 BLOB）
  └── summaries 摘要表

对话时（mode=knowledge）
      ↓
KnowledgeSearchTool（eino/knowledge_tool.go）← eino InvokableTool
      ↓
Store.Search()  ← 四路融合：FTS5-AND + 向量余弦 + FTS5-OR + LIKE
```

---

## 二、数据存储

### 2.1 目录结构

```
~/.wails-chat/
  chat.db                           ← 主数据库（GORM/SQLite）
    └── knowledge_bases 表          ← 知识库元数据（id, name, doc_count…）
  knowledgebases/
    {kb_id}/
      kb.db                         ← 每个知识库独立数据库
      files/
        {doc_id}_{filename}         ← 原始文件备份
```

### 2.2 主数据库 knowledge_bases 表

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

GORM model：`storage.KnowledgeBase`，由 `AutoMigrate` 自动创建。

### 2.3 知识库数据库 kb.db 表结构

```sql
-- 文档元数据
CREATE TABLE documents (
  id          TEXT PRIMARY KEY,           -- newID()（UnixNano 字符串）
  name        TEXT NOT NULL,              -- 原始文件名
  mime_type   TEXT NOT NULL,
  size        INTEGER DEFAULT 0,          -- 字节数
  chunk_count INTEGER DEFAULT 0,
  status      TEXT DEFAULT 'pending',     -- pending|processing|ready|error
  error       TEXT DEFAULT '',
  created_at  DATETIME
);

-- 文本分块
CREATE TABLE chunks (
  id          TEXT PRIMARY KEY,           -- {doc_id}_{chunk_index}
  doc_id      TEXT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
  content     TEXT NOT NULL,
  chunk_index INTEGER NOT NULL,
  created_at  DATETIME
);

-- FTS5 全文索引（trigram tokenizer，独立存储）
CREATE VIRTUAL TABLE chunks_fts USING fts5(
  chunk_id    UNINDEXED,
  doc_name    UNINDEXED,
  chunk_index UNINDEXED,
  content,
  tokenize='trigram'
);

-- 向量表（float32 BLOB，384 维）
CREATE TABLE vectors (
  id       TEXT PRIMARY KEY,              -- v_{chunk_id}
  chunk_id TEXT NOT NULL REFERENCES chunks(id) ON DELETE CASCADE,
  embedding BLOB
);

-- 文档摘要（LLM 异步生成）
CREATE TABLE summaries (
  doc_id       TEXT PRIMARY KEY REFERENCES documents(id) ON DELETE CASCADE,
  doc_name     TEXT NOT NULL,
  summary      TEXT NOT NULL,             -- 100 字以内摘要
  key_entities TEXT DEFAULT '[]',         -- JSON 数组，如 ["张嘎","奶奶"]
  created_at   DATETIME
);
```

---

## 三、创建知识库

**调用链：**
```
前端 CreateKnowledgeBase(name, desc)
  → KnowledgeHandler.CreateKnowledgeBase()
      → storage.CreateKnowledgeBase()    ← 写主库 knowledge_bases 表
      → os.MkdirAll(kbDir(kb.ID))       ← 创建文件系统目录
```

**代码位置：** `internal/handler/knowledge.go:47`

kb.db 此时尚未创建，首次调用 `kb.GetStore()` 时才会触发 `openStore` + `migrate`。

---

## 四、上传文件与文档处理 Pipeline

### 4.1 上传流程

```
前端点击「上传文件」
  → KnowledgeHandler.PickAndUploadDocuments(kbID)
      → runtime.OpenMultipleFilesDialog()    ← Wails 系统原生文件选择框
      → 逐文件：
          os.Stat → 大小检查（>50MB 跳过）
          mime.TypeByExtension（扩展名转小写）
          store.InsertDocument(name, mimeType, size)  ← status=pending
          os.ReadFile → os.WriteFile(files/{docID}_{name})
          go processDocument(store, kbID, docID, name, data)  ← 异步
      → 返回 []KBDocument（status=processing）
  → 前端每 2 秒轮询 GetDocumentStatus
```

**文件大小限制：** 50MB，超过则跳过并记录日志。
**代码位置：** `internal/handler/knowledge.go:115`

### 4.2 文档处理（processDocument）

```go
// internal/handler/knowledge.go:174
func processDocument(store, kbID, docID, name, data) {
    // Step 1: 解析文本
    text, err := kb.ParseText(data, name)   // 根据扩展名选解析器

    // Step 2: 分块
    chunks := kb.SplitChunks(text)

    // Step 3: 写入 chunks + FTS5
    store.InsertChunks(docID, chunks)       // 同一事务写 chunks + chunks_fts

    // Step 4: 更新状态
    store.UpdateDocumentStatus(docID, "ready", "", len(chunks))
    storage.IncrKBDocCount(kbID, 1)

    // Step 5: 异步摘要生成（不阻塞）
    go generateAndStoreSummary(store, docID, name, text)

    // Step 6: 异步向量化（不阻塞）
    go kb.VectorizeDocument(store, docID, chunks)
}
```

### 4.3 文档解析（ParseText）

**代码位置：** `internal/kb/parser.go`

| 扩展名 | 解析库 | 说明 |
|--------|--------|------|
| `.pdf` | `github.com/dslipak/pdf` v0.0.2 | 逐页提取文本层，扫描版返回空 |
| `.docx` | `github.com/nguyenthenguyen/docx` | 提取段落文本 |
| `.xlsx` `.xls` | `github.com/xuri/excelize/v2` v2.9.1 | 逐 sheet/行转 TSV 文本 |
| 其他 | 直接 `string(data)` | txt/md/csv/json/yaml/xml/html/代码等 |

### 4.4 文本分块（SplitChunks）

**代码位置：** `internal/kb/chunker.go`

| 参数 | 值 | 说明 |
|------|----|------|
| chunkSize | 400 字符 | 约 512 tokens（中文保守估算） |
| chunkOverlap | 64 字符 | 相邻块重叠，保留上下文连续性 |
| minChunkSize | 50 字符 | 小于此值合并到前一块 |

**策略：**
1. 按 `\n\n` 段落边界分割，优先保留段落完整性
2. 段落 >400 字符时按字符强制切割，加 64 字符重叠
3. <50 字符的小块合并到前一块

---

## 五、FTS5 全文索引

### 5.1 选择 trigram tokenizer 的原因

`unicode61`（默认）对中文分词不可靠：`孙小仙` 被切成 `孙` + `小仙` 两个 token，搜索 `"孙小仙"` 短语返回 0 结果。

**trigram** 把每段文本切成所有可能的 3 字符子串，任意子串都能被搜到：
- `孙小仙` → `孙小仙`（3字正好）直接入索引 ✅
- `张嘎`（2字）不满足 trigram 最小长度，用 LIKE 补充 ✅

### 5.2 独立存储模式

FTS5 **不使用** `content=` 外部表模式（该模式在事务内 rebuild 看不到未提交数据，导致索引为空）。改为独立存储：`InsertChunks` 在同一事务内同时写 `chunks` 和 `chunks_fts`。

### 5.3 自动迁移

`migrate()` 检测 `chunks_fts` 的建表 SQL，若含 `content=` 或 `unicode61` 关键字，自动 DROP 并重建，同时从 `chunks` 表重新填充索引（用户无感知）。

---

## 六、向量检索

### 6.1 Embedding 模型

| 项目 | 值 |
|------|----|
| 模型 | `all-MiniLM-L6-v2` |
| 维度 | 384 维 float32 |
| 文件 | ONNX 格式，约 90MB |
| 运行时 | `hugot` v0.7.4 纯 Go backend（GoMLX），零 CGO，完全离线 |
| 模型路径 | 按优先级：app bundle → `build/models/` → `~/.cache/chroma/`（开发机自动命中）|

**代码位置：** `internal/kb/embedder.go`

```go
// 懒加载全局单例
func getEmbedder() (*pipelines.FeatureExtractionPipeline, error) {
    embedderOnce.Do(func() {
        sess, _ := hugot.NewGoSession(ctx)
        p, _ := hugot.NewPipeline(sess, hugot.FeatureExtractionConfig{
            ModelPath: dir,
        })
        embedderPipeline = p
    })
    return embedderPipeline, embedderErr
}

// L2 归一化 → 余弦相似度 = 点积
func Embed(texts []string) ([][]float32, error)
```

### 6.2 向量化 Pipeline

**代码位置：** `internal/kb/vectorize.go`

```
文档 status=ready 后，异步 goroutine：
  VectorizeDocument(store, docID, chunks)
    → 按 32 条/批调用 Embed()
    → L2 归一化
    → Float32SliceToBytes() 序列化为 little-endian BLOB
    → store.InsertVectors() 写入 vectors 表
```

**批大小：** 32 条（hugot 纯 Go backend 推荐值）

### 6.3 向量检索

**代码位置：** `internal/kb/vectorize.go:VectorSearch`

```
query → Embed([query]) → queryVec（384 维，已 L2 归一化）
  → SELECT * FROM vectors JOIN chunks JOIN documents
  → 逐行 BytesToFloat32Slice → CosineSim(queryVec, vec)
  → sort by score DESC → topK
```

全量扫描（适合本地小规模知识库，无需 ANN 索引）。

---

## 七、摘要索引

### 7.1 生成流程

**代码位置：** `internal/handler/knowledge.go:generateAndStoreSummary`

```
文档就绪后，异步调用：
  取前 3000 字符
  → 获取第一个 enabled 的 LLM provider + 第一个 model
  → ChatService.Generate()（非流式，30 秒超时）
  → prompt：请分析文档，返回 {"summary": "...", "key_entities": [...]}
  → JSON 解析（失败时用原始回复作摘要）
  → store.UpsertSummary(docID, docName, summary, entitiesJSON)
```

**降级：** LLM 不可用时摘要生成失败，仅记录日志，不影响 FTS5 和向量检索。

### 7.2 摘要表用途

- 快速定位相关文档（两阶段检索第一阶段）
- 重建索引时提供文档上下文
- 未来：搜索时先匹配摘要层，再搜 chunk 层（TODO，待完整实现）

---

## 八、四路融合检索

**代码位置：** `internal/kb/store.go:Search`

```
Search(query, topK) → []SearchResult

路径1：FTS5 AND 查询（精度高）
  buildFTS5Query(query) → andQuery（长词用双引号包裹，用空格连接）
  chunks_fts MATCH andQuery

路径2：向量余弦相似度（语义）
  VectorSearch(query, topK) → 全量扫描 + cosine sort

路径3：FTS5 OR 查询（召回补充）
  chunks_fts MATCH orQuery（各词 OR 连接）

路径4：LIKE 短词补充
  <3字符的词（如"张嘎"）用 chunks LIKE '%张嘎%'

四路结果去重合并，取前 topK 条
```

**topK：** 默认 10，上限 20。

### 8.1 FTS5 查询构建

```go
// buildFTS5Query 区分长词和短词
func buildFTS5Query(query string) (andQuery, orQuery string, shortTerms []string) {
    // >=3 字符 → FTS5 短语查询（双引号包裹）
    // <3 字符 → LIKE 补充
}
```

---

## 九、对话集成

### 9.1 KnowledgeSearchTool

**代码位置：** `internal/eino/knowledge_tool.go`

实现 eino `InvokableTool` 接口，供 LLM 通过 function call 调用：

```go
// Tool schema
Name: "search_knowledge"
Params:
  query  string  必填 // 每次只搜一个核心概念
  top_k  int     可选 // 默认 10，最大 20

// 返回 JSON
{
  "results": [
    {"doc_name": "小兵张嘎.txt", "chunk_index": 3, "content": "..."}
  ],
  "total": 5
}
```

### 9.2 StreamChat 集成

**代码位置：** `internal/handler/chat.go:303`

```go
// mode=knowledge 时注入工具
if req.Mode == "knowledge" && req.KnowledgeBaseID != "" {
    kbPath := kbDirForChat(req.KnowledgeBaseID)
    kbTool, _ := eino.NewKnowledgeSearchTool(req.KnowledgeBaseID, kbPath)
    allTools = append(allTools, kbTool)
}

// system prompt 强制规则
systemContent += `
你有 search_knowledge 工具可以查询知识库。严格遵守：
- 回答前必须先搜索，禁止凭空编造
- 涉及多个实体时，必须对每个实体单独搜索
- 每次 query 只包含一个核心词（人名/地名/事件）
- 搜到结果后综合推理，明确标注信息来源
- 结果不足时换关键词重试，最多 3 轮`
```

---

## 十、重建索引

### 10.1 触发方式

设置 → 知识库 → 点击知识库详情 → 右上角「重建索引」按钮。

### 10.2 重建流程

**代码位置：** `internal/handler/knowledge.go:RebuildIndex`

```
RebuildIndex(kbID) → 立即返回，异步执行

goroutine：
  store.AllReadyDocuments()   ← 只处理 status=ready 的文档
  for each doc:
    1. store.DeleteVectorsForDoc(docID)
    2. store.AllChunksForDoc(docID)
    3. kb.VectorizeDocument(store, docID, chunks)
    4. store.DeleteSummaryForDoc(docID)
    5. generateAndStoreSummary(store, docID, name, text)
    6. 推送事件 kb:rebuild:progress
  完成后推送 kb:rebuild:done
```

### 10.3 进度事件

**`kb:rebuild:progress`** 事件结构：
```json
{
  "kbID": "...",
  "docID": "...",
  "docName": "小兵张嘎_全文.txt",
  "step": "vectorizing" | "summarizing",
  "current": 1,
  "total": 2
}
```

**`kb:rebuild:done`** 事件结构：
```json
{
  "kbID": "...",
  "success": true,
  "message": "重建完成，共 2 个文档"
}
```

前端进度条宽度 = `current / total * 100%`，完成后 4 秒自动消失。

---

## 十一、降级策略

| 场景 | 处理方式 |
|------|---------|
| 模型文件不存在 | 向量化跳过，仅 FTS5+LIKE 检索，不影响基本功能 |
| LLM 不可用 | 摘要生成失败，记录日志，不影响 FTS5 和向量检索 |
| FTS5 搜索失败 | 返回空结果，LLM 告知用户未找到相关内容 |
| kb.db 损坏 | `GetStore` 返回错误，StreamChat 降级为普通对话 |
| 文件解析失败 | status=error，前端显示错误图标，其他文件继续处理 |
| 文件超 50MB | 跳过，记录日志 |

---

## 十二、关键依赖

| 包 | 版本 | 用途 |
|----|------|------|
| `mattn/go-sqlite3` | v1.14.22 | SQLite（需 `-tags fts5` 启用 FTS5 扩展） |
| `knights-analytics/hugot` | v0.7.4 | 纯 Go ONNX 推理（embedding pipeline） |
| `dslipak/pdf` | v0.0.2 | PDF 文本提取 |
| `nguyenthenguyen/docx` | commit | Word 文本提取 |
| `xuri/excelize/v2` | v2.9.1 | Excel/TSV 转换 |
| `wailsapp/wails/v2` | v2.12.0 | 原生文件对话框 |
| `cloudwego/eino` | v0.9.2 | InvokableTool 接口 |

**重要：** 构建时必须加 `-tags fts5`，否则 FTS5 扩展不可用：
```bash
wails build -tags fts5 -ldflags "-X main.Version=1.0.4"
# 或
make build
```

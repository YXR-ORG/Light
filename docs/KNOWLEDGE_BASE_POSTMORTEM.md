# 知识库功能 — 问题复盘与架构分析

> 记录时间：2026-06-02 | 版本：v1.0.3

---

## 一、遇到的问题与根因定位

### 问题 1：上传文件提示 `no such module: fts5`

**现象**：上传文档时后端报错，文档无法入库。

**根因**：`mattn/go-sqlite3` 默认编译不启用 FTS5 扩展，需要在编译时传 `-tags fts5` 才会在 C 层加 `-DSQLITE_ENABLE_FTS5`。

**修复**：所有构建命令统一加 `-tags fts5`，固化到 `Makefile` 和 CI workflow。

---

### 问题 2：FTS5 索引为空，搜索始终返回 0 结果

**现象**：文档上传成功，但知识库问答完全没有召回任何内容。

**根因**：FTS5 建表时使用了 `content='chunks'` 外部表模式（content table mode）。这种模式下 FTS5 自身不存数据，查询时从 `chunks` 表读取。`InsertChunks` 在事务内执行 `INSERT INTO chunks_fts(chunks_fts) VALUES('rebuild')`，但 `rebuild` 命令在事务提交前执行，此时 `chunks` 数据尚未提交，FTS5 读到的是空表，索引重建为空。

**修复**：放弃 `content=` 外部表模式，改为 FTS5 独立存储模式。`InsertChunks` 在同一事务内同时写 `chunks` 和 `chunks_fts`，彻底消除同步问题。

---

### 问题 3：`unicode61` tokenizer 对中文分词不可靠

**现象**：`孙小仙`（公平国往事的角色）搜索返回 0 结果，但 `小仙`、`公主` 可以搜到。`张嘎`（2字）也搜不到。

**根因**：`unicode61` tokenizer 按 Unicode 字符类别分词，对中文的切分行为不可预测。`孙小仙` 被切成 `孙` + `小仙` 两个 token，搜索 `"孙小仙"`（短语查询）要求这3个字作为整体 token 出现，找不到匹配。`张嘎` 只有2个汉字，同样无法被正确索引。

**验证**：
```sql
-- unicode61 下
SELECT count(*) FROM chunks_fts WHERE chunks_fts MATCH '"孙小仙"';  -- 0
SELECT count(*) FROM chunks_fts WHERE chunks_fts MATCH '"小仙"';    -- 4（可以搜到）

-- trigram 下
SELECT count(*) FROM chunks_fts WHERE chunks_fts MATCH '孙小仙';    -- 43 ✅
SELECT count(*) FROM chunks_fts WHERE chunks_fts MATCH '张嘎子';    -- 18 ✅
```

**修复**：换用 `trigram` tokenizer。trigram 把每段文本切成所有可能的3字符子串，任意子串都能被搜到，对中文完全可靠。代价是索引体积约为 unicode61 的3-5倍，对本地知识库完全可接受。

对 `<3字符` 的短词（如"张嘎"），trigram 不建索引，额外用 `LIKE '%张嘎%'` 补充查询。

**自动迁移**：`migrate()` 检测到旧 schema 含 `unicode61` 关键字时，自动 DROP 并重建为 trigram，从 `chunks` 表重新填充索引，用户无感知。

---

### 问题 4：跨文档推理失败

**现象**：分别问两个文档的内容都能回答，但问"张嘎进入公平国会发生什么"、"孙小仙和张嘎有什么共同点"时，模型找不到相关内容。

**根因**：模型把多个实体混在一个 query 里搜索（如 `"张嘎 公平国会"`），FTS5 找不到同时包含这两个词的 chunk（它们在不同文档），返回 0。

**修复**：
1. `top_k` 默认值从 5 提升到 10，上限从 10 提升到 20
2. tool description 明确要求：跨文档问题必须对每个实体单独搜索，每次 query 只包含一个核心概念
3. system prompt 用中文明确规则：先搜索再回答，分实体搜索，标注来源

---

## 二、架构层面的深层分析

### 当前架构（Naive RAG）

```
用户问题
   ↓
LLM 自主决定搜什么（不可控）
   ↓
FTS5 关键词匹配（无语义理解）
   ↓
返回 top-k chunks
   ↓
LLM 综合回答
```

**本质局限**：检索质量完全依赖 LLM 的 query 生成质量，而检索层是关键词匹配，无法理解语义。"勇敢的少年"和"机智的小鬼"在语义上相同，但关键词匹配找不到关联。

### 问题的三个层次

| 层次 | 问题 | 当前解法 | 根本解法 |
|------|------|---------|---------|
| 检索层 | tokenizer 不可靠、索引为空 | trigram + 独立存储 | ✅ 已解决 |
| 策略层 | 模型 query 生成质量差 | system prompt 引导 | Query 改写 / HyDE |
| 语义层 | 关键词无法理解语义 | 无 | 向量检索 |

---

## 三、RAG 演进路径（TODO）

见 README.md `## 知识库 TODO` 章节。

---

## 四、数据库 Schema（当前）

```sql
-- 每个知识库一个独立的 kb.db，存储在：
-- ~/.wails-chat/knowledgebases/{kb_id}/kb.db

CREATE TABLE documents (
  id TEXT PRIMARY KEY,
  name TEXT, mime_type TEXT, size INTEGER,
  chunk_count INTEGER, status TEXT, error TEXT, created_at DATETIME
);

CREATE TABLE chunks (
  id TEXT PRIMARY KEY,
  doc_id TEXT REFERENCES documents(id) ON DELETE CASCADE,
  content TEXT, chunk_index INTEGER, created_at DATETIME
);

-- trigram tokenizer，独立存储模式
CREATE VIRTUAL TABLE chunks_fts USING fts5(
  chunk_id UNINDEXED, doc_name UNINDEXED, chunk_index UNINDEXED,
  content,
  tokenize='trigram'
);

-- 预留向量检索（v1.0.4+）
CREATE TABLE vectors (
  id TEXT PRIMARY KEY,
  chunk_id TEXT REFERENCES chunks(id) ON DELETE CASCADE,
  embedding BLOB
);
```

---

## 五、分块策略（当前）

- 段落优先：按空行分段，保留自然段落边界
- 块大小：400字符，重叠64字符
- 支持格式：TXT、MD、PDF（dslipak/pdf）、DOCX（nguyenthenguyen/docx）、Excel（excelize）

# 知识库查询预处理与同义词归一化

> 版本：v2.0.0 | 更新：2026-06-29

---

## 一、背景与问题

知识库检索质量优化——解决"库里有答案但搜不到"的痛点。

### 1.1 问题诊断

原有 `Search()` 入口对用户 query **零预处理**，原样丢给 FTS5 和向量检索。真实用户提问充满噪音：

| 噪音类型 | 示例 | 问题 |
|---------|------|------|
| 礼貌废话 | 请问、麻烦、谢谢 | 凑字数，干扰匹配 |
| 疑问模板 | 是什么、有哪些、怎么办 | 通用句式，无检索价值 |
| 口语助词 | 啊、呀、吧、呢 | 纯噪音 |
| 口语别称 | 爬高干活 / 高处作业 | 同义不同字，匹配失败 |

前三类稀释核心语义，第四类最致命——**用户用词和文档标准术语对不上时，四路检索全 miss**。

### 1.2 分词缺失的连锁影响

项目原本**无中文分词**，`buildFTS5Query` 只用 `strings.Fields`（按空格切）。中文 query 几乎不带空格，所以：

- FTS5 trigram 靠 3 字符滑窗硬匹配，核心词被切碎
- 停用词清洗无法按词操作（中文无词边界）
- 同义词归一化无法按词匹配（连续字符串里 `strings.Contains` 易误匹配）

**结论：分词是查询预处理能否生效的根基。**

---

## 二、整体方案

在 `Search()` 入口加一层 query 预处理，**不动四路检索和 RRF 主体**：

```
用户原始 query (如"请问新手夏天工地爬高干活要注意啥呀")
        │
        ▼
┌──────────────────────────────────────────┐
│  ExpandQuery  (internal/kb/query.go)     │
│                                          │
│  ① 停用词清洗  → "夏天工地爬高干活注意"      │
│  ② gse 分词    → ["夏天","工地","爬高","干活","注意"]
│  ③ 同义词归一化 → ["夏天","工地","高处作业","注意"]
│  ④ 输出 Cleaned + Terms + Mappings        │
└──────────────────────────────────────────┘
        │
        ├─→ FTS5/摘要检索：用归一化后 query（字面匹配需标准术语）
        └─→ 向量检索：用【原始 query】（保持语义完整性，分工互补）
```

### 2.1 关键设计决策

1. **只分词 query，不碰文档索引**
   - 文档侧已有 trigram FTS5（任意子串可搜，可靠），重建索引成本高
   - query 是实时输入，分词开销在检索路径上，毫秒级可接受
   - 风险隔离：分词只在 query 侧，出问题只影响检索精度，不破坏已建索引

2. **向量检索用原 query，FTS5/摘要用归一化 query**
   - 向量模型（all-MiniLM-L6-v2）靠语义，完整句子语义更稳
   - FTS5/摘要靠字面匹配，必须用归一化后的标准词
   - 两者分工互补：核心词精准锁定（FTS5），语义召回（向量）

3. **同义词表按知识库维度独立维护**
   - 存在每个 kb.db 的 `synonyms` 表，领域相关
   - 符合"每库独立 SQLite"的现有设计
   - 通用停用词表不硬编码行业词，垂直行业词由用户自维护

---

## 三、实现细节

### 3.1 分词器 `internal/kb/tokenizer.go`

使用 `go-ego/gse` v1.0.2（纯 Go，零 CGO，嵌入中文词典 ~4MB）。

- `sync.Once` 全局单例懒加载，首次 `NewEmbed("zh")` 约 50-100ms，之后无开销
- `Tokenize(text)`：CutSearch 搜索引擎模式，过滤单字、标点、符号
- `CutToQuery(text)`：分词后空格拼接，供 `buildFTS5Query` 的 `strings.Fields` 正确切分
- 分词不可用时降级为按空格切分，保持原有行为

```go
func Tokenize(text string) []string {
    seg, err := getSegmenter()
    if err != nil || seg == nil {
        return strings.Fields(text) // 降级
    }
    tokens := seg.CutSearch(text, true)
    // 过滤单字、标点...
}
```

### 3.2 查询预处理 `internal/kb/query.go`

#### 停用词清洗 `PreprocessQuery`

通用停用词表（礼貌废话、疑问模板、口语助词、场景口水），`strings.ReplaceAll` 删除。

- 清洗后为空（纯语气句）时回退原 query，避免空检索
- **只清洗检索用 query，不影响传给 LLM 的原始用户消息**

#### 同义词归一化 `ExpandQuery`

完整链路：清洗 → 分词 → 归一化。

```go
type ExpandedQuery struct {
    Cleaned    string        // 归一化后、空格拼接的 query
    Terms      []string      // 最终检索词列表
    HasSynonym bool          // 是否命中同义词
    Mappings   []SynonymPair // 命中的映射（透传给 LLM）
}
```

**多 token 短语匹配**：分词器可能把"爬高干活"切成 `["爬高","干活"]` 两 token。`applySynonyms` 采用贪心最长匹配——按 source 词数降序，先匹配长短语再匹配短词：

```
tokens: ["夏天","工地","爬高","干活","注意"]
synonyms: {"爬高干活"→"高处作业"}
结果: ["夏天","工地","高处作业","注意"]  ← "爬高"+"干活"合并替换
```

### 3.3 同义词表存储

每个 kb.db 新增 `synonyms` 表：

```sql
CREATE TABLE IF NOT EXISTS synonyms (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  source     TEXT NOT NULL UNIQUE,  -- 口语词，如 "爬高干活"
  target     TEXT NOT NULL,         -- 标准词，如 "高处作业"
  created_at DATETIME
);
```

- `loadSynonyms()` 带缓存（`sync.RWMutex` + dirty 标志），增删时失效
- `source` 在加载时分词成 `sourceParts`，与 query tokens 形式一致才能匹配
- 同时保留 `origSource`（原始录入形式），用于透传给 LLM 显示

### 3.4 Search 入口接入 `internal/kb/store.go`

```go
func (s *Store) Search(query string, topK int) ([]SearchResult, error) {
    // 阶段零：query 预处理
    expanded := s.ExpandQuery(query)
    retrievalQuery := expanded.Cleaned
    if strings.TrimSpace(retrievalQuery) == "" {
        retrievalQuery = query // 回退
    }
    // 阶段一摘要层、路径1/3 FTS5：用 retrievalQuery
    // 路径2 向量检索：用原始 query
}
```

---

## 四、关键缺陷修复：归一化信息透传给 LLM

### 4.1 问题现象

检索层把"孙彤"归一化成"孙小仙"并成功召回，但 **LLM 不知道这件事**——它看到检索返回的文档片段全是"孙小仙"、没有"孙彤"字样，在"禁止凭空编造/标注来源"规则下，判定"原文没有孙彤"，拒绝作答。

### 4.2 根因

检索结果返回给 LLM 的 JSON 只有 `results` 和 `total`，没有告知 query 被改写过。检索层做对了，但信息断在生成层。

### 4.3 修复：三处强化

**① 工具返回透传归一化信息** `SearchWithExpansion`

```go
type SearchResultWithExpansion struct {
    Results          []SearchResult
    Total            int
    OriginalQuery    string         // 原始 query
    RetrievalQuery   string         // 归一化后 query
    SynonymMappings  []SynonymPair  // 命中的映射
}
```

`knowledge_tool.go` 在返回 JSON 里加 `note` 字段提示 LLM：

```json
{
  "results": [...],
  "original_query": "孙彤",
  "retrieval_query": "孙小仙",
  "synonym_mappings": [{"source":"孙彤","target":"孙小仙"}],
  "note": "查询已做同义词归一化：「孙彤」→「孙小仙」...不得因原文未出现「孙彤」就断言找不到。"
}
```

**② 三处 system prompt 强化引导**

- `knowledge_tool.go` 工具 Desc：第 5 条规则说明 synonym_mappings 含义
- `chat.go` knowledge 模式 system prompt：加【同义词归一化】规则
- `react_agent.go` task 模式 system prompt：同上

核心：让 LLM 知道**同义词归一化是系统行为，用户词=标准词，检索到的就是用户问的实体，可以直接答**。

---

## 五、前端配置 UI

`KnowledgeConfig.vue` 在知识库详情视图文档列表下方，加可折叠的「同义词映射」面板：

- 折叠头：标题 + 数量徽标
- 展开后：说明文字 + `口语词 → 标准词` 输入框 + 添加按钮
- 列表展示已有映射，hover 显示删除按钮（直接删除，无 confirm 阻塞）

进入知识库时自动加载，增删后自动刷新。

---

## 六、与文章方案的差异

本方案参考《90% RAG 检索拉胯都是关键词提取拖后腿》的"关键词提纯"思路，但针对客户端通用工具定位做了调整：

| 文章做法 | 客户端调整 | 原因 |
|---------|-----------|------|
| 硬编码行业词典（建筑安全） | 通用停用词 + 知识库级用户自填映射表 | 客户端是通用工具，领域未知 |
| 核心/辅助词分两级权重 | 复用现有 RRF + 摘要加权 | 避免过度工程，小规模场景 RRF 已够 |
| 单路向量检索 | 保留四路，向量用原query、FTS5用归一化query | 发挥混合检索优势，分工互补 |
| 清洗后直接送生成 | **只清洗检索query，不动LLM输入** | 避免回答变生硬 |
| 无分词 | gse 中文分词（query 侧） | 中文无空格，分词是清洗/归一化生效的前提 |

---

## 七、测试覆盖

`internal/kb/query_test.go` + `test_helpers_test.go`：

- `TestPreprocessQuery`：停用词清洗各类型 + 空串回退
- `TestTokenize`：口语提问、标准术语、含标点的分词
- `TestApplySynonyms`：单 token 命中、多 token 短语命中、无命中
- `TestExpandQuery`：全链路（清洗→分词→归一化）
- `TestSearchWithSynonym`：端到端——同义词前后召回对比（核心价值验证）
- `TestSearchWithExpansion`：归一化信息透传（修复缺陷的回归测试）
- `TestSynonymCRUD`：增删查 + 缓存

`internal/eino/note_check_test.go`：验证工具返回 JSON 包含 note 字段。

端到端验证日志（`TestSearchWithSynonym`）：
```
before synonym: query=爬高干活注意 retrieval_query="爬高 干活 注意" results=0
after synonym:  query=爬高干活注意 retrieval_query="高处作业 注意" results=1  ← 命中
```

---

## 八、文件清单

| 文件 | 说明 |
|------|------|
| `internal/kb/tokenizer.go` | gse 分词器（懒加载单例 + Tokenize/CutToQuery） |
| `internal/kb/query.go` | 停用词清洗 + 同义词归一化 + ExpandQuery + CRUD |
| `internal/kb/store.go` | migrate 加 synonyms 表；Search 入口接入；SearchWithExpansion |
| `internal/eino/knowledge_tool.go` | 用 SearchWithExpansion，返回 note + mappings |
| `internal/eino/react_agent.go` | task 模式 system prompt 加同义词规则 |
| `internal/handler/chat.go` | knowledge 模式 system prompt 加同义词规则 |
| `internal/handler/knowledge.go` | 同义词 CRUD handler 方法 |
| `frontend/src/components/KnowledgeConfig.vue` | 同义词管理面板 |
| `internal/kb/query_test.go` | 单元测试 |
| `internal/eino/note_check_test.go` | note 透传测试 |

---

## 九、依赖

- `github.com/go-ego/gse` v1.0.2（纯 Go 中文分词，嵌入词典 ~4MB）
- `github.com/vcaesar/cedar` v0.30.0（gse 依赖）

构建：`wails build -tags fts5`（gse 默认嵌入中文词典，无需额外配置）。

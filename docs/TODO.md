# TODO 列表

> 最后更新：2026-06-02

所有待办、进行中、已完成的功能规划统一在此维护。

---

## 知识库

### ✅ TODO-KB-1：文档摘要索引（已完成 v1.0.4）

**问题**：FTS5 只能匹配关键词，无法理解"这篇文档讲的是什么"。文档量大时模型需要多次搜索才能定位到正确文档。

**实现**：
- 新增 `summaries` 表（`doc_id`, `summary`, `key_entities` JSON 数组）
- 文档就绪后异步调用已配置的 LLM，生成 100 字摘要 + 最多 10 个关键实体
- `Store` 提供 `UpsertSummary` / `SearchSummaries` / `GetAllSummaries`
- `ChatService.Generate()` 非流式方法供后台任务使用

**遗留**：摘要层目前已建表、已生成，但尚未接入 `Search()` 的两阶段检索路径（先搜摘要定位文档，再搜 chunk）。成本低，效果直接，是下一个小优化点。

---

### ✅ TODO-KB-2：向量检索 + 融合（已完成 v1.0.4）

**问题**：关键词检索无法理解语义，"勇敢的少年"和"机智的小鬼"语义相同但 FTS5 找不到关联。

**实现**：
- `hugot` v0.7.4 纯 Go backend + `all-MiniLM-L6-v2` ONNX（384 维，完全离线）
- 模型路径优先级：app bundle → `build/models/` → `~/.cache/chroma`（开发机自动命中）
- 文档就绪后批量向量化（32 条/批），写入 `vectors` 表
- `Search()` 升级为四路融合：FTS5-AND（精度）→ 向量余弦（语义）→ FTS5-OR（召回）→ LIKE（短词兜底）
- 向量检索不可用时自动降级为纯 FTS5

**遗留**：
- 现有文档需手动点「重建索引」按钮触发向量化（已有入口）
- 模型文件尚未打包进 app bundle（依赖开发机 chroma 缓存），正式发布前需处理
- 真正的 RRF（Reciprocal Rank Fusion）评分融合尚未实现，目前是简单去重合并

---

### ✅ TODO-KB-3：摘要层两阶段检索（已完成 v1.0.4）

**实现**：`Search()` 新增阶段一——先调用 `SearchSummaries(query)` 得到相关文档集合，后续四路检索结果按是否命中摘要过滤分为 `primary`（优先）和 `fallback`（兜底）两组，合并后返回 top-k。摘要层无命中时（文档摘要尚未生成）自动降级为全量检索，不影响功能。

---

### 🔮 TODO-KB-4：知识图谱 / 实体关系索引（长期，暂缓）

**背景**：上传文档后，LLM 抽取实体和关系，存入 SQLite 邻接表，检索时图遍历辅助定位相关 chunk。

**评估结论（2026-06-02）**：暂不实现。理由：

1. 现有四路融合检索已能覆盖大多数跨文档问题，摘要层和向量层的效果还没被充分验证
2. 通用场景下实体消歧是硬问题，抽取质量不稳定，可能引入噪声
3. KISS 原则——LLM 自身的推理能力可以弥补图谱缺失，只要 chunks 召回对了
4. 唯一真正的增量场景（实体关系链查询，如"和孙小仙有过冲突的人"）在个人知识库日常使用中频率极低

**重新评估条件**：当用户反馈出现"现有检索方案明确答不上来"的具体 case，再针对性设计。

**技术方向（备用）**：
- 每文档 1 次 LLM 调用抽取 `{entities, relations}` JSON（O(N)，非文档间两两比较）
- `entities` 表 + `relations` 表，SQLite 邻接表存储，无需图数据库
- 跨文档关联靠实体名字符串自动匹配，不做复杂消歧
- 检索前置图谱预处理：识别问题实体 → 图查询扩展 → 定向 chunk 检索

---

## 其他功能

### ✅ TODO-APP-1：embedding 模型打包进 app bundle（已完成 v1.0.4）

**实现**：
- 模型文件（`model.onnx`、`tokenizer.json`、`vocab.txt` 等）放入 `build/models/all-MiniLM-L6-v2/`
- `Makefile` 新增 `copy-models` target，`build` 后自动复制到 `build/bin/Light.app/Contents/Resources/models/`
- `embedder.go` 的路径探测改用 `os.Getwd()` 向上遍历，替代不可靠的相对深度路径，兼容 wails dev 和 go run
- `.gitignore` 排除 `*.onnx` 大文件（86MB 不进 git，需手动放置或 CI 下载）

**注意**：`make build` 会自动触发 `copy-models`；直接调用 `wails build` 不会，需手动执行 `make copy-models`。

---

### ✅ TODO-APP-2：RRF 评分融合（已完成 v1.2.2）

**实现**：`Store.Search()` 替换原来的去重追加逻辑，改用 Reciprocal Rank Fusion（k=60）：
- 四路各取 `topK×2` 候选，保证融合池充分
- 每路携带排名，`RRF(d) = Σ 1/(60 + rank_i + 1)`，多路命中累加得分
- 摘要层命中文档的 chunk 额外 ×1.2 加权（替代原 primary/fallback 分组）
- 最终按 RRF 分数降序取 top-k

**选型说明**：评估了 RRF vs Reranker（cross-encoder），在个人知识库小规模场景下 RRF 投入产出比更高——无需额外模型文件（节省 ~100MB），零推理延迟，效果差异不显著。Reranker 作为未来选项，触发条件：知识库文档超过 500 个或用户明确反馈排序不准。

---

### 🔮 TODO-APP-3：知识库多选挂载

**问题**：当前知识模式只能选一个知识库，跨知识库问答不支持。

**方案**：输入框知识库选择器改为多选，`search_knowledge` tool 遍历多个 Store 合并结果。

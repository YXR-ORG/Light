package kb

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// query 预处理：停用词清洗 + 分词 + 同义词归一化。
//
// 这一层只服务于"检索"，不影响传给 LLM 的原始用户消息。
// 原则（取自关键词提纯实践）：删废话、留干货、统一黑话、锁定意图。
// 分词是前提——中文无空格，只有先切词，停用词替换和同义词按词匹配才成立。

// stopWords 通用停用词：礼貌废话、疑问模板、无效修饰、口语助词。
// 注意：这些词对"检索"无价值（稀释核心语义、制造噪音匹配），
// 但对"回答"可能有用——本表只清洗检索用 query。
var stopWords = map[string]struct{}{
	// 礼貌/语气
	"请问": {}, "麻烦": {}, "谢谢": {}, "一下": {}, "帮帮": {}, "帮我": {},
	"能不能": {}, "有没有": {}, "我想知道": {}, "我想问": {}, "请问一下": {},
	"麻烦一下": {}, "帮忙": {}, "可以吗": {}, "行吗": {},
	// 疑问模板
	"是什么": {}, "有哪些": {}, "怎么办": {}, "怎么处理": {}, "注意什么": {},
	"怎么样": {}, "如何处理": {}, "是什么意思": {}, "什么意思": {}, "是什么东西": {},
	"怎么说": {}, "为啥": {}, "为什么": {}, "是不是": {},
	// 场景口水
	"新手": {}, "第一次": {}, "日常": {}, "普通": {}, "最近": {},
	"咱们这边": {}, "我们这边": {}, "你们这边": {}, "这边": {},
	// 口语助词
	"啊": {}, "呀": {}, "吧": {}, "呢": {}, "嘛": {}, "哦": {},
	"哎": {}, "哈": {}, "喽": {}, "啦": {}, "呗": {},
}

// ExpandedQuery 预处理后的检索查询。
type ExpandedQuery struct {
	Cleaned    string        // 归一化后、空格拼接的 query，供 FTS5/摘要检索
	Terms      []string      // 最终用于检索的词列表（含归一化后的标准词）
	HasSynonym bool          // 是否命中同义词表（用于日志/调试）
	Mappings   []SynonymPair // 命中的同义词映射（source→target），透传给 LLM
}

// PreprocessQuery 停用词清洗：删除对检索无价值的废话词。
// 在分词前对原始字符串做替换，保留实体/业务词。
// 清洗结果为空时回退原 query，避免空检索。
func PreprocessQuery(query string) string {
	q := strings.TrimSpace(query)
	if q == "" {
		return q
	}
	for w := range stopWords {
		if strings.Contains(q, w) {
			q = strings.ReplaceAll(q, w, " ")
		}
	}
	// 压缩多余空白
	q = strings.Join(strings.Fields(q), " ")
	if strings.TrimSpace(q) == "" {
		return query // 纯语气句清洗后为空，回退
	}
	return q
}

// ExpandQuery 完整预处理：停用词清洗 → 分词 → 同义词归一化。
// store 为 nil 时仅做清洗+分词（无同义词上下文）。
//
// 同义词归一化支持两种匹配：
//  1. 单 token 精确匹配：如 token "塔吊" 命中表 → 替换
//  2. 相邻 token 短语匹配：如 ["爬高","干活"] 命中 source "爬高干活" → 合并替换
//     （分词器可能把多字口语词切成多个 token，需合并匹配才能命中）
func (s *Store) ExpandQuery(query string) ExpandedQuery {
	cleaned := PreprocessQuery(query)
	tokens := Tokenize(cleaned)

	if s == nil || len(tokens) == 0 {
		return ExpandedQuery{Cleaned: strings.Join(tokens, " "), Terms: tokens}
	}

	synonyms, err := s.loadSynonyms()
	if err != nil || len(synonyms) == 0 {
		return ExpandedQuery{Cleaned: strings.Join(tokens, " "), Terms: tokens}
	}

	normalized, mappings := applySynonyms(tokens, synonyms)
	return ExpandedQuery{
		Cleaned:    strings.Join(normalized, " "),
		Terms:      normalized,
		HasSynonym: len(mappings) > 0,
		Mappings:   mappings,
	}
}

// applySynonyms 对分词结果做同义词归一化。
// entries: 同义词表（source 已分词，便于和 tokens 匹配）
// 采用贪心最长匹配：从最长 source 开始尝试，命中即替换并跳过已消费 token。
// 返回归一化后的 tokens 和命中的映射列表（Source 用原始录入形式，透传给 LLM）。
func applySynonyms(tokens []string, entries []synEntry) ([]string, []SynonymPair) {
	if len(tokens) == 0 || len(entries) == 0 {
		return tokens, nil
	}

	// 复制一份并按 parts 长度降序，优先匹配长短语（如"爬高 干活"优先于"爬高"）
	sorted := make([]synEntry, len(entries))
	copy(sorted, entries)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if len(sorted[j].sourceParts) > len(sorted[i].sourceParts) {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	var result []string
	var mappings []SynonymPair
	i := 0
	for i < len(tokens) {
		matched := false
		for _, e := range sorted {
			if i+len(e.sourceParts) > len(tokens) {
				continue
			}
			match := true
			for k, p := range e.sourceParts {
				if tokens[i+k] != p {
					match = false
					break
				}
			}
			if match {
				result = append(result, e.target)
				i += len(e.sourceParts)
				mappings = append(mappings, SynonymPair{
					Source: e.origSource, // 原始录入形式，如 "爬高干活"
					Target: e.target,
				})
				matched = true
				break
			}
		}
		if !matched {
			result = append(result, tokens[i])
			i++
		}
	}
	return result, mappings
}

// --- 同义词表存储（每个知识库的 kb.db 独立维护）---

// Synonym 同义词条目。
type Synonym struct {
	ID        int64  `json:"id"`
	Source    string `json:"source"` // 口语词/别名，如 "爬高干活"
	Target    string `json:"target"` // 标准词，如 "高处作业"
	CreatedAt string `json:"created_at"`
}

// synEntry 同义词表的内存表示（分词后便于匹配）。
type synEntry struct {
	origSource  string   // 原始 source（用户录入形式，如 "爬高干活"），用于透传给 LLM
	sourceParts []string // source 分词后的 parts，如 ["爬高","干活"]，用于和 tokens 匹配
	target      string
}

var (
	synCacheMu    sync.RWMutex
	synCache      = map[string][]synEntry{} // storeKey -> entries
	synCacheDirty = map[string]bool{}       // storeKey -> 是否需重新加载
)

// loadSynonyms 加载本知识库的同义词表（带缓存）。
// 返回 synEntry 切片，source 已分词便于和 query tokens 匹配。
func (s *Store) loadSynonyms() ([]synEntry, error) {
	key := fmt.Sprintf("%p", s)
	synCacheMu.RLock()
	if cache, ok := synCache[key]; ok && !synCacheDirty[key] {
		synCacheMu.RUnlock()
		return cache, nil
	}
	synCacheMu.RUnlock()

	synCacheMu.Lock()
	defer synCacheMu.Unlock()
	// double-check
	if cache, ok := synCache[key]; ok && !synCacheDirty[key] {
		return cache, nil
	}

	rows, err := s.db.Query(`SELECT source, target FROM synonyms`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []synEntry
	for rows.Next() {
		var src, tgt string
		if err := rows.Scan(&src, &tgt); err != nil {
			continue
		}
		// source 经过分词，使其与 query 分词后的 tokens 形式一致。
		// 例如 "爬高干活" → ["爬高","干活"]，才能匹配 tokens ["爬高","干活"]
		parts := Tokenize(src)
		if len(parts) == 0 {
			continue
		}
		entries = append(entries, synEntry{
			origSource:  src,
			sourceParts: parts,
			target:      tgt,
		})
	}
	synCache[key] = entries
	synCacheDirty[key] = false
	return entries, rows.Err()
}

// AddSynonym 新增同义词。
func (s *Store) AddSynonym(source, target string) error {
	source = strings.TrimSpace(source)
	target = strings.TrimSpace(target)
	if source == "" || target == "" {
		return nil
	}
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO synonyms(source, target, created_at) VALUES(?,?,?)`,
		source, target, time.Now().Format(time.RFC3339),
	)
	if err == nil {
		s.invalidateSynCache()
	}
	return err
}

// DeleteSynonym 删除同义词。
func (s *Store) DeleteSynonym(id int64) error {
	_, err := s.db.Exec(`DELETE FROM synonyms WHERE id=?`, id)
	if err == nil {
		s.invalidateSynCache()
	}
	return err
}

// ListSynonyms 列出所有同义词。
func (s *Store) ListSynonyms() ([]Synonym, error) {
	rows, err := s.db.Query(`SELECT id, source, target, created_at FROM synonyms ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Synonym
	for rows.Next() {
		var syn Synonym
		if err := rows.Scan(&syn.ID, &syn.Source, &syn.Target, &syn.CreatedAt); err != nil {
			continue
		}
		result = append(result, syn)
	}
	return result, rows.Err()
}

// invalidateSynCache 失效本 Store 的同义词缓存。
func (s *Store) invalidateSynCache() {
	synCacheMu.Lock()
	defer synCacheMu.Unlock()
	synCacheDirty[fmt.Sprintf("%p", s)] = true
}

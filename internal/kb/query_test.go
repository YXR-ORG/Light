package kb

import (
	"testing"
)

// TestPreprocessQuery 停用词清洗
func TestPreprocessQuery(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"礼貌废话", "请问高处作业的安全规范", "高处作业的安全规范"},
		{"疑问模板", "高处作业有哪些注意事项", "高处作业 注意事项"}, // "有哪些"删后留空格，对后续分词无害
		{"口语助词", "塔吊操作啊", "塔吊操作"},
		{"场景口水", "新手日常配电柜维护", "配电柜维护"},
		{"纯语气句回退", "请问一下", "请问一下"}, // 清洗后为空，回退原 query
		{"空串", "", ""},
		{"无停用词", "高处作业安全规范", "高处作业安全规范"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := PreprocessQuery(c.in)
			if got != c.want {
				t.Errorf("PreprocessQuery(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// TestTokenize 中文分词
func TestTokenize(t *testing.T) {
	cases := []struct {
		name string
		in   string
		// 只验证关键词是否出现，不严格断言完整切片（分词结果可能微调）
		mustContain []string
		mustAbsent  []string // 不应出现的单字/标点
	}{
		{
			name:        "口语提问",
			in:          "请问新手夏天工地爬高干活要注意啥呀",
			mustContain: []string{"夏天", "工地", "爬高", "干活", "注意"},
			mustAbsent:  []string{"呀", "啊", "啥"},
		},
		{
			name:        "标准术语",
			in:          "高处作业的安全防护规范",
			mustContain: []string{"高处", "作业", "安全", "防护", "规范"},
		},
		{
			name:        "含标点",
			in:          "塔吊、配电柜、电焊机",
			mustContain: []string{"塔吊", "配电", "电焊"},
			mustAbsent:  []string{"、"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tokens := Tokenize(c.in)
			if len(tokens) == 0 {
				t.Fatalf("Tokenize(%q) returned empty", c.in)
			}
			tokenSet := make(map[string]bool)
			for _, tk := range tokens {
				tokenSet[tk] = true
			}
			for _, w := range c.mustContain {
				if !tokenSet[w] {
					t.Errorf("Tokenize(%q): expected token %q in %v", c.in, w, tokens)
				}
			}
			for _, w := range c.mustAbsent {
				if tokenSet[w] {
					t.Errorf("Tokenize(%q): unexpected token %q in %v", c.in, w, tokens)
				}
			}
		})
	}
}

// TestCutToQuery 分词后空格拼接（供 FTS5）
func TestCutToQuery(t *testing.T) {
	got := CutToQuery("高处作业安全规范")
	if got == "高处作业安全规范" {
		t.Errorf("CutToQuery 未分词，仍为连续字符串: %q", got)
	}
	// 应包含空格分隔的 token
	if !contains(got, " ") {
		t.Errorf("CutToQuery 结果应含空格: %q", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// TestApplySynonyms 同义词归一化（单 token + 多 token 短语匹配）
func TestApplySynonyms(t *testing.T) {
	entries := []synEntry{
		{origSource: "爬高干活", sourceParts: []string{"爬高", "干活"}, target: "高处作业"}, // 多 token 短语
		{origSource: "塔吊", sourceParts: []string{"塔吊"}, target: "塔式起重机"},            // 单 token
	}

	cases := []struct {
		name        string
		tokens      []string
		wantResult  []string
		wantHit     bool
	}{
		{
			name:       "多token短语命中",
			tokens:     []string{"夏天", "工地", "爬高", "干活", "注意"},
			wantResult: []string{"夏天", "工地", "高处作业", "注意"},
			wantHit:    true,
		},
		{
			name:       "单token命中",
			tokens:     []string{"塔吊", "操作", "规程"},
			wantResult: []string{"塔式起重机", "操作", "规程"},
			wantHit:    true,
		},
		{
			name:       "无命中",
			tokens:     []string{"配电柜", "维护"},
			wantResult: []string{"配电柜", "维护"},
			wantHit:    false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, mappings := applySynonyms(c.tokens, entries)
			hit := len(mappings) > 0
			if hit != c.wantHit {
				t.Errorf("hit = %v, want %v", hit, c.wantHit)
			}
			if !sliceEqual(got, c.wantResult) {
				t.Errorf("applySynonyms(%v) = %v, want %v", c.tokens, got, c.wantResult)
			}
		})
	}
}

func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestExpandQuery 全链路：清洗→分词→同义词归一化
func TestExpandQuery(t *testing.T) {
	s := newTestStore(t)
	defer s.db.Close()

	// 录入同义词：口语 → 标准术语
	if err := s.AddSynonym("爬高干活", "高处作业"); err != nil {
		t.Fatalf("AddSynonym failed: %v", err)
	}
	if err := s.AddSynonym("电线漏电", "剩余电流故障"); err != nil {
		t.Fatalf("AddSynonym failed: %v", err)
	}

	cases := []struct {
		name           string
		query          string
		wantContain    string // Cleaned 应包含的归一化标准词
		wantSynonymHit bool
	}{
		{
			name:           "口语归一化",
			query:          "请问新手工地爬高干活要注意啥呀",
			wantContain:    "高处作业",
			wantSynonymHit: true,
		},
		{
			name:           "另一口语归一化",
			query:          "电线漏电怎么办",
			wantContain:    "剩余电流故障",
			wantSynonymHit: true,
		},
		{
			name:           "无同义词命中",
			query:          "配电柜安全规范",
			wantContain:    "配电柜",
			wantSynonymHit: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := s.ExpandQuery(c.query)
			if got.HasSynonym != c.wantSynonymHit {
				t.Errorf("HasSynonym = %v, want %v (query=%q, cleaned=%q, terms=%v)",
					got.HasSynonym, c.wantSynonymHit, c.query, got.Cleaned, got.Terms)
			}
			if !contains(got.Cleaned, c.wantContain) {
				t.Errorf("ExpandQuery(%q).Cleaned = %q, want contain %q (terms=%v)",
					c.query, got.Cleaned, c.wantContain, got.Terms)
			}
		})
	}
}

// TestSearchWithSynonym 端到端：验证"同义不同字"能被召回
// 文档写"高处作业"，用户问"爬高干活"，无同义词则搜不到，有则能搜到。
func TestSearchWithSynonym(t *testing.T) {
	s := newTestStore(t)
	defer s.db.Close()

	// 灌入文档：标准术语"高处作业"
	docID, err := s.InsertDocument("安全规范.pdf", "application/pdf", 100)
	if err != nil {
		t.Fatalf("InsertDocument: %v", err)
	}
	chunks := []Chunk{
		{Content: "高处作业是指坠落高度基准面2米以上有可能坠落的高处进行的作业，必须系安全带。", ChunkIndex: 0},
	}
	if err := s.InsertChunks(docID, chunks); err != nil {
		t.Fatalf("InsertChunks: %v", err)
	}

	// 无同义词：用口语"爬高干活"搜索，应召回很少或为空（trigram 难匹配）
	// 注意：trigram 仍可能命中"高处作业"的子串如"作业"，所以这里用更严格的判定
	resultsBefore, err := s.Search("爬高干活注意", 5)
	if err != nil {
		t.Fatalf("Search before synonym: %v", err)
	}
	t.Logf("before synonym: %d results", len(resultsBefore))

	// 加入同义词映射
	if err := s.AddSynonym("爬高干活", "高处作业"); err != nil {
		t.Fatalf("AddSynonym: %v", err)
	}

	// 有同义词：同样的口语搜索，应能召回"高处作业"文档
	resultsAfter, err := s.Search("爬高干活注意", 5)
	if err != nil {
		t.Fatalf("Search after synonym: %v", err)
	}
	t.Logf("after synonym: %d results", len(resultsAfter))

	if len(resultsAfter) == 0 {
		t.Errorf("同义词归一化后仍无召回，预期应能搜到高处作业文档")
	}
	// 验证至少有一条结果包含"高处作业"
	found := false
	for _, r := range resultsAfter {
		if contains(r.Content, "高处作业") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("召回结果未包含高处作业文档，results=%v", resultsAfter)
	}
}

// TestSearchWithExpansion 验证归一化信息透传（关键：修复"检索层做了归一化但 LLM 不知情"）
func TestSearchWithExpansion(t *testing.T) {
	s := newTestStore(t)
	defer s.db.Close()

	docID, err := s.InsertDocument("安全规范.pdf", "application/pdf", 100)
	if err != nil {
		t.Fatalf("InsertDocument: %v", err)
	}
	if err := s.InsertChunks(docID, []Chunk{
		{Content: "高处作业是指坠落高度基准面2米以上的作业，必须系安全带。", ChunkIndex: 0},
	}); err != nil {
		t.Fatalf("InsertChunks: %v", err)
	}

	if err := s.AddSynonym("爬高干活", "高处作业"); err != nil {
		t.Fatalf("AddSynonym: %v", err)
	}

	// 用口语"爬高干活"检索，应返回归一化信息
	resp, err := s.SearchWithExpansion("爬高干活注意", 5)
	if err != nil {
		t.Fatalf("SearchWithExpansion: %v", err)
	}

	if resp.OriginalQuery != "爬高干活注意" {
		t.Errorf("OriginalQuery = %q, want 爬高干活注意", resp.OriginalQuery)
	}
	if !contains(resp.RetrievalQuery, "高处作业") {
		t.Errorf("RetrievalQuery = %q, want contain 高处作业", resp.RetrievalQuery)
	}
	if len(resp.SynonymMappings) == 0 {
		t.Fatalf("SynonymMappings 为空，应包含命中的映射")
	}
	found := false
	for _, m := range resp.SynonymMappings {
		if m.Source == "爬高干活" && m.Target == "高处作业" {
			found = true
		}
	}
	if !found {
		t.Errorf("SynonymMappings 未包含 爬高干活→高处作业，got %v", resp.SynonymMappings)
	}
	if resp.Total == 0 {
		t.Errorf("Total=0，预期应召回高处作业文档")
	}
}

// TestSynonymCRUD 同义词增删查
func TestSynonymCRUD(t *testing.T) {
	s := newTestStore(t)
	defer s.db.Close()

	if err := s.AddSynonym("爬高干活", "高处作业"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := s.AddSynonym("塔吊", "塔式起重机"); err != nil {
		t.Fatalf("Add: %v", err)
	}

	list, err := s.ListSynonyms()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("ListSynonyms count = %d, want 2", len(list))
	}

	// 验证缓存生效：loadSynonyms 应返回 entries（含原始 source 与分词 parts）
	entries, err := s.loadSynonyms()
	if err != nil {
		t.Fatalf("loadSynonyms: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("loadSynonyms count = %d, want 2", len(entries))
	}
	foundSyn := false
	for _, e := range entries {
		if e.origSource == "爬高干活" && e.target == "高处作业" {
			foundSyn = true
			// source 应被分词为 ["爬高","干活"]
			if !sliceEqual(e.sourceParts, []string{"爬高", "干活"}) {
				t.Errorf("sourceParts = %v, want [爬高 干活]", e.sourceParts)
			}
		}
	}
	if !foundSyn {
		t.Errorf("loadSynonyms 未找到 爬高干活→高处作业，got %+v", entries)
	}

	// 删除
	if err := s.DeleteSynonym(list[0].ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	list, _ = s.ListSynonyms()
	if len(list) != 1 {
		t.Errorf("after delete count = %d, want 1", len(list))
	}
}

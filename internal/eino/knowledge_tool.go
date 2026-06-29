package eino

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"light-ai/internal/kb"
)

// KnowledgeSearchTool 实现 eino InvokableTool，通过 FTS5 检索知识库。
type KnowledgeSearchTool struct {
	kbID  string
	store *kb.Store
}

// NewKnowledgeSearchTool 创建知识库检索工具。kbDir 为 kb.db 所在目录。
func NewKnowledgeSearchTool(kbID, kbDir string) (*KnowledgeSearchTool, error) {
	s, err := kb.GetStore(kbID, kbDir)
	if err != nil {
		return nil, fmt.Errorf("knowledge tool: open store failed: %w", err)
	}
	return &KnowledgeSearchTool{kbID: kbID, store: s}, nil
}

// Info 返回 tool schema，供 LLM function call 使用。
func (t *KnowledgeSearchTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "search_knowledge",
		Desc: `在知识库中搜索相关文档片段。使用规则：
1. 必须先调用此工具再回答，不得凭空编造。
2. 跨文档问题（如"A和B有什么关系"）：必须分别搜索每个实体，至少调用2次。
3. 每次查询用单一精确词，不要把多个概念混在一个query里。
4. 搜索结果不足时换不同关键词重试，最多搜索3轮。
5. 若返回中带有 synonym_mappings（如"孙彤→孙小仙"），说明用户用词已归一化为文档标准词并命中，检索到的内容即为用户所问实体的信息，应据此作答，不得因原文未出现用户用词而拒绝回答。`,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Type:     schema.String,
				Desc:     "搜索关键词，每次只搜一个核心概念，如人名、地名、事件名",
				Required: true,
			},
			"top_k": {
				Type:     schema.Integer,
				Desc:     "返回结果数量，默认 10，最大 20",
				Required: false,
			},
		}),
	}, nil
}

type searchArgs struct {
	Query string `json:"query"`
	TopK  int    `json:"top_k"`
}

type searchResponse struct {
	Results         []kb.SearchResult `json:"results"`
	Total           int               `json:"total"`
	OriginalQuery   string            `json:"original_query,omitempty"`   // LLM 原始 query
	RetrievalQuery  string            `json:"retrieval_query,omitempty"`  // 实际检索用的归一化后 query
	SynonymMappings []kb.SynonymPair  `json:"synonym_mappings,omitempty"` // 命中的同义词映射
	Note            string            `json:"note,omitempty"`             // 给 LLM 的提示
}

// InvokableRun 执行检索，返回 JSON 格式结果。
// 当 query 被同义词归一化时，会在返回里说明映射关系，
// 避免检索层做了归一化但 LLM 不知情，误判"库里没有该词"。
func (t *KnowledgeSearchTool) InvokableRun(_ context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	var args searchArgs
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", fmt.Errorf("search_knowledge: invalid args: %w", err)
	}
	if args.Query == "" {
		return `{"results":[],"total":0}`, nil
	}

	resp, err := t.store.SearchWithExpansion(args.Query, args.TopK)
	if err != nil {
		slog.Warn("search_knowledge failed", "query", args.Query, "error", err)
		return `{"results":[],"total":0,"error":"搜索失败，请重试"}`, nil
	}

	out := searchResponse{
		Results:         resp.Results,
		Total:           resp.Total,
		OriginalQuery:   resp.OriginalQuery,
		RetrievalQuery:  resp.RetrievalQuery,
		SynonymMappings: resp.SynonymMappings,
	}

	// 若发生了同义词归一化，给 LLM 明确提示：
	// 搜的"孙彤"被映射成"孙小仙"检索，命中即等于搜到"孙彤"。
	if len(resp.SynonymMappings) > 0 {
		var pairs []string
		for _, m := range resp.SynonymMappings {
			pairs = append(pairs, fmt.Sprintf("「%s」→「%s」", m.Source, m.Target))
		}
		out.Note = fmt.Sprintf(
			"查询已做同义词归一化：%s。已用标准词检索并命中相关文档，"+
				"请基于检索结果回答，不要因原文未出现「%s」就断言找不到。",
			strings.Join(pairs, "，"), resp.OriginalQuery,
		)
	}

	b, _ := json.Marshal(out)
	return string(b), nil
}

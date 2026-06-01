package eino

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

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
4. 搜索结果不足时换不同关键词重试，最多搜索3轮。`,
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
	Results []kb.SearchResult `json:"results"`
	Total   int               `json:"total"`
}

// InvokableRun 执行 FTS5 检索，返回 JSON 格式结果。
func (t *KnowledgeSearchTool) InvokableRun(_ context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	var args searchArgs
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", fmt.Errorf("search_knowledge: invalid args: %w", err)
	}
	if args.Query == "" {
		return `{"results":[],"total":0}`, nil
	}

	results, err := t.store.Search(args.Query, args.TopK)
	if err != nil {
		slog.Warn("search_knowledge failed", "query", args.Query, "error", err)
		return `{"results":[],"total":0,"error":"搜索失败，请重试"}`, nil
	}

	resp := searchResponse{Results: results, Total: len(results)}
	b, _ := json.Marshal(resp)
	return string(b), nil
}

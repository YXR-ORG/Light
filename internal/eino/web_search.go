package eino

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// WebSearchTool implements eino InvokableTool using Tavily Search API.
type WebSearchTool struct {
	apiKey     string
	maxResults int
	client     *http.Client
}

func NewWebSearchTool(apiKey string, maxResults int) *WebSearchTool {
	if maxResults <= 0 {
		maxResults = 5
	}
	return &WebSearchTool{
		apiKey:     apiKey,
		maxResults: maxResults,
		client:     &http.Client{Timeout: 15 * time.Second},
	}
}

func (t *WebSearchTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "web_search",
		Desc: "Search the web for current information, news, facts, or any topic requiring up-to-date knowledge. Returns relevant results with titles, URLs, and content snippets.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Type:     schema.String,
				Desc:     "The search query",
				Required: true,
			},
		}),
	}, nil
}

func (t *WebSearchTool) InvokableRun(_ context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	query := extractJSONString(argumentsInJSON, "query")
	if query == "" {
		return "", fmt.Errorf("query is required")
	}

	results, err := t.search(query)
	if err != nil {
		return fmt.Sprintf("搜索失败: %v", err), nil
	}
	if len(results) == 0 {
		return "未找到相关结果", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("搜索「%s」的结果：\n\n", query))
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("%d. **%s**\n   %s\n   来源: %s\n\n", i+1, r.title, r.content, r.url))
	}
	return sb.String(), nil
}

type tavilyRequest struct {
	APIKey     string `json:"api_key"`
	Query      string `json:"query"`
	MaxResults int    `json:"max_results"`
	SearchDepth string `json:"search_depth"`
}

type tavilyResult struct {
	URL     string  `json:"url"`
	Title   string  `json:"title"`
	Content string  `json:"content"`
	Score   float64 `json:"score"`
}

type tavilyResponse struct {
	Results []tavilyResult `json:"results"`
}

type searchResult struct {
	title   string
	url     string
	content string
}

func (t *WebSearchTool) search(query string) ([]searchResult, error) {
	reqBody, _ := json.Marshal(tavilyRequest{
		APIKey:      t.apiKey,
		Query:       query,
		MaxResults:  t.maxResults,
		SearchDepth: "basic",
	})

	resp, err := t.client.Post(
		"https://api.tavily.com/search",
		"application/json",
		bytes.NewReader(reqBody),
	)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API 错误 %d: %s", resp.StatusCode, string(body))
	}

	var tavilyResp tavilyResponse
	if err := json.Unmarshal(body, &tavilyResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	var results []searchResult
	for _, r := range tavilyResp.Results {
		results = append(results, searchResult{
			title:   r.Title,
			url:     r.URL,
			content: r.Content,
		})
	}
	return results, nil
}

// extractJSONString extracts a string value from a simple JSON object.
func extractJSONString(jsonStr, key string) string {
	needle := `"` + key + `"`
	idx := strings.Index(jsonStr, needle)
	if idx < 0 {
		return ""
	}
	rest := jsonStr[idx+len(needle):]
	colon := strings.Index(rest, ":")
	if colon < 0 {
		return ""
	}
	rest = strings.TrimSpace(rest[colon+1:])
	if len(rest) == 0 || rest[0] != '"' {
		return ""
	}
	rest = rest[1:]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return ""
	}
	return rest[:end]
}

package eino

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// NewWebSearchTool creates a search tool for the given engine.
// engine: "tavily" | "exa" | "brave" | "searxng"
// For searxng, apiKey is the base URL of the SearXNG instance.
func NewWebSearchTool(engine, apiKey string, maxResults int) tool.InvokableTool {
	if maxResults <= 0 {
		maxResults = 5
	}
	c := &http.Client{Timeout: 15 * time.Second}
	switch engine {
	case "exa":
		return &exaSearchTool{apiKey: apiKey, maxResults: maxResults, client: c}
	case "brave":
		return &braveSearchTool{apiKey: apiKey, maxResults: maxResults, client: c}
	case "searxng":
		instanceURL := apiKey // for searxng the "key" field holds the instance URL
		if instanceURL == "" {
			instanceURL = "https://searx.be"
		}
		return &searxngSearchTool{instanceURL: instanceURL, maxResults: maxResults, client: c}
	default: // tavily
		return &tavilySearchTool{apiKey: apiKey, maxResults: maxResults, client: c}
	}
}

// ── shared tool info & helpers ────────────────────────────────────────────────

func webSearchToolInfo() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: "web_search",
		Desc: "Search the web for current information, news, facts, or any topic requiring up-to-date knowledge.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Type:     schema.String,
				Desc:     "The search query",
				Required: true,
			},
		}),
	}
}

func doPost(client *http.Client, rawURL string, headers map[string]string, payload any) ([]byte, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, rawURL, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return doRequest(client, req)
}

func doGet(client *http.Client, rawURL string, headers map[string]string, params map[string]string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	q := req.URL.Query()
	for k, v := range params {
		q.Set(k, v)
	}
	req.URL.RawQuery = q.Encode()
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return doRequest(client, req)
}

func doRequest(client *http.Client, req *http.Request) ([]byte, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API 错误 %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	return body, nil
}

func formatResults(query string, items []struct{ title, snippet, link string }) string {
	if len(items) == 0 {
		return "未找到相关结果"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("搜索「%s」的结果：\n\n", query))
	for i, r := range items {
		sb.WriteString(fmt.Sprintf("%d. **%s**\n   %s\n   来源: %s\n\n", i+1, r.title, r.snippet, r.link))
	}
	return sb.String()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
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

// ── Tavily ────────────────────────────────────────────────────────────────────

type tavilySearchTool struct {
	apiKey     string
	maxResults int
	client     *http.Client
}

func (t *tavilySearchTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return webSearchToolInfo(), nil
}

func (t *tavilySearchTool) InvokableRun(_ context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	query := extractJSONString(argumentsInJSON, "query")
	if query == "" {
		return "", fmt.Errorf("query is required")
	}
	type request struct {
		APIKey      string `json:"api_key"`
		Query       string `json:"query"`
		MaxResults  int    `json:"max_results"`
		SearchDepth string `json:"search_depth"`
	}
	type result struct {
		URL     string `json:"url"`
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	type response struct {
		Results []result `json:"results"`
	}
	body, err := doPost(t.client, "https://api.tavily.com/search",
		map[string]string{"Content-Type": "application/json"},
		request{APIKey: t.apiKey, Query: query, MaxResults: t.maxResults, SearchDepth: "basic"})
	if err != nil {
		return fmt.Sprintf("搜索失败: %v", err), nil
	}
	var resp response
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Sprintf("解析响应失败: %v", err), nil
	}
	items := make([]struct{ title, snippet, link string }, len(resp.Results))
	for i, r := range resp.Results {
		items[i] = struct{ title, snippet, link string }{r.Title, r.Content, r.URL}
	}
	return formatResults(query, items), nil
}

// ── Exa ──────────────────────────────────────────────────────────────────────

type exaSearchTool struct {
	apiKey     string
	maxResults int
	client     *http.Client
}

func (t *exaSearchTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return webSearchToolInfo(), nil
}

func (t *exaSearchTool) InvokableRun(_ context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	query := extractJSONString(argumentsInJSON, "query")
	if query == "" {
		return "", fmt.Errorf("query is required")
	}
	type contentsReq struct {
		Text bool `json:"text"`
	}
	type request struct {
		Query      string      `json:"query"`
		NumResults int         `json:"numResults"`
		Type       string      `json:"type"`
		Contents   contentsReq `json:"contents"`
	}
	type result struct {
		Title   string `json:"title"`
		URL     string `json:"url"`
		Text    string `json:"text"`
		Summary string `json:"summary"`
	}
	type response struct {
		Results []result `json:"results"`
	}
	body, err := doPost(t.client, "https://api.exa.ai/search",
		map[string]string{
			"Content-Type": "application/json",
			"x-api-key":    t.apiKey,
		},
		request{Query: query, NumResults: t.maxResults, Type: "auto", Contents: contentsReq{Text: true}})
	if err != nil {
		return fmt.Sprintf("搜索失败: %v", err), nil
	}
	var resp response
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Sprintf("解析响应失败: %v", err), nil
	}
	items := make([]struct{ title, snippet, link string }, len(resp.Results))
	for i, r := range resp.Results {
		snippet := r.Summary
		if snippet == "" {
			snippet = r.Text
		}
		items[i] = struct{ title, snippet, link string }{r.Title, truncate(snippet, 300), r.URL}
	}
	return formatResults(query, items), nil
}

// ── Brave ─────────────────────────────────────────────────────────────────────

type braveSearchTool struct {
	apiKey     string
	maxResults int
	client     *http.Client
}

func (t *braveSearchTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return webSearchToolInfo(), nil
}

func (t *braveSearchTool) InvokableRun(_ context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	query := extractJSONString(argumentsInJSON, "query")
	if query == "" {
		return "", fmt.Errorf("query is required")
	}
	type webResult struct {
		Title       string `json:"title"`
		URL         string `json:"url"`
		Description string `json:"description"`
	}
	type webResults struct {
		Results []webResult `json:"results"`
	}
	type response struct {
		Web webResults `json:"web"`
	}
	count := t.maxResults
	if count > 20 {
		count = 20
	}
	body, err := doGet(t.client,
		"https://api.search.brave.com/res/v1/web/search",
		map[string]string{
			"Accept":               "application/json",
			"X-Subscription-Token": t.apiKey,
		},
		map[string]string{
			"q":     query,
			"count": fmt.Sprintf("%d", count),
		})
	if err != nil {
		return fmt.Sprintf("搜索失败: %v", err), nil
	}
	var resp response
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Sprintf("解析响应失败: %v", err), nil
	}
	items := make([]struct{ title, snippet, link string }, len(resp.Web.Results))
	for i, r := range resp.Web.Results {
		items[i] = struct{ title, snippet, link string }{r.Title, r.Description, r.URL}
	}
	return formatResults(query, items), nil
}

// ── SearXNG ───────────────────────────────────────────────────────────────────

type searxngSearchTool struct {
	instanceURL string
	maxResults  int
	client      *http.Client
}

func (t *searxngSearchTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return webSearchToolInfo(), nil
}

func (t *searxngSearchTool) InvokableRun(_ context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	query := extractJSONString(argumentsInJSON, "query")
	if query == "" {
		return "", fmt.Errorf("query is required")
	}
	type result struct {
		Title   string `json:"title"`
		URL     string `json:"url"`
		Content string `json:"content"`
	}
	type response struct {
		Results []result `json:"results"`
	}
	base := strings.TrimRight(t.instanceURL, "/")
	searchURL := base + "/search"
	body, err := doGet(t.client, searchURL,
		map[string]string{"Accept": "application/json"},
		map[string]string{
			"q":       query,
			"format":  "json",
			"engines": "general",
			"pageno":  "1",
		})
	if err != nil {
		// Try with URL-encoded query as fallback
		searchURL = base + "/search?q=" + url.QueryEscape(query) + "&format=json"
		body, err = doGet(t.client, searchURL, map[string]string{"Accept": "application/json"}, nil)
		if err != nil {
			return fmt.Sprintf("搜索失败: %v", err), nil
		}
	}
	var resp response
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Sprintf("解析响应失败: %v", err), nil
	}
	results := resp.Results
	if len(results) > t.maxResults {
		results = results[:t.maxResults]
	}
	items := make([]struct{ title, snippet, link string }, len(results))
	for i, r := range results {
		items[i] = struct{ title, snippet, link string }{r.Title, truncate(r.Content, 300), r.URL}
	}
	return formatResults(query, items), nil
}

package eino

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"

	mcpTool "github.com/cloudwego/eino-ext/components/tool/mcp"
	"github.com/cloudwego/eino/components/tool"

	"light-ai/internal/storage"
)

// BuildTaskTools 自动发现所有已启用资源，组装完整工具列表供 task 模式使用：
//   - 所有启用的 MCP servers（每个 server 的所有 tool）
//   - 所有 skills（每个 skill 一个 SkillTool）
//   - 所有知识库（每个 KB 一个 KnowledgeSearchTool）
//   - Web search（若已配置 API key）
//   - BashTool（workDir 限定）
//   - FileTool x4（workDir 限定）
//
// 任何单个资源加载失败只记 warn 日志，不中断整体。
func BuildTaskTools(ctx context.Context, workDir string, emitter BashStepEmitter) []tool.BaseTool {
	var tools []tool.BaseTool

	// 1. MCP servers（全部已启用的）
	tools = append(tools, loadAllMCPTools(ctx)...)

	// 2. Skills
	skills, err := storage.ListSkills()
	if err != nil {
		slog.Warn("BuildTaskTools: list skills failed", "error", err)
	}
	for _, s := range skills {
		if !s.Enabled {
			continue
		}
		tools = append(tools, NewSkillTool(s.ID, s.Name, s.Description, s.Content))
	}

	// 3. 知识库
	kbs, err := storage.ListKnowledgeBases()
	if err != nil {
		slog.Warn("BuildTaskTools: list knowledge bases failed", "error", err)
	}
	for _, kb := range kbs {
		kbPath := kbDirForRegistry(kb.ID)
		kbTool, err := NewKnowledgeSearchTool(kb.ID, kbPath)
		if err != nil {
			slog.Warn("BuildTaskTools: KB tool init failed", "kb", kb.Name, "error", err)
			continue
		}
		tools = append(tools, kbTool)
	}

	// 4. Web search（若已配置）
	if wsTool := buildWebSearchTool(); wsTool != nil {
		tools = append(tools, wsTool)
	}

	// 5. BashTool
	blacklist := storage.GetSettingWithDefault("task_bash_blacklist", "")
	tools = append(tools, NewBashTool(workDir, blacklist, emitter))

	// 6. FileTool x4
	tools = append(tools, NewFileTools(workDir)...)

	slog.Info("BuildTaskTools", "total", len(tools),
		"mcp_skills_kbs", len(tools)-5)
	return tools
}

// loadAllMCPTools 连接所有已启用 MCP server，返回其工具。
func loadAllMCPTools(ctx context.Context) []tool.BaseTool {
	servers, err := storage.ListMCPServers()
	if err != nil {
		slog.Warn("loadAllMCPTools: list servers failed", "error", err)
		return nil
	}
	var tools []tool.BaseTool
	for _, srv := range servers {
		if !srv.Enabled {
			continue
		}
		t := connectMCPServer(ctx, srv)
		tools = append(tools, t...)
	}
	return tools
}

// connectMCPServer 连接单个 MCP server，失败只 warn 不 panic。
func connectMCPServer(ctx context.Context, srv storage.MCPServer) []tool.BaseTool {
	connCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	var cli *mcpclient.Client
	var err error

	switch srv.Type {
	case "sse":
		cli, err = mcpclient.NewSSEMCPClient(srv.URL)
	default:
		args := registryParseArgs(srv.Args)
		envPairs := registryParseEnv(srv.Env)
		cli, err = mcpclient.NewStdioMCPClient(srv.Command, envPairs, args...)
	}
	if err != nil {
		slog.Warn("MCP client create failed", "server", srv.Name, "error", err)
		return nil
	}
	if err = cli.Start(connCtx); err != nil {
		slog.Warn("MCP client start failed", "server", srv.Name, "error", err)
		return nil
	}
	_, err = cli.Initialize(connCtx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo:      mcp.Implementation{Name: "light-ai", Version: "1.0.0"},
		},
	})
	if err != nil {
		slog.Warn("MCP initialize failed", "server", srv.Name, "error", err)
		cli.Close()
		return nil
	}
	tools, err := mcpTool.GetTools(connCtx, &mcpTool.Config{Cli: cli})
	if err != nil {
		slog.Warn("MCP GetTools failed", "server", srv.Name, "error", err)
		cli.Close()
		return nil
	}
	slog.Info("MCP tools loaded", "server", srv.Name, "count", len(tools))
	return tools
}

// buildWebSearchTool 根据 settings 决定是否创建 web search tool。
func buildWebSearchTool() tool.BaseTool {
	engine := storage.GetSettingWithDefault("search_engine", "tavily")
	var apiKey string
	switch engine {
	case "exa":
		apiKey, _ = storage.GetSetting("exa_api_key")
	case "brave":
		apiKey, _ = storage.GetSetting("brave_api_key")
	case "searxng":
		apiKey, _ = storage.GetSetting("searxng_url")
	default:
		apiKey, _ = storage.GetSetting("tavily_api_key")
		engine = "tavily"
	}
	if apiKey == "" && engine != "searxng" {
		return nil
	}
	return NewWebSearchTool(engine, apiKey, 5)
}

// kbDirForRegistry 返回知识库数据目录（与 chat handler 保持一致）。
func kbDirForRegistry(kbID string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".wails-chat", "knowledgebases", kbID)
}

func registryParseArgs(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if strings.HasPrefix(raw, "[") {
		var args []string
		if err := json.Unmarshal([]byte(raw), &args); err == nil {
			return args
		}
	}
	return strings.Fields(raw)
}

func registryParseEnv(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil
	}
	pairs := make([]string, 0, len(m))
	for k, v := range m {
		pairs = append(pairs, k+"="+v)
	}
	return pairs
}

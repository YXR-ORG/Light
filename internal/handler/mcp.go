package handler

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"

	"light-ai/internal/storage"
)

type MCPHandler struct{}

func NewMCPHandler() *MCPHandler {
	return &MCPHandler{}
}

func (h *MCPHandler) List() ([]storage.MCPServer, error) {
	return storage.ListMCPServers()
}

func (h *MCPHandler) Save(server storage.MCPServer) error {
	return storage.SaveMCPServer(&server)
}

func (h *MCPHandler) Delete(id string) error {
	return storage.DeleteMCPServer(id)
}

func (h *MCPHandler) Toggle(id string, enabled bool) error {
	return storage.ToggleMCPServer(id, enabled)
}

// TestConnection connects to the MCP server and returns the list of tool names.
func (h *MCPHandler) TestConnection(server storage.MCPServer) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var cli *mcpclient.Client
	var err error

	switch server.Type {
	case "sse":
		cli, err = mcpclient.NewSSEMCPClient(server.URL)
	default: // stdio
		args := parseArgs(server.Args)
		envPairs := parseEnv(server.Env)
		cli, err = mcpclient.NewStdioMCPClient(server.Command, envPairs, args...)
	}
	if err != nil {
		return nil, err
	}

	if err = cli.Start(ctx); err != nil {
		return nil, err
	}
	defer cli.Close()

	_, err = cli.Initialize(ctx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo: mcp.Implementation{
				Name:    "wails-ai-chat",
				Version: "1.0.0",
			},
		},
	})
	if err != nil {
		return nil, err
	}

	result, err := cli.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(result.Tools))
	for _, t := range result.Tools {
		names = append(names, t.Name)
	}
	return names, nil
}

// parseArgs parses a JSON array string or space-separated string into []string.
func parseArgs(raw string) []string {
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

// parseEnv parses a JSON object string into KEY=VALUE pairs.
func parseEnv(raw string) []string {
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

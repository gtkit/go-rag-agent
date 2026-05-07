package ragagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/gtkit/httpc"
)

// MCPToolConfig configures an HTTP JSON-RPC MCP tool adapter.
type MCPToolConfig struct {
	Name        string
	Description string
	Endpoint    string
	Method      string
	ToolName    string
}

type mcpTool struct {
	http *httpc.Client
	cfg  MCPToolConfig
	seq  atomic.Int64
}

type mcpToolRequest struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      int64          `json:"id"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params"`
}

type mcpToolResponse struct {
	Result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Text string `json:"text"`
	} `json:"result"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// NewMCPTool creates a Tool backed by an HTTP JSON-RPC MCP endpoint.
func NewMCPTool(httpClient *httpc.Client, cfg MCPToolConfig) (Tool, error) {
	cfg.Name = strings.TrimSpace(cfg.Name)
	cfg.Description = strings.TrimSpace(cfg.Description)
	cfg.Endpoint = strings.TrimSpace(cfg.Endpoint)
	cfg.Method = strings.TrimSpace(cfg.Method)
	cfg.ToolName = strings.TrimSpace(cfg.ToolName)
	if httpClient == nil {
		return nil, fmt.Errorf("mcp tool http client is required: %w", ErrInvalidConfig)
	}
	if cfg.Name == "" {
		return nil, fmt.Errorf("mcp tool name is required: %w", ErrInvalidConfig)
	}
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("mcp tool endpoint is required: %w", ErrInvalidConfig)
	}
	if cfg.Method == "" {
		cfg.Method = "tools/call"
	}
	if cfg.ToolName == "" {
		cfg.ToolName = cfg.Name
	}
	return &mcpTool{http: httpClient, cfg: cfg}, nil
}

func (t *mcpTool) Name() string { return t.cfg.Name }

func (t *mcpTool) Description() string { return t.cfg.Description }

func (t *mcpTool) Run(ctx context.Context, input string) (string, error) {
	arguments := map[string]any{}
	if strings.TrimSpace(input) != "" {
		if err := json.Unmarshal([]byte(input), &arguments); err != nil {
			arguments = map[string]any{"input": input}
		}
	}
	request := mcpToolRequest{
		JSONRPC: "2.0",
		ID:      t.seq.Add(1),
		Method:  t.cfg.Method,
		Params: map[string]any{
			"name":      t.cfg.ToolName,
			"arguments": arguments,
		},
	}
	var response mcpToolResponse
	status, err := t.http.RequestJSON(ctx, "POST", t.cfg.Endpoint, nil, request, &response)
	if err != nil {
		return "", fmt.Errorf("run mcp tool %s: %w", t.cfg.Name, err)
	}
	if status < 200 || status >= 300 {
		return "", fmt.Errorf("run mcp tool %s: unexpected status %d", t.cfg.Name, status)
	}
	if response.Error != nil {
		return "", fmt.Errorf("run mcp tool %s: rpc error %d: %s", t.cfg.Name, response.Error.Code, response.Error.Message)
	}
	if strings.TrimSpace(response.Result.Text) != "" {
		return response.Result.Text, nil
	}
	parts := make([]string, 0, len(response.Result.Content))
	for _, item := range response.Result.Content {
		if strings.TrimSpace(item.Text) == "" {
			continue
		}
		parts = append(parts, item.Text)
	}
	return strings.Join(parts, "\n"), nil
}

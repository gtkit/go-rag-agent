package tools

import (
	"context"
	"fmt"
	"strings"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	gtjson "github.com/gtkit/json"

	"github.com/gtkit/go-rag-agent/internal/websearch"
)

const webSearchToolName = "search_web"

// WebSearchTool 把联网搜索能力适配为 Eino 工具。
type WebSearchTool struct {
	searcher websearch.Searcher
}

type webSearchArgs struct {
	Query string `json:"query"`
}

func NewWebSearchTool(searcher websearch.Searcher) *WebSearchTool {
	return &WebSearchTool{searcher: searcher}
}

func (t *WebSearchTool) Info(context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: webSearchToolName,
		Desc: "Search the public web for fresh information when local evidence is insufficient.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Type:     schema.String,
				Desc:     "web search query",
				Required: true,
			},
		}),
	}, nil
}

func (t *WebSearchTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	if t.searcher == nil {
		return "", fmt.Errorf("web searcher is required")
	}

	var args webSearchArgs
	if err := gtjson.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", fmt.Errorf("decode web search args: %w", err)
	}
	args.Query = strings.TrimSpace(args.Query)
	if args.Query == "" {
		return "", fmt.Errorf("query is required")
	}

	results, err := t.searcher.Search(ctx, args.Query)
	if err != nil {
		return "", fmt.Errorf("search web: %w", err)
	}

	lines := make([]string, 0, len(results))
	for _, item := range results {
		lines = append(lines, fmt.Sprintf("%s\nURL: %s\nSnippet: %s", item.Title, item.URL, strings.TrimSpace(item.Content)))
	}
	return strings.Join(lines, "\n\n"), nil
}

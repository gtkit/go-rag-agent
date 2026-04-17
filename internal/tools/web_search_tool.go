package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/gtkit/go-rag-agent/internal/websearch"
)

const webSearchToolName = "search_web"

// WebSearchTool 把联网搜索能力适配为 Eino 工具。
type WebSearchTool struct {
	searcher websearch.Searcher
}

func NewWebSearchTool(searcher websearch.Searcher) *WebSearchTool {
	return &WebSearchTool{searcher: searcher}
}

func (t *WebSearchTool) Name() string {
	return webSearchToolName
}

func (t *WebSearchTool) Description() string {
	return "Search the public web for fresh information when local evidence is insufficient."
}

func (t *WebSearchTool) Run(ctx context.Context, input string) (string, error) {
	if t.searcher == nil {
		return "", fmt.Errorf("web searcher is required")
	}

	query := strings.TrimSpace(input)
	if query == "" {
		return "", fmt.Errorf("query is required")
	}

	results, err := t.searcher.Search(ctx, query)
	if err != nil {
		return "", fmt.Errorf("search web: %w", err)
	}

	lines := make([]string, 0, len(results))
	for _, item := range results {
		lines = append(lines, fmt.Sprintf("%s\nURL: %s\nSnippet: %s", item.Title, item.URL, strings.TrimSpace(item.Content)))
	}
	return strings.Join(lines, "\n\n"), nil
}

package websearch

import (
	"context"
	"fmt"
	"strings"

	"github.com/gtkit/httpc"
)

// Result 表示一次联网搜索结果。
type Result struct {
	Title   string
	URL     string
	Content string
}

// Searcher 定义联网搜索能力。
type Searcher interface {
	Search(ctx context.Context, query string) ([]Result, error)
}

// TavilyConfig 定义 Tavily 搜索配置。
type TavilyConfig struct {
	BaseURL     string
	APIKey      string
	MaxResults  int
	SearchDepth string
	Topic       string
}

type TavilyClient struct {
	http *httpc.Client
	cfg  TavilyConfig
}

type tavilySearchRequest struct {
	Query       string `json:"query"`
	SearchDepth string `json:"search_depth,omitempty"`
	Topic       string `json:"topic,omitempty"`
	MaxResults  int    `json:"max_results,omitempty"`
}

type tavilySearchResponse struct {
	Results []struct {
		Title   string `json:"title"`
		URL     string `json:"url"`
		Content string `json:"content"`
	} `json:"results"`
}

func NewTavilyClient(httpClient *httpc.Client, cfg TavilyConfig) *TavilyClient {
	return &TavilyClient{
		http: httpClient,
		cfg:  cfg,
	}
}

func (c *TavilyClient) Search(ctx context.Context, query string) ([]Result, error) {
	if c == nil || c.http == nil {
		return nil, fmt.Errorf("tavily http client is required")
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}

	var resp tavilySearchResponse
	status, err := c.http.RequestJSON(ctx, "POST", c.cfg.BaseURL, map[string]string{
		"Authorization": "Bearer " + c.cfg.APIKey,
	}, tavilySearchRequest{
		Query:       query,
		SearchDepth: c.cfg.SearchDepth,
		Topic:       c.cfg.Topic,
		MaxResults:  c.cfg.MaxResults,
	}, &resp)
	if err != nil {
		return nil, fmt.Errorf("request tavily search: %w", err)
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("tavily search unexpected status %d", status)
	}

	results := make([]Result, 0, len(resp.Results))
	for _, item := range resp.Results {
		results = append(results, Result{
			Title:   item.Title,
			URL:     item.URL,
			Content: item.Content,
		})
	}
	return results, nil
}

package ragagent

import (
	"fmt"
	"strings"

	"github.com/gtkit/httpc"

	"github.com/gtkit/go-rag-agent/internal/websearch"
)

// WebSearchConfig 定义首版联网搜索配置。
type WebSearchConfig struct {
	APIKey      string
	BaseURL     string
	MaxResults  int
	SearchDepth string
	Topic       string
}

func (c WebSearchConfig) normalized() WebSearchConfig {
	c.APIKey = strings.TrimSpace(c.APIKey)
	c.BaseURL = strings.TrimSpace(c.BaseURL)
	c.SearchDepth = strings.TrimSpace(c.SearchDepth)
	c.Topic = strings.TrimSpace(c.Topic)
	if c.BaseURL == "" {
		c.BaseURL = "https://api.tavily.com/search"
	}
	if c.MaxResults == 0 {
		c.MaxResults = 5
	}
	if c.SearchDepth == "" {
		c.SearchDepth = "basic"
	}
	if c.Topic == "" {
		c.Topic = "general"
	}
	return c
}

func (c WebSearchConfig) validate(enabled bool) error {
	c = c.normalized()
	if !enabled {
		return nil
	}
	if c.APIKey == "" {
		return fmt.Errorf("web search api key is required: %w", ErrInvalidConfig)
	}
	if c.MaxResults <= 0 {
		return fmt.Errorf("web search max results must be positive: %w", ErrInvalidConfig)
	}
	switch c.SearchDepth {
	case "basic", "advanced":
	default:
		return fmt.Errorf("web search search depth must be basic or advanced: %w", ErrInvalidConfig)
	}
	switch c.Topic {
	case "general", "news":
	default:
		return fmt.Errorf("web search topic must be general or news: %w", ErrInvalidConfig)
	}
	return nil
}

func newWebSearcher(cfg Config, governors *providerGovernors) websearch.Searcher {
	if !cfg.EnableWebSearch {
		return nil
	}

	searchCfg := cfg.WebSearch.normalized()
	httpClient := httpc.New(httpc.WithTimeout(cfg.RequestTimeout))
	if cfg.Logger != nil {
		httpClient = httpc.New(
			httpc.WithTimeout(cfg.RequestTimeout),
			httpc.WithLogger(cfg.Logger),
		)
	}
	searcher := websearch.NewTavilyClient(httpClient, websearch.TavilyConfig{
		BaseURL:     searchCfg.BaseURL,
		APIKey:      searchCfg.APIKey,
		MaxResults:  searchCfg.MaxResults,
		SearchDepth: searchCfg.SearchDepth,
		Topic:       searchCfg.Topic,
	})
	return newResilientSearcher(searcher, cfg.ProviderGovernance, governors.forProvider("tavily"), "tavily")
}

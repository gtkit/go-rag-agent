package ragagent

import (
	"context"
	"fmt"
	"strings"

	"github.com/gtkit/httpc"
)

// OpenAIRerankerConfig configures an OpenAI-compatible HTTP reranker adapter.
type OpenAIRerankerConfig struct {
	BaseURL string
	APIKey  string
	Model   string
}

type openAIReranker struct {
	http *httpc.Client
	cfg  OpenAIRerankerConfig
}

type openAIRerankRequest struct {
	Model     string   `json:"model,omitempty"`
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
}

type openAIRerankResponse struct {
	Results []struct {
		Index int     `json:"index"`
		Score float32 `json:"score"`
	} `json:"results"`
}

// NewOpenAIReranker creates a Reranker backed by an OpenAI-compatible rerank endpoint.
func NewOpenAIReranker(httpClient *httpc.Client, cfg OpenAIRerankerConfig) (Reranker, error) {
	cfg.BaseURL = strings.TrimSpace(cfg.BaseURL)
	cfg.APIKey = strings.TrimSpace(cfg.APIKey)
	cfg.Model = strings.TrimSpace(cfg.Model)
	if httpClient == nil {
		return nil, fmt.Errorf("openai reranker http client is required: %w", ErrInvalidConfig)
	}
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("openai reranker base url is required: %w", ErrInvalidConfig)
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("openai reranker api key is required: %w", ErrInvalidConfig)
	}
	return &openAIReranker{http: httpClient, cfg: cfg}, nil
}

func (r *openAIReranker) Rerank(ctx context.Context, query string, candidates []SearchHit, opts RerankOptions) ([]SearchHit, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("rerank query is required")
	}
	if opts.ShortlistSize <= 0 || len(candidates) == 0 || opts.TopK <= 0 {
		return nil, nil
	}
	shortlistSize := opts.ShortlistSize
	if shortlistSize > len(candidates) {
		shortlistSize = len(candidates)
	}
	documents := make([]string, 0, shortlistSize)
	for _, candidate := range candidates[:shortlistSize] {
		documents = append(documents, candidate.Chunk.Text)
	}

	var resp openAIRerankResponse
	status, err := r.http.RequestJSON(ctx, "POST", r.cfg.BaseURL, map[string]string{
		"Authorization": "Bearer " + r.cfg.APIKey,
	}, openAIRerankRequest{
		Model:     r.cfg.Model,
		Query:     query,
		Documents: documents,
	}, &resp)
	if err != nil {
		return nil, fmt.Errorf("request openai reranker: %w", err)
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("openai reranker unexpected status %d", status)
	}

	limit := opts.TopK
	if limit > len(resp.Results) {
		limit = len(resp.Results)
	}
	out := make([]SearchHit, 0, limit)
	seen := make(map[int]struct{}, limit)
	for _, result := range resp.Results {
		if len(out) == limit {
			break
		}
		if result.Index < 0 || result.Index >= shortlistSize {
			return nil, fmt.Errorf("invalid rerank index %d for %d candidates", result.Index, shortlistSize)
		}
		if _, ok := seen[result.Index]; ok {
			continue
		}
		seen[result.Index] = struct{}{}
		hit := candidates[result.Index]
		hit.Score = result.Score
		out = append(out, hit)
	}
	return out, nil
}

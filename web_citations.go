package ragagent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/gtkit/go-rag-agent/internal/tools"
	"github.com/gtkit/go-rag-agent/internal/websearch"
)

type webSearchCitationCollector struct {
	mu      sync.Mutex
	results []websearch.Result
	emitted int
}

func (c *webSearchCitationCollector) CollectWebSearchResults(results []websearch.Result) {
	if len(results) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.results = append(c.results, slices.Clone(results)...)
}

func (c *webSearchCitationCollector) SnapshotCitations() []Citation {
	c.mu.Lock()
	defer c.mu.Unlock()
	return citationsFromWebResults(c.results)
}

func (c *webSearchCitationCollector) DrainNewCitations() []Citation {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.emitted >= len(c.results) {
		return nil
	}
	citations := citationsFromWebResults(c.results[c.emitted:])
	c.emitted = len(c.results)
	return citations
}

func withWebSearchCitationCollector(ctx context.Context, collector *webSearchCitationCollector) context.Context {
	if collector == nil {
		return ctx
	}
	return tools.WithWebSearchResultCollector(ctx, collector)
}

func citationsFromWebResults(results []websearch.Result) []Citation {
	citations := make([]Citation, 0, len(results))
	for i, result := range results {
		source := strings.TrimSpace(result.URL)
		if source == "" {
			source = strings.TrimSpace(result.Title)
		}
		if source == "" {
			continue
		}
		citations = append(citations, Citation{
			SourcePath: source,
			Title:      strings.TrimSpace(result.Title),
			ChunkID:    webCitationChunkID(result, i),
		})
	}
	return citations
}

func webCitationChunkID(result websearch.Result, index int) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(result.URL) + "\n" + strings.TrimSpace(result.Title)))
	return fmt.Sprintf("search_web:%d:%s", index, hex.EncodeToString(sum[:8]))
}

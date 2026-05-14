package ragagent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gtkit/go-rag-agent/internal/storage"
	"github.com/gtkit/httpc"
	"github.com/redis/go-redis/v9"
)

func TestImageTextBridgeExtractText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		script    string
		wantText  string
		wantErr   string
		cancelCtx bool
	}{
		{
			name: "runs command and reads output",
			script: `#!/bin/sh
printf 'image text from %s' "$1" > "$2"
`,
			wantText: "image text from",
		},
		{
			name: "empty output is rejected",
			script: `#!/bin/sh
: > "$2"
`,
			wantErr: "produced empty text",
		},
		{
			name:      "canceled context returns context error before command",
			script:    "#!/bin/sh\nexit 0\n",
			cancelCtx: true,
			wantErr:   "context canceled",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			inputPath := filepath.Join(root, "scan.png")
			writeTestFile(t, inputPath, "png")
			scriptPath := filepath.Join(root, "image-bridge.sh")
			writeExecutableFile(t, scriptPath, tt.script)

			ctx := context.Background()
			if tt.cancelCtx {
				canceled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = canceled
			}
			cfg := ImageTextBridgeConfig{
				Command: scriptPath,
				Args:    []string{"{input}", "{output}"},
			}
			got, err := cfg.extractText(ctx, inputPath)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("extractText() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("extractText() error = %v", err)
			}
			if !strings.Contains(got, tt.wantText) {
				t.Fatalf("extractText() = %q, want containing %q", got, tt.wantText)
			}
		})
	}
}

func TestSidecarMetadataParsingEdges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		ext     string
		data    string
		want    map[string]string
		wantErr bool
	}{
		{
			name: "yaml scalars and nested values stringify",
			ext:  ".yaml",
			data: "team: search\nactive: true\nrank: 3\nnested:\n  owner: api\n",
			want: map[string]string{"team": "search", "active": "true", "rank": "3", "nested": `{"owner":"api"}`},
		},
		{
			name: "json null and blank key",
			ext:  ".json",
			data: `{"":"ignored","note":null,"score":1.5}`,
			want: map[string]string{"note": "", "score": "1.5"},
		},
		{
			name:    "unsupported extension",
			ext:     ".toml",
			data:    "team = search",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseMetadataMap([]byte(tt.data), tt.ext)
			if tt.wantErr {
				if err == nil {
					t.Fatal("parseMetadataMap() error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseMetadataMap() error = %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("metadata len = %d, want %d: %#v", len(got), len(tt.want), got)
			}
			for key, want := range tt.want {
				if got[key] != want {
					t.Fatalf("metadata[%q] = %q, want %q", key, got[key], want)
				}
			}
		})
	}

	root := t.TempDir()
	docPath := filepath.Join(root, "doc.md")
	writeTestFile(t, docPath, "body")
	writeTestFile(t, filepath.Join(root, "doc.meta.yml"), "team: search\n")
	got, err := loadSidecarMetadataForSource(docPath)
	if err != nil {
		t.Fatalf("loadSidecarMetadataForSource() error = %v", err)
	}
	if got["team"] != "search" {
		t.Fatalf("sidecar metadata = %#v, want team=search", got)
	}
	if !isMetadataSidecarPath("DOC.META.JSON") {
		t.Fatal("isMetadataSidecarPath() = false, want true for case-insensitive suffix")
	}
}

func TestRetrieverAndAdapterEdges(t *testing.T) {
	t.Parallel()

	store := &capturingRootVectorStoreStub{
		hits: []SearchHit{{
			Chunk: ChunkRecord{
				ChunkID:    "doc:0",
				ParentID:   "doc",
				SourcePath: "/kb/doc.md",
				Title:      "doc",
				Text:       "gateway api",
				Metadata:   map[string]string{"team": "search"},
				Embedding:  []float32{1, 0},
			},
			Score: 0.9,
		}},
	}
	retriever := NewRetriever(store, &fakeEmbedder{defaultVec: []float32{1, 0}}, nil, RetrieverConfig{
		TopK:                1,
		SimilarityThreshold: 0.1,
	})
	if retriever == nil {
		t.Fatal("NewRetriever() = nil, want retriever")
	}
	hits, metrics, fallbacks, err := retriever.SearchDetailed(context.Background(), RetrieverRequest{
		Query: "gateway",
		Filter: RetrievalFilter{
			SourcePrefixes: []string{"/kb"},
			Metadata:       map[string]string{"team": "search"},
		},
	})
	if err != nil {
		t.Fatalf("SearchDetailed() error = %v", err)
	}
	if len(hits) != 1 || hits[0].Chunk.ChunkID != "doc:0" {
		t.Fatalf("SearchDetailed() hits = %#v, want doc:0", hits)
	}
	if metrics.VectorCandidateCount != 1 {
		t.Fatalf("metrics.VectorCandidateCount = %d, want 1", metrics.VectorCandidateCount)
	}
	if len(fallbacks) != 0 {
		t.Fatalf("fallbacks = %#v, want empty", fallbacks)
	}
	plainHits, err := retriever.Search(context.Background(), RetrieverRequest{Query: "gateway"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(plainHits) != 1 {
		t.Fatalf("Search() len = %d, want 1", len(plainHits))
	}
	if NewRetriever(nil, &fakeEmbedder{}, nil, RetrieverConfig{}) != nil {
		t.Fatal("NewRetriever(nil store) = non-nil, want nil")
	}

	internalFilter := toInternalSearchFilter(SearchFilter{
		SourcePaths:    []string{"/kb/doc.md"},
		SourcePrefixes: []string{"/kb"},
		Metadata:       map[string]string{"team": "search"},
	})
	if len(internalFilter.SourcePaths) != 1 || internalFilter.Metadata["team"] != "search" {
		t.Fatalf("toInternalSearchFilter() = %#v, want cloned fields", internalFilter)
	}
}

func TestMCPToolNamesAndPlainInputFallback(t *testing.T) {
	t.Parallel()

	var captured map[string]any
	server := newJSONTestServer(t, 200, `{"jsonrpc":"2.0","id":1,"result":{"text":"plain result"}}`, &captured)
	tool, err := NewMCPTool(newHTTPCTestClient(), MCPToolConfig{
		Name:        "lookup",
		Description: "Lookup docs",
		Endpoint:    server,
	})
	if err != nil {
		t.Fatalf("NewMCPTool() error = %v", err)
	}
	if tool.Name() != "lookup" || tool.Description() != "Lookup docs" {
		t.Fatalf("tool name/description = %q/%q", tool.Name(), tool.Description())
	}
	got, err := tool.Run(context.Background(), "raw query")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got != "plain result" {
		t.Fatalf("Run() = %q, want plain result", got)
	}
	params := captured["params"].(map[string]any)
	args := params["arguments"].(map[string]any)
	if args["input"] != "raw query" || params["name"] != "lookup" {
		t.Fatalf("captured params = %#v, want fallback input and default tool name", params)
	}
}

func TestOpenAIRerankerEdges(t *testing.T) {
	t.Parallel()

	reranker, err := NewOpenAIReranker(newHTTPCTestClient(), OpenAIRerankerConfig{
		BaseURL: "http://127.0.0.1/unused",
		APIKey:  "key",
	})
	if err != nil {
		t.Fatalf("NewOpenAIReranker() error = %v", err)
	}
	candidates := []SearchHit{{Chunk: ChunkRecord{ChunkID: "a", Text: "alpha"}}}
	tests := []struct {
		name       string
		query      string
		candidates []SearchHit
		opts       RerankOptions
		wantErr    bool
		wantLen    int
	}{
		{name: "blank query rejected", query: " ", candidates: candidates, opts: RerankOptions{ShortlistSize: 1, TopK: 1}, wantErr: true},
		{name: "zero shortlist returns nil", query: "q", candidates: candidates, opts: RerankOptions{TopK: 1}},
		{name: "empty candidates returns nil", query: "q", opts: RerankOptions{ShortlistSize: 1, TopK: 1}},
		{name: "zero top k returns nil", query: "q", candidates: candidates, opts: RerankOptions{ShortlistSize: 1}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := reranker.Rerank(context.Background(), tt.query, tt.candidates, tt.opts)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Rerank() error = %v, wantErr %v", err, tt.wantErr)
			}
			if len(got) != tt.wantLen {
				t.Fatalf("Rerank() len = %d, want %d", len(got), tt.wantLen)
			}
		})
	}
}

func TestExtensionConstructorValidationEdges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		run     func() error
		wantErr error
	}{
		{
			name: "openai reranker requires http client",
			run: func() error {
				_, err := NewOpenAIReranker(nil, OpenAIRerankerConfig{BaseURL: "https://example.com", APIKey: "key"})
				return err
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name: "openai reranker requires base url",
			run: func() error {
				_, err := NewOpenAIReranker(newHTTPCTestClient(), OpenAIRerankerConfig{APIKey: "key"})
				return err
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name: "openai reranker requires api key",
			run: func() error {
				_, err := NewOpenAIReranker(newHTTPCTestClient(), OpenAIRerankerConfig{BaseURL: "https://example.com"})
				return err
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name: "mcp tool requires http client",
			run: func() error {
				_, err := NewMCPTool(nil, MCPToolConfig{Name: "tool", Endpoint: "https://example.com"})
				return err
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name: "mcp tool requires name",
			run: func() error {
				_, err := NewMCPTool(newHTTPCTestClient(), MCPToolConfig{Endpoint: "https://example.com"})
				return err
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name: "mcp tool requires endpoint",
			run: func() error {
				_, err := NewMCPTool(newHTTPCTestClient(), MCPToolConfig{Name: "tool"})
				return err
			},
			wantErr: ErrInvalidConfig,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.run()
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("constructor error = %v, want errors.Is(..., %v)", err, tt.wantErr)
			}
		})
	}
}

func TestInMemoryPromptCacheContextAndCloneEdges(t *testing.T) {
	t.Parallel()

	cache := NewInMemoryPromptCacheWithConfig(PromptCacheConfig{MaxEntries: 2}).(*inMemoryPromptCache)
	message := []Message{{Role: RoleUser, Content: "original"}}
	cache.Set(context.Background(), "a", message)
	message[0].Content = "mutated"
	got, ok := cache.Get(context.Background(), "a")
	if !ok {
		t.Fatal("Get(a) miss, want hit")
	}
	got[0].Content = "mutated again"
	got2, _ := cache.Get(context.Background(), "a")
	if got2[0].Content != "original" {
		t.Fatalf("cached message = %q, want clone isolation", got2[0].Content)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := cache.Delete(ctx, "a"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Delete(canceled) error = %v, want context.Canceled", err)
	}
	if err := cache.Clear(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Clear(canceled) error = %v, want context.Canceled", err)
	}
	if _, err := cache.Stats(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Stats(canceled) error = %v, want context.Canceled", err)
	}
	cache.removeElement(nil)
}

func TestAccessBoundaryMergePrefixEdges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		current []string
		allowed []string
		want    []string
	}{
		{name: "allowed empty clones current", current: []string{"/a"}, want: []string{"/a"}},
		{name: "current empty clones allowed", allowed: []string{"/allowed"}, want: []string{"/allowed"}},
		{name: "current narrower than allowed", current: []string{"/allowed/team"}, allowed: []string{"/allowed"}, want: []string{"/allowed/team"}},
		{name: "allowed narrower than current", current: []string{"/allowed"}, allowed: []string{"/allowed/team"}, want: []string{"/allowed"}},
		{name: "no overlap returns no match sentinel", current: []string{"/a"}, allowed: []string{"/b"}, want: []string{accessBoundaryNoMatchSource}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := mergeAllowedPrefixes(tt.current, tt.allowed)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("mergeAllowedPrefixes() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEvalAndStructuredHelperEdges(t *testing.T) {
	t.Parallel()

	structured := []StructuredEvalResult{
		{Name: "pass", StructuredOutputValid: true, Passed: true},
		{Name: "invalid", StructuredOutputValid: false, Passed: false},
	}
	summary := SummarizeStructuredEvalResults(structured)
	if summary.TotalCases != 2 || summary.PassedCases != 1 || summary.StructuredOutputValidRate != 0.5 {
		t.Fatalf("SummarizeStructuredEvalResults() = %+v, want total=2 passed=1 valid=0.5", summary)
	}
	if zero := SummarizeStructuredEvalResults(nil); zero.TotalCases != 0 || zero.StructuredOutputValidRate != 0 {
		t.Fatalf("SummarizeStructuredEvalResults(nil) = %+v, want zero summary", zero)
	}

	type nested struct {
		Name    string             `json:"name"`
		Ignored string             `json:"-"`
		Scores  []int              `json:"scores"`
		Labels  map[string]bool    `json:"labels"`
		Raw     map[int]string     `json:"raw"`
		Ratio   float64            `json:"ratio"`
		Enabled bool               `json:"enabled"`
		Child   *structuredSummary `json:"child"`
	}
	description := describeStructuredType(reflect.TypeOf(nested{}))
	for _, want := range []string{`"name": string`, `"scores": [integer]`, `"labels": {string: boolean}`, `"raw": object`, `"ratio": number`, `"enabled": boolean`, `"child": {`} {
		if !strings.Contains(description, want) {
			t.Fatalf("describeStructuredType() = %q, missing %q", description, want)
		}
	}
	if strings.Contains(description, "Ignored") {
		t.Fatalf("describeStructuredType() = %q, should omit ignored field", description)
	}

	if got, ok := extractFencedJSON("```json\n{\"ok\":true}\n```"); !ok || got != `{"ok":true}` {
		t.Fatalf("extractFencedJSON() = %q/%v, want object/true", got, ok)
	}
	if got, ok := extractBalancedJSON(`prefix [{"ok":true}] suffix`); !ok || got != `[{"ok":true}]` {
		t.Fatalf("extractBalancedJSON() = %q/%v, want array/true", got, ok)
	}
	if err := unmarshalStructuredJSON(`{"score":"bad"}`, &structuredSummary{}); !errors.Is(err, ErrStructuredOutputInvalid) {
		t.Fatalf("unmarshalStructuredJSON() error = %v, want ErrStructuredOutputInvalid", err)
	}
}

func TestPromptCacheTwoLevelMaintenanceEdges(t *testing.T) {
	t.Parallel()

	local := NewInMemoryPromptCacheWithConfig(PromptCacheConfig{MaxEntries: 4})
	remote := NewInMemoryPromptCacheWithConfig(PromptCacheConfig{MaxEntries: 4})
	twoLevel := NewTwoLevelPromptCache(local, remote)
	remote.Set(context.Background(), "key", []Message{{Role: RoleUser, Content: "remote"}})
	if got, ok := twoLevel.Get(context.Background(), "key"); !ok || got[0].Content != "remote" {
		t.Fatalf("twoLevel.Get() = %#v/%v, want remote hit", got, ok)
	}
	if got, ok := local.Get(context.Background(), "key"); !ok || got[0].Content != "remote" {
		t.Fatalf("local warmed value = %#v/%v, want remote value", got, ok)
	}
	twoLevel.Set(context.Background(), "next", []Message{{Role: RoleAssistant, Content: "both"}})
	if _, ok := local.Get(context.Background(), "next"); !ok {
		t.Fatal("local next miss after Set, want hit")
	}
	if _, ok := remote.Get(context.Background(), "next"); !ok {
		t.Fatal("remote next miss after Set, want hit")
	}
	maint := twoLevel.(PromptCacheMaintenance)
	if err := maint.Delete(context.Background(), "next"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, ok := remote.Get(context.Background(), "next"); ok {
		t.Fatal("remote next hit after Delete, want miss")
	}
	stats, err := maint.Stats(context.Background())
	if err != nil {
		t.Fatalf("Stats() error = %v", err)
	}
	if stats.Entries != 1 {
		t.Fatalf("Stats().Entries = %d, want 1", stats.Entries)
	}
	if err := maint.Clear(context.Background()); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}
	stats, err = maint.Stats(context.Background())
	if err != nil {
		t.Fatalf("Stats() after clear error = %v", err)
	}
	if stats.Entries != 0 {
		t.Fatalf("Stats() after clear = %d, want 0", stats.Entries)
	}
}

func TestPGVectorConstructorAndSearchFailFastEdges(t *testing.T) {
	t.Parallel()

	_, err := NewPGVectorStore(PGVectorStoreConfig{
		ConnString: "://bad-dsn",
		TableName:  "chunks",
		Dimensions: 3,
	})
	if err == nil || !strings.Contains(err.Error(), "parse pgvector conn string") {
		t.Fatalf("NewPGVectorStore() error = %v, want parse error", err)
	}

	store := &pgVectorStore{cfg: PGVectorStoreConfig{Dimensions: 3}.normalized()}
	tests := []struct {
		name      string
		embedding []float32
		topK      int
		want      string
	}{
		{name: "empty embedding", topK: 1, want: "query embedding is empty"},
		{name: "non-positive top k", embedding: []float32{1, 0, 0}, want: "topK must be > 0"},
		{name: "dimension mismatch", embedding: []float32{1, 0}, topK: 1, want: "do not match"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := store.SearchWithFilter(context.Background(), tt.embedding, tt.topK, 0, SearchFilter{})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("SearchWithFilter() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestRuntimeOpenAIConstructorsValidateConfig(t *testing.T) {
	t.Parallel()

	if _, err := NewOpenAIChatModel(context.Background(), ChatModelConfig{}); err == nil {
		t.Fatal("NewOpenAIChatModel(empty) error = nil, want validation error")
	}
	if _, err := NewOpenAIEmbedder(context.Background(), EmbedderConfig{}); err == nil {
		t.Fatal("NewOpenAIEmbedder(empty) error = nil, want validation error")
	}
}

func TestVectorStoreAdapters(t *testing.T) {
	t.Parallel()

	rootStore := &capturingRootVectorStoreStub{
		hits: []SearchHit{{
			Chunk: ChunkRecord{
				ChunkID:    "doc:0",
				ParentID:   "doc",
				SourcePath: "/kb/doc.md",
				Title:      "doc",
				Text:       "body",
				Metadata:   map[string]string{"team": "search"},
				Embedding:  []float32{1, 0},
			},
			Score: 0.9,
		}},
	}
	internal := rootToInternalVectorStore{inner: rootStore}
	if err := internal.DeleteBySourcePaths(context.Background(), []string{"/kb/doc.md"}); err != nil {
		t.Fatalf("rootToInternal DeleteBySourcePaths() error = %v", err)
	}
	hits, err := internal.Search(context.Background(), []float32{1, 0}, 1, 0)
	if err != nil {
		t.Fatalf("rootToInternal Search() error = %v", err)
	}
	if len(hits) != 1 || hits[0].Chunk.ChunkID != "doc:0" {
		t.Fatalf("rootToInternal Search() hits = %#v, want doc:0", hits)
	}
	filtered, err := internal.SearchWithFilter(context.Background(), []float32{1, 0}, 1, 0, storage.SearchFilter{
		Metadata: map[string]string{"team": "search"},
	})
	if err != nil {
		t.Fatalf("rootToInternal SearchWithFilter() error = %v", err)
	}
	if len(filtered) != 1 {
		t.Fatalf("rootToInternal SearchWithFilter() len = %d, want 1", len(filtered))
	}

	chromem, err := NewChromemVectorStore(ChromemVectorStoreConfig{})
	if err != nil {
		t.Fatalf("NewChromemVectorStore() error = %v", err)
	}
	t.Cleanup(func() {
		if err := chromem.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})
	err = chromem.Upsert(context.Background(), []ChunkRecord{{
		ChunkID:    "doc:0",
		ParentID:   "doc",
		SourcePath: "/kb/doc.md",
		Title:      "doc",
		Text:       "body",
		StartRune:  0,
		EndRune:    4,
		Metadata:   map[string]string{"team": "search"},
		Embedding:  []float32{1, 0},
	}})
	if err != nil {
		t.Fatalf("chromem Upsert() error = %v", err)
	}
	chromemHits, err := chromem.SearchWithFilter(context.Background(), []float32{1, 0}, 1, 0, SearchFilter{
		Metadata: map[string]string{"team": "search"},
	})
	if err != nil {
		t.Fatalf("chromem SearchWithFilter() error = %v", err)
	}
	if len(chromemHits) != 1 || chromemHits[0].Chunk.ChunkID != "doc:0" {
		t.Fatalf("chromem SearchWithFilter() hits = %#v, want doc:0", chromemHits)
	}
	if err := chromem.DeleteBySourcePaths(context.Background(), []string{"/kb/doc.md"}); err != nil {
		t.Fatalf("chromem DeleteBySourcePaths() error = %v", err)
	}
	chromemHits, err = chromem.Search(context.Background(), []float32{1, 0}, 1, 0)
	if err != nil {
		t.Fatalf("chromem Search() error = %v", err)
	}
	if len(chromemHits) != 0 {
		t.Fatalf("chromem Search() len after delete = %d, want 0", len(chromemHits))
	}
}

func TestLongTermMemoryUtilityEdges(t *testing.T) {
	t.Parallel()

	now := time.Now()
	if got := formatLongTermMemoryTimestamp(time.Time{}); got != "" {
		t.Fatalf("formatLongTermMemoryTimestamp(zero) = %q, want empty", got)
	}
	if got := formatLongTermMemoryTimestamp(now); got == "" {
		t.Fatal("formatLongTermMemoryTimestamp(now) = empty, want timestamp")
	}
	if got := trimRunes("abcdef", 3); got != "abc" {
		t.Fatalf("trimRunes(short budget) = %q, want abc", got)
	}
	user, assistant := compressLongTermMemoryTurn("  hello   world  ", "  assistant   reply  ", 0)
	if user != "hello world" || assistant != "assistant reply" {
		t.Fatalf("compressLongTermMemoryTurn() = %q/%q, want normalized text", user, assistant)
	}

	store := NewInMemoryLongTermMemoryStore()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.Store(ctx, []LongTermMemoryRecord{{ID: "m", SessionID: "s", Embedding: []float32{1}}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Store(canceled) error = %v, want context.Canceled", err)
	}
	if _, err := store.Search(ctx, "s", []float32{1}, 1, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("Search(canceled) error = %v, want context.Canceled", err)
	}
	if err := store.ClearSession(ctx, "s"); !errors.Is(err, context.Canceled) {
		t.Fatalf("ClearSession(canceled) error = %v, want context.Canceled", err)
	}
}

func TestRootRetrieverAndProviderTraceEdges(t *testing.T) {
	t.Parallel()

	rootStore := &capturingRootVectorStoreStub{
		hits: []SearchHit{{
			Chunk: ChunkRecord{ChunkID: "doc:0", ParentID: "doc", SourcePath: "/kb/doc.md", Title: "doc", Text: "body", Embedding: []float32{1, 0}},
			Score: 0.9,
		}},
	}
	retriever := NewRetriever(rootStore, &fakeEmbedder{defaultVec: []float32{1, 0}}, nil, RetrieverConfig{
		TopK:                1,
		SimilarityThreshold: 0,
	})
	hits, err := retriever.Search(context.Background(), RetrieverRequest{Query: "body"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("Search() len = %d, want 1", len(hits))
	}

	var calls []ProviderCallTrace
	ctx := withProviderTraceObserver(context.Background(), func(call ProviderCallTrace) {
		calls = append(calls, call)
	})
	emitProviderTrace(ctx, ProviderCallTrace{Provider: "test", Operation: "op"})
	emitProviderTrace(context.Background(), ProviderCallTrace{Provider: "ignored"})
	if len(calls) != 1 || calls[0].Provider != "test" {
		t.Fatalf("provider trace calls = %#v, want single test call", calls)
	}
}

func TestSourceAndGovernorEdgeBranches(t *testing.T) {
	t.Parallel()

	source := &youdaoNoteSource{exportDir: t.TempDir()}
	if err := source.Close(); err != nil {
		t.Fatalf("youdao Close() error = %v", err)
	}
	if source.exportDir != "" {
		t.Fatalf("youdao exportDir = %q, want empty", source.exportDir)
	}
	if err := source.Close(); err != nil {
		t.Fatalf("youdao second Close() error = %v", err)
	}

	breaker := newProviderCircuitBreaker("test", ProviderCircuitBreakerConfig{
		FailureThreshold: 1,
		OpenTimeout:      time.Nanosecond,
		HalfOpenMaxCalls: 1,
	}, nil)
	reservation, _, err := breaker.reserve(time.Now())
	if err != nil {
		t.Fatalf("reserve(closed) error = %v", err)
	}
	reservation.finish(providerErrorClassTransient)
	time.Sleep(time.Millisecond)
	halfOpen, state, err := breaker.reserve(time.Now())
	if err != nil {
		t.Fatalf("reserve(half-open) error = %v", err)
	}
	if state != providerCircuitStateHalfOpen || !halfOpen.halfOpen {
		t.Fatalf("reserve state = %q halfOpen=%v, want half_open reservation", state, halfOpen.halfOpen)
	}
	halfOpen.abort()
	breaker.mu.Lock()
	inFlight := breaker.halfOpenInFlight
	breaker.mu.Unlock()
	if inFlight != 0 {
		t.Fatalf("halfOpenInFlight after abort = %d, want 0", inFlight)
	}

	handle := providerAttemptHandle{reservation: halfOpen}
	handle.abort()
	(providerAttemptHandle{}).abort()
}

func TestTraceSummaryAndPromptCacheFallbackEdges(t *testing.T) {
	t.Parallel()

	summary := SummarizeExecutionTrace(ExecutionTrace{
		SessionID: "s",
		Query:     "q",
		Err:       errors.New("boom"),
		Memory:    []MemoryTrace{{Operation: "retrieve"}},
	})
	if summary.Err != "boom" || summary.MemoryEventCount != 1 {
		t.Fatalf("SummarizeExecutionTrace() = %+v, want err and memory count", summary)
	}

	local := NewInMemoryPromptCacheWithConfig(PromptCacheConfig{MaxEntries: 2})
	twoLevel := NewTwoLevelPromptCache(local, nil)
	twoLevel.Set(context.Background(), "a", []Message{{Role: RoleUser, Content: "local"}})
	stats, err := twoLevel.(PromptCacheMaintenance).Stats(context.Background())
	if err != nil {
		t.Fatalf("Stats() error = %v", err)
	}
	if stats.Entries != 1 {
		t.Fatalf("Stats().Entries = %d, want 1", stats.Entries)
	}
	emptyStats, err := NewTwoLevelPromptCache(nil, nil).(PromptCacheMaintenance).Stats(context.Background())
	if err != nil {
		t.Fatalf("empty Stats() error = %v", err)
	}
	if emptyStats.Entries != 0 {
		t.Fatalf("empty Stats().Entries = %d, want 0", emptyStats.Entries)
	}
}

func TestRedisPromptCacheMaintenanceSuccessEdges(t *testing.T) {
	t.Parallel()

	server := miniredis.RunT(t)
	t.Cleanup(server.Close)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	cache := NewRedisPromptCache(client, RedisPromptCacheConfig{KeyPrefix: "prompt", TTL: time.Minute})
	cache.Set(context.Background(), "a", []Message{{Role: RoleUser, Content: "a"}})
	cache.Set(context.Background(), "b", []Message{{Role: RoleUser, Content: "b"}})
	maint := cache.(PromptCacheMaintenance)
	stats, err := maint.Stats(context.Background())
	if err != nil {
		t.Fatalf("Stats() error = %v", err)
	}
	if stats.Entries != 2 {
		t.Fatalf("Stats().Entries = %d, want 2", stats.Entries)
	}
	if err := maint.Delete(context.Background(), "a"); err != nil {
		t.Fatalf("Delete(a) error = %v", err)
	}
	if _, ok := cache.Get(context.Background(), "a"); ok {
		t.Fatal("Get(a) after Delete hit, want miss")
	}
	if err := maint.Delete(context.Background()); err != nil {
		t.Fatalf("Delete() with no keys error = %v", err)
	}
	if err := maint.Clear(context.Background()); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}
	stats, err = maint.Stats(context.Background())
	if err != nil {
		t.Fatalf("Stats() after Clear error = %v", err)
	}
	if stats.Entries != 0 {
		t.Fatalf("Stats().Entries after Clear = %d, want 0", stats.Entries)
	}
}

func TestStructuredToolAdapterEdgeBranches(t *testing.T) {
	t.Parallel()

	var nilAdapter *StructuredToolAdapter
	if nilAdapter.Name() != "" || nilAdapter.Description() != "" || nilAdapter.Structured() != nil {
		t.Fatal("nil StructuredToolAdapter should return zero values")
	}
	if _, err := nilAdapter.Run(context.Background(), "{}"); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("nil adapter Run() error = %v, want ErrInvalidConfig", err)
	}

	minimum := 1.0
	maximum := 5.0
	schema := ToolSchema{Properties: map[string]ToolParameterSchema{
		"n":      {Type: ToolParameterNumber, Minimum: &minimum, Maximum: &maximum},
		"object": {Type: ToolParameterObject},
		"array":  {Type: ToolParameterArray},
	}}
	tests := []struct {
		name    string
		args    map[string]any
		wantErr bool
	}{
		{name: "float32 number accepted", args: map[string]any{"n": float32(2), "object": map[string]any{}, "array": []any{"x"}}},
		{name: "int64 number accepted", args: map[string]any{"n": int64(3), "object": map[string]any{}, "array": []any{}}},
		{name: "json number accepted", args: map[string]any{"n": json.Number("4"), "object": map[string]any{}, "array": []any{}}},
		{name: "minimum rejected", args: map[string]any{"n": 0}, wantErr: true},
		{name: "maximum rejected", args: map[string]any{"n": 6}, wantErr: true},
		{name: "object type rejected", args: map[string]any{"object": "bad"}, wantErr: true},
		{name: "array type rejected", args: map[string]any{"array": "bad"}, wantErr: true},
		{name: "unsupported type rejected", args: map[string]any{"bad": "x"}, wantErr: true},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			activeSchema := schema
			if tt.name == "unsupported type rejected" {
				activeSchema = ToolSchema{Properties: map[string]ToolParameterSchema{"bad": {Type: ToolParameterType("bad")}}}
			}
			err := ValidateToolArguments(activeSchema, tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateToolArguments() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestStructuredToolFormattingAndDirectLookup(t *testing.T) {
	t.Parallel()

	result := formatToolResult(ToolResult{
		Text: " text ",
		JSON: map[string]any{"ok": true},
	})
	if !strings.Contains(result, "text") || !strings.Contains(result, `"ok":true`) {
		t.Fatalf("formatToolResult() = %q, want text and json", result)
	}
	tool := directStructuredTool{structuredToolFunc: testSearchDocsTool().(structuredToolFunc)}
	if got := structuredToolFromTool(tool); got == nil || got.Name() != tool.Name() {
		t.Fatalf("structuredToolFromTool() = %#v, want direct structured tool", got)
	}
	if got := structuredToolFromTool(registryTestTool{name: "legacy"}); got != nil {
		t.Fatalf("structuredToolFromTool(legacy) = %#v, want nil", got)
	}
}

type directStructuredTool struct {
	structuredToolFunc
}

func (t directStructuredTool) Run(ctx context.Context, input string) (string, error) {
	return NewStructuredToolAdapter(t.structuredToolFunc).Run(ctx, input)
}

func TestInMemoryLongTermMemoryValidationEdges(t *testing.T) {
	t.Parallel()

	store := NewInMemoryLongTermMemoryStore()
	tests := []struct {
		name    string
		records []LongTermMemoryRecord
		want    string
	}{
		{name: "empty batch accepted", records: nil},
		{name: "missing id", records: []LongTermMemoryRecord{{SessionID: "s", Embedding: []float32{1}}}, want: "memory id is required"},
		{name: "missing session", records: []LongTermMemoryRecord{{ID: "m", Embedding: []float32{1}}}, want: "memory session id is required"},
		{name: "missing embedding", records: []LongTermMemoryRecord{{ID: "m", SessionID: "s"}}, want: "memory embedding is required"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := store.Store(context.Background(), tt.records)
			if tt.want != "" {
				if err == nil || !strings.Contains(err.Error(), tt.want) {
					t.Fatalf("Store() error = %v, want containing %q", err, tt.want)
				}
				return
			}
			if err != nil {
				t.Fatalf("Store() error = %v", err)
			}
		})
	}
}

func newJSONTestServer(t *testing.T, status int, body string, captured *map[string]any) string {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if captured != nil {
			var request map[string]any
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("Decode request error = %v", err)
			}
			*captured = request
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server.URL
}

func newHTTPCTestClient() *httpc.Client {
	return httpc.New(httpc.WithTimeout(time.Second))
}

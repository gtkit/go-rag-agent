package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/gtkit/go-rag-agent/internal/storage"
	"github.com/gtkit/go-rag-agent/internal/websearch"
)

type fakeRetriever struct {
	hits []storage.SearchHit
	err  error
}

func (f fakeRetriever) Search(context.Context, string) ([]storage.SearchHit, error) {
	return f.hits, f.err
}

type fakeWebSearcher struct {
	results []websearch.Result
	err     error
}

func (f fakeWebSearcher) Search(context.Context, string) ([]websearch.Result, error) {
	return f.results, f.err
}

func TestRetrievalToolContract(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		hits    []storage.SearchHit
		query   string
		wantAny []string
	}{
		{
			name:  "returns non empty evidence from hits",
			query: "what is rag?",
			hits: []storage.SearchHit{
				{
					Chunk: storage.ChunkRecord{
						SourcePath: "docs/intro.md",
						Text:       "RAG combines retrieval and generation.",
					},
				},
			},
			wantAny: []string{"docs/intro.md", "RAG combines retrieval and generation."},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tool := NewRetrievalTool(fakeRetriever{hits: tt.hits})
			if tool.Name() != "retrieve_context" {
				t.Fatalf("Name() = %q, want %q", tool.Name(), "retrieve_context")
			}
			got, err := tool.Run(context.Background(), tt.query)
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			for _, want := range tt.wantAny {
				if !strings.Contains(got, want) {
					t.Fatalf("Run() = %q, missing %q", got, want)
				}
			}
		})
	}
}

func TestRetrievalToolRunValidateInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      string
		wantErr    bool
		wantErrSub string
	}{
		{
			name:       "rejects blank query",
			input:      "   ",
			wantErr:    true,
			wantErrSub: "query is required",
		},
		{
			name:       "accepts non-empty query",
			input:      "rag",
			wantErr:    false,
			wantErrSub: "",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tool := NewRetrievalTool(fakeRetriever{})
			_, err := tool.Run(context.Background(), tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Run() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErrSub != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErrSub)) {
				t.Fatalf("Run() error = %v, want contains %q", err, tt.wantErrSub)
			}
		})
	}
}

func TestWebSearchToolContract(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		results []websearch.Result
		query   string
		wantAny []string
	}{
		{
			name:  "returns formatted search results",
			query: "what is rag",
			results: []websearch.Result{
				{
					Title:   "Example",
					URL:     "https://example.com",
					Content: "snippet",
				},
			},
			wantAny: []string{"Example", "https://example.com", "snippet"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tool := NewWebSearchTool(fakeWebSearcher{results: tt.results})
			if tool.Name() != "search_web" {
				t.Fatalf("Name() = %q, want %q", tool.Name(), "search_web")
			}
			got, err := tool.Run(context.Background(), tt.query)
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			for _, want := range tt.wantAny {
				if !strings.Contains(got, want) {
					t.Fatalf("Run() = %q, want contains %q", got, want)
				}
			}
		})
	}
}

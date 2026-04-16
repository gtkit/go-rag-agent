package tools

import (
	"context"
	"strings"
	"testing"

	"my-gtkit-package/go-rag-agent/internal/storage"
)

type fakeRetriever struct {
	hits []storage.SearchHit
	err  error
}

func (f fakeRetriever) Search(context.Context, string) ([]storage.SearchHit, error) {
	return f.hits, f.err
}

func TestRetrievalToolInfoName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want string
	}{
		{
			name: "returns retrieve_context name",
			want: "retrieve_context",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tool := NewRetrievalTool(fakeRetriever{})
			info, err := tool.Info(context.Background())
			if err != nil {
				t.Fatalf("Info() error = %v", err)
			}
			if info.Name != tt.want {
				t.Fatalf("Info().Name = %q, want %q", info.Name, tt.want)
			}
		})
	}
}

func TestRetrievalToolInvokableRunReturnsEvidence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		hits    []storage.SearchHit
		query   string
		wantAny []string
	}{
		{
			name:  "returns non empty evidence from hits",
			query: `{"query":"what is rag?"}`,
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
			got, err := tool.InvokableRun(context.Background(), tt.query)
			if err != nil {
				t.Fatalf("InvokableRun() error = %v", err)
			}
			if strings.TrimSpace(got) == "" {
				t.Fatal("InvokableRun() returned empty evidence")
			}
			for _, want := range tt.wantAny {
				if !strings.Contains(got, want) {
					t.Fatalf("InvokableRun() = %q, missing %q", got, want)
				}
			}
		})
	}
}

func TestRetrievalToolInvokableRunValidateInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		argsJSON   string
		wantErr    bool
		wantErrSub string
	}{
		{
			name:       "rejects invalid json",
			argsJSON:   "{bad",
			wantErr:    true,
			wantErrSub: "decode retrieval args",
		},
		{
			name:       "rejects blank query",
			argsJSON:   `{"query":"   "}`,
			wantErr:    true,
			wantErrSub: "query is required",
		},
		{
			name:       "accepts non-empty query",
			argsJSON:   `{"query":"rag"}`,
			wantErr:    false,
			wantErrSub: "",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tool := NewRetrievalTool(fakeRetriever{})
			_, err := tool.InvokableRun(context.Background(), tt.argsJSON)
			if (err != nil) != tt.wantErr {
				t.Fatalf("InvokableRun() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErrSub != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErrSub)) {
				t.Fatalf("InvokableRun() error = %v, want contains %q", err, tt.wantErrSub)
			}
		})
	}
}

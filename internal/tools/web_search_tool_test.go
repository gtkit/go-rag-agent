package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/gtkit/go-rag-agent/internal/websearch"
)

type fakeWebSearcher struct {
	results []websearch.Result
	err     error
}

func (f fakeWebSearcher) Search(context.Context, string) ([]websearch.Result, error) {
	return f.results, f.err
}

func TestWebSearchToolInvokableRun(t *testing.T) {
	t.Parallel()

	tool := NewWebSearchTool(fakeWebSearcher{
		results: []websearch.Result{
			{
				Title:   "Example",
				URL:     "https://example.com",
				Content: "snippet",
			},
		},
	})

	got, err := tool.InvokableRun(context.Background(), `{"query":"what is rag"}`)
	if err != nil {
		t.Fatalf("InvokableRun() error = %v", err)
	}
	for _, want := range []string{"Example", "https://example.com", "snippet"} {
		if !strings.Contains(got, want) {
			t.Fatalf("InvokableRun() = %q, want contains %q", got, want)
		}
	}
}

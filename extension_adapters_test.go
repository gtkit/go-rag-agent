package ragagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gtkit/httpc"
)

type testDocumentConverter struct {
	name        string
	supports    map[string]bool
	supportsErr error
	convertErr  error
	calls       []string
}

func (c *testDocumentConverter) Name() string { return c.name }

func (c *testDocumentConverter) Supports(_ context.Context, path string) (bool, error) {
	if c.supportsErr != nil {
		return false, c.supportsErr
	}
	return c.supports[filepath.Ext(path)], nil
}

func (c *testDocumentConverter) Convert(_ context.Context, path string, title string, metadata map[string]string) (Document, error) {
	c.calls = append(c.calls, path)
	if c.convertErr != nil {
		return Document{}, c.convertErr
	}
	return Document{
		SourcePath: path,
		Title:      title,
		Content:    "converted document body",
		Metadata:   metadata,
	}, nil
}

func TestAddKnowledgeUsesDocumentConverters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		pathName     string
		converters   []*testDocumentConverter
		wantErr      error
		wantCalls    []int
		wantTextPart string
	}{
		{
			name:     "converter handles default unsupported extension",
			pathName: "guide.docx",
			converters: []*testDocumentConverter{
				{name: "skip", supports: map[string]bool{".docx": false}},
				{name: "docx", supports: map[string]bool{".docx": true}},
			},
			wantCalls:    []int{0, 1},
			wantTextPart: "converted document body",
		},
		{
			name:     "converter failure is wrapped",
			pathName: "bad.docx",
			converters: []*testDocumentConverter{
				{name: "docx", supports: map[string]bool{".docx": true}, convertErr: errors.New("converter down")},
			},
			wantErr:   errors.New("converter down"),
			wantCalls: []int{1},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			path := filepath.Join(root, tc.pathName)
			writeTestFile(t, path, "placeholder")

			store := &fakeStore{}
			converters := make([]DocumentConverter, 0, len(tc.converters))
			for _, converter := range tc.converters {
				converters = append(converters, converter)
			}
			agent := &Agent{
				cfg: Config{
					DocumentConverters: converters,
				},
				store:    store,
				embedder: &fakeEmbedder{defaultVec: []float32{0.1, 0.2, 0.3}},
				chunker:  mustNewChunkerForTest(t, 128, 0),
				sessions: make(map[string]*Session),
			}

			err := agent.AddKnowledge(t.Context(), ConvertibleFileSource(path))
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) && !strings.Contains(fmt.Sprint(err), tc.wantErr.Error()) {
					t.Fatalf("AddKnowledge() error = %v, want containing %v", err, tc.wantErr)
				}
			} else if err != nil {
				t.Fatalf("AddKnowledge() error = %v", err)
			}

			for i, converter := range tc.converters {
				if got := len(converter.calls); got != tc.wantCalls[i] {
					t.Fatalf("converter %d call count = %d, want %d", i, got, tc.wantCalls[i])
				}
			}
			if tc.wantTextPart == "" {
				return
			}
			store.mu.Lock()
			defer store.mu.Unlock()
			if len(store.upsertBatches) != 1 || len(store.upsertBatches[0]) == 0 {
				t.Fatalf("upsert batches = %v, want non-empty", store.upsertBatches)
			}
			if got := store.upsertBatches[0][0].Text; !strings.Contains(got, tc.wantTextPart) {
				t.Fatalf("upsert text = %q, want containing %q", got, tc.wantTextPart)
			}
		})
	}
}

func TestCommandDocumentConverter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		script    string
		wantText  string
		wantErr   string
		omitInput bool
	}{
		{
			name: "runs command and loads output",
			script: `#!/bin/sh
printf 'converted from %s' "$1" > "$2"
`,
			wantText: "converted from",
		},
		{
			name: "command failure returns error",
			script: `#!/bin/sh
exit 7
`,
			wantErr: "convert document",
		},
		{
			name:      "missing input placeholder is invalid",
			script:    "#!/bin/sh\nexit 0\n",
			omitInput: true,
			wantErr:   "{input}",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			inputPath := filepath.Join(root, "guide.docx")
			writeTestFile(t, inputPath, "docx")
			scriptPath := filepath.Join(root, "convert.sh")
			writeExecutableFile(t, scriptPath, tc.script)

			args := []string{"{input}", "{output}"}
			if tc.omitInput {
				args = []string{"{output}"}
			}
			converter, err := NewCommandDocumentConverter(CommandDocumentConverterConfig{
				Name:       "docx-command",
				Extensions: []string{".docx"},
				Command:    scriptPath,
				Args:       args,
			})
			if tc.wantErr != "" && err != nil {
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("NewCommandDocumentConverter() error = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewCommandDocumentConverter() error = %v", err)
			}

			supported, err := converter.Supports(t.Context(), inputPath)
			if err != nil {
				t.Fatalf("Supports() error = %v", err)
			}
			if !supported {
				t.Fatal("Supports() = false, want true")
			}

			doc, err := converter.Convert(t.Context(), inputPath, "guide", map[string]string{"team": "docs"})
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("Convert() error = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Convert() error = %v", err)
			}
			if !strings.Contains(doc.Content, tc.wantText) {
				t.Fatalf("doc content = %q, want containing %q", doc.Content, tc.wantText)
			}
			if doc.SourcePath != inputPath || doc.Title != "guide" || doc.Metadata["team"] != "docs" {
				t.Fatalf("doc = %+v, want source/title/metadata preserved", doc)
			}
		})
	}
}

func TestOpenAIReranker(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
		response   string
		wantOrder  []string
		wantErr    string
	}{
		{
			name:       "reranks by returned indexes",
			statusCode: http.StatusOK,
			response:   `{"results":[{"index":1,"score":0.91},{"index":0,"score":0.42}]}`,
			wantOrder:  []string{"b", "a"},
		},
		{
			name:       "invalid index returns error",
			statusCode: http.StatusOK,
			response:   `{"results":[{"index":9,"score":0.1}]}`,
			wantErr:    "invalid rerank index",
		},
		{
			name:       "http failure returns error",
			statusCode: http.StatusBadGateway,
			response:   `{}`,
			wantErr:    "unexpected status",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var gotRequest struct {
				Model     string   `json:"model"`
				Query     string   `json:"query"`
				Documents []string `json:"documents"`
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer test-key" {
					t.Fatalf("Authorization = %q, want bearer", r.Header.Get("Authorization"))
				}
				if err := json.NewDecoder(r.Body).Decode(&gotRequest); err != nil {
					t.Fatalf("Decode request error = %v", err)
				}
				w.WriteHeader(tc.statusCode)
				_, _ = w.Write([]byte(tc.response))
			}))
			defer server.Close()

			reranker, err := NewOpenAIReranker(httpc.New(httpc.WithTimeout(time.Second)), OpenAIRerankerConfig{
				BaseURL: server.URL,
				APIKey:  "test-key",
				Model:   "rerank-model",
			})
			if err != nil {
				t.Fatalf("NewOpenAIReranker() error = %v", err)
			}

			candidates := []SearchHit{
				{Chunk: ChunkRecord{ChunkID: "a", Text: "alpha"}, Score: 0.1},
				{Chunk: ChunkRecord{ChunkID: "b", Text: "bravo"}, Score: 0.2},
			}
			got, err := reranker.Rerank(t.Context(), "query", candidates, RerankOptions{ShortlistSize: 2, TopK: 2})
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("Rerank() error = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Rerank() error = %v", err)
			}
			if gotRequest.Model != "rerank-model" || gotRequest.Query != "query" || !slices.Equal(gotRequest.Documents, []string{"alpha", "bravo"}) {
				t.Fatalf("request = %+v, want model/query/documents", gotRequest)
			}
			gotOrder := make([]string, 0, len(got))
			for _, hit := range got {
				gotOrder = append(gotOrder, hit.Chunk.ChunkID)
			}
			if !slices.Equal(gotOrder, tc.wantOrder) {
				t.Fatalf("reranked order = %v, want %v", gotOrder, tc.wantOrder)
			}
		})
	}
}

func TestMCPTool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
		response   string
		wantText   string
		wantErr    string
	}{
		{
			name:       "returns text result",
			statusCode: http.StatusOK,
			response:   `{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"tool result"}]}}`,
			wantText:   "tool result",
		},
		{
			name:       "rpc error is wrapped",
			statusCode: http.StatusOK,
			response:   `{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"tool failed"}}`,
			wantErr:    "tool failed",
		},
		{
			name:       "http error is wrapped",
			statusCode: http.StatusServiceUnavailable,
			response:   `{}`,
			wantErr:    "unexpected status",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var gotRequest struct {
				Method string         `json:"method"`
				Params map[string]any `json:"params"`
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&gotRequest); err != nil {
					t.Fatalf("Decode request error = %v", err)
				}
				w.WriteHeader(tc.statusCode)
				_, _ = w.Write([]byte(tc.response))
			}))
			defer server.Close()

			tool, err := NewMCPTool(httpc.New(httpc.WithTimeout(time.Second)), MCPToolConfig{
				Name:        "search_docs",
				Description: "Search docs",
				Endpoint:    server.URL,
				Method:      "tools/call",
				ToolName:    "search_docs",
			})
			if err != nil {
				t.Fatalf("NewMCPTool() error = %v", err)
			}

			got, err := tool.Run(t.Context(), `{"query":"gateway"}`)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("Run() error = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if got != tc.wantText {
				t.Fatalf("Run() = %q, want %q", got, tc.wantText)
			}
			if gotRequest.Method != "tools/call" || gotRequest.Params["name"] != "search_docs" {
				t.Fatalf("request = %+v, want tools/call search_docs", gotRequest)
			}
		})
	}
}

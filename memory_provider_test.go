package ragagent

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/gtkit/go-rag-agent/internal/graph"
	"github.com/gtkit/go-rag-agent/internal/memory"
	"github.com/gtkit/go-rag-agent/internal/storage"
	"github.com/gtkit/go-rag-agent/internal/telemetry"
)

type fakeMemoryProvider struct {
	mu           sync.Mutex
	retrieveText string
	retrieveErr  error
	memorizeErr  error
	retrieveReqs []MemoryRetrieveRequest
	memorizeReqs []MemoryMemorizeRequest
}

func (p *fakeMemoryProvider) Retrieve(_ context.Context, req MemoryRetrieveRequest) (MemoryRetrieveResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.retrieveReqs = append(p.retrieveReqs, req)
	if p.retrieveErr != nil {
		return MemoryRetrieveResult{}, p.retrieveErr
	}
	return MemoryRetrieveResult{MemoryText: p.retrieveText}, nil
}

func (p *fakeMemoryProvider) Memorize(_ context.Context, req MemoryMemorizeRequest) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.memorizeReqs = append(p.memorizeReqs, req)
	return p.memorizeErr
}

func (p *fakeMemoryProvider) Close() error { return nil }

func TestMemoryProviderInjectsAndMemorizes(t *testing.T) {
	t.Parallel()

	provider := &fakeMemoryProvider{retrieveText: "User likes production-grade Go."}
	runner := &fakeRunner{answer: "answer"}
	agent := newMemoryProviderTestAgent(provider, runner)
	session := agent.GetSession("session-1")

	answer, err := session.AskWithOptions(context.Background(), "what should I optimize?", QueryOptions{
		MemoryScope: MemoryScope{UserID: "user-1", Tenant: "tenant-a"},
	})
	if err != nil {
		t.Fatalf("AskWithOptions() error = %v", err)
	}
	if answer.Text != "answer" {
		t.Fatalf("answer.Text = %q, want answer", answer.Text)
	}
	if !strings.Contains(runner.lastReq.LongTermMemoryText, "production-grade Go") {
		t.Fatalf("runner memory text = %q, want provider memory", runner.lastReq.LongTermMemoryText)
	}

	provider.mu.Lock()
	defer provider.mu.Unlock()
	if got := len(provider.retrieveReqs); got != 1 {
		t.Fatalf("retrieve calls = %d, want 1", got)
	}
	if got := provider.retrieveReqs[0].Scope.UserID; got != "user-1" {
		t.Fatalf("retrieve scope user = %q, want user-1", got)
	}
	if got := len(provider.memorizeReqs); got != 1 {
		t.Fatalf("memorize calls = %d, want 1", got)
	}
	if got := provider.memorizeReqs[0].Assistant; got != "answer" {
		t.Fatalf("memorize assistant = %q, want answer", got)
	}
}

func TestMemoryProviderRetrieveFailurePolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		policy    MemoryFailurePolicy
		wantErr   bool
		wantTrace bool
	}{
		{name: "fail closed returns error", policy: MemoryFailurePolicyFailClosed, wantErr: true, wantTrace: true},
		{name: "fail open degrades to empty memory", policy: MemoryFailurePolicyFailOpen, wantErr: false, wantTrace: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			provider := &fakeMemoryProvider{retrieveErr: errors.New("memory backend down")}
			recorder := &memoryTraceRecorderStub{}
			agent := newMemoryProviderTestAgent(provider, &fakeRunner{answer: "answer"})
			agent.cfg.Memory.ProviderFailurePolicy = tt.policy
			agent.cfg.TraceRecorder = recorder

			_, err := agent.GetSession("session-1").Ask(context.Background(), "question")
			if (err != nil) != tt.wantErr {
				t.Fatalf("Ask() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantTrace {
				traces := recorder.snapshot()
				if len(traces) != 1 {
					t.Fatalf("trace count = %d, want 1", len(traces))
				}
				if len(traces[0].Memory) == 0 {
					t.Fatalf("trace memory events empty, want retrieve failure")
				}
			}
		})
	}
}

func TestMemoryProviderDoesNotMemorizeFailedAnswer(t *testing.T) {
	t.Parallel()

	provider := &fakeMemoryProvider{}
	agent := newMemoryProviderTestAgent(provider, &fakeRunner{err: errors.New("model down")})

	_, err := agent.GetSession("session-1").Ask(context.Background(), "question")
	if err == nil {
		t.Fatal("Ask() error = nil, want non-nil")
	}

	provider.mu.Lock()
	defer provider.mu.Unlock()
	if got := len(provider.memorizeReqs); got != 0 {
		t.Fatalf("memorize calls = %d, want 0", got)
	}
}

func newMemoryProviderTestAgent(provider MemoryProvider, runner *fakeRunner) *Agent {
	searchHits := []storage.SearchHit{{
		Chunk: storage.ChunkRecord{
			ChunkID:    "doc:0",
			ParentID:   "doc",
			SourcePath: "doc.md",
			Title:      "doc",
			Text:       "evidence",
			EndRune:    len("evidence"),
		},
		Score: 0.9,
	}}
	return &Agent{
		cfg: Config{
			ChatModel:         "chat-test",
			MaxHistoryRounds:  8,
			MaxToolCalls:      4,
			MaxPromptTokens:   4096,
			MaxHistoryTokens:  1024,
			MaxEvidenceTokens: 2048,
			MaxMemoryTokens:   512,
			MaxSummaryTokens:  256,
			Memory: MemoryComponents{
				Provider:              provider,
				ProviderFailurePolicy: MemoryFailurePolicyFailClosed,
			},
		},
		store:      &fakeStore{searchHits: searchHits},
		embedder:   &fakeEmbedder{defaultVec: []float32{1, 2, 3}},
		runner:     runner,
		dispatcher: telemetry.NewDispatcher(nil),
		sessions:   make(map[string]*Session),
	}
}

type memoryTraceRecorderStub struct {
	mu     sync.Mutex
	traces []ExecutionTrace
}

func (r *memoryTraceRecorderStub) OnExecutionTrace(_ context.Context, trace ExecutionTrace) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.traces = append(r.traces, trace)
}

func (r *memoryTraceRecorderStub) snapshot() []ExecutionTrace {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]ExecutionTrace(nil), r.traces...)
}

func TestLongTermMemoryProviderAdapter(t *testing.T) {
	t.Parallel()

	store := NewInMemoryLongTermMemoryStore()
	embedder := &fakeEmbedder{defaultVec: []float32{1, 0}}
	provider := NewLongTermMemoryProvider(store, embedder, LongTermMemoryProviderConfig{
		TopK:      3,
		Threshold: 0,
	})

	if err := provider.Memorize(context.Background(), MemoryMemorizeRequest{
		SessionID: "session-1",
		Scope:     MemoryScope{UserID: "user-1", Tenant: "tenant-a"},
		User:      "remember Go",
		Assistant: "Go is relevant",
	}); err != nil {
		t.Fatalf("Memorize() error = %v", err)
	}

	result, err := provider.Retrieve(context.Background(), MemoryRetrieveRequest{
		SessionID: "session-1",
		Scope:     MemoryScope{UserID: "user-1", Tenant: "tenant-a"},
		Query:     "Go",
	})
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}
	if !strings.Contains(result.MemoryText, "remember Go") {
		t.Fatalf("MemoryText = %q, want stored memory", result.MemoryText)
	}
}

func TestLongTermMemoryProviderBoundaryCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		make    func() MemoryProvider
		req     MemoryRetrieveRequest
		wantErr bool
	}{
		{
			name: "nil store returns empty result",
			make: func() MemoryProvider {
				return NewLongTermMemoryProvider(nil, &fakeEmbedder{defaultVec: []float32{1}}, LongTermMemoryProviderConfig{})
			},
			req: MemoryRetrieveRequest{Query: "rag"},
		},
		{
			name: "blank query returns empty result",
			make: func() MemoryProvider {
				return NewLongTermMemoryProvider(NewInMemoryLongTermMemoryStore(), &fakeEmbedder{defaultVec: []float32{1}}, LongTermMemoryProviderConfig{})
			},
			req: MemoryRetrieveRequest{Query: "   "},
		},
		{
			name: "embed retrieve error is returned",
			make: func() MemoryProvider {
				return NewLongTermMemoryProvider(NewInMemoryLongTermMemoryStore(), &fakeEmbedder{err: errors.New("embed down")}, LongTermMemoryProviderConfig{})
			},
			req:     MemoryRetrieveRequest{Query: "rag"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := tt.make().Retrieve(context.Background(), tt.req)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Retrieve() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && strings.TrimSpace(got.MemoryText) != "" {
				t.Fatalf("Retrieve() MemoryText = %q, want empty", got.MemoryText)
			}
		})
	}
}

func TestLongTermMemoryProviderMemorizeErrorsAndClose(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		make    func() MemoryProvider
		wantErr bool
	}{
		{
			name: "nil store memorize is no-op",
			make: func() MemoryProvider {
				return NewLongTermMemoryProvider(nil, &fakeEmbedder{defaultVec: []float32{1}}, LongTermMemoryProviderConfig{})
			},
		},
		{
			name: "embed memorize error is returned",
			make: func() MemoryProvider {
				return NewLongTermMemoryProvider(NewInMemoryLongTermMemoryStore(), &fakeEmbedder{err: errors.New("embed down")}, LongTermMemoryProviderConfig{})
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			provider := tt.make()
			err := provider.Memorize(context.Background(), MemoryMemorizeRequest{
				SessionID: "session-1",
				User:      "question",
				Assistant: "answer",
			})
			if (err != nil) != tt.wantErr {
				t.Fatalf("Memorize() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err := provider.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}
		})
	}
}

var _ graph.Runner = (*fakeRunner)(nil)
var _ = memory.Turn{}

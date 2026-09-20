package ragagent

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/gtkit/go-rag-agent/internal/graph"
	"github.com/gtkit/go-rag-agent/internal/memory"
	"github.com/gtkit/go-rag-agent/internal/storage"
)

type fakeHistoryStore struct {
	mu        sync.Mutex
	turns     map[string][]HistoryTurn
	loadErr   error
	appendErr error
	ops       []string
}

func (s *fakeHistoryStore) Load(_ context.Context, sessionID string) ([]HistoryTurn, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ops = append(s.ops, "load")
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	return append([]HistoryTurn(nil), s.turns[sessionID]...), nil
}

func (s *fakeHistoryStore) Append(_ context.Context, sessionID string, turn HistoryTurn) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ops = append(s.ops, "append")
	if s.appendErr != nil {
		return s.appendErr
	}
	if s.turns == nil {
		s.turns = map[string][]HistoryTurn{}
	}
	s.turns[sessionID] = append(s.turns[sessionID], turn)
	return nil
}

func (s *fakeHistoryStore) Clear(_ context.Context, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ops = append(s.ops, "clear")
	delete(s.turns, sessionID)
	return nil
}

func (s *fakeHistoryStore) snapshot(sessionID string) ([]HistoryTurn, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]HistoryTurn(nil), s.turns[sessionID]...), strings.Join(s.ops, ",")
}

func newHistoryStoreAgent(store HistoryStore, policy MemoryFailurePolicy, runner graph.Runner) *Agent {
	return &Agent{
		cfg: Config{
			ChatModel:           "history-model",
			TopK:                1,
			SimilarityThreshold: 0.5,
			MaxHistoryRounds:    2,
			ReasoningEffort:     "high",
			Memory:              MemoryComponents{HistoryStore: store, HistoryFailurePolicy: policy},
		},
		store: &fakeStore{searchHits: []storage.SearchHit{{
			Chunk: storage.ChunkRecord{ChunkID: "doc:0", SourcePath: "/tmp/doc.md", Title: "doc", Text: "gateway api", EndRune: 11},
			Score: 0.99,
		}}},
		embedder: &fakeEmbedder{defaultVec: []float32{1, 0}},
		runner:   runner,
		sessions: make(map[string]*Session),
	}
}

func memoryOperations(trace *ExecutionTrace) string {
	if trace == nil {
		return ""
	}
	ops := make([]string, 0, len(trace.Memory))
	for _, entry := range trace.Memory {
		status := "ok"
		if !entry.Success {
			status = "err"
		}
		ops = append(ops, entry.Operation+":"+status)
	}
	return strings.Join(ops, ",")
}

func TestSessionExternalHistoryLoadAndAppend(t *testing.T) {
	t.Parallel()

	store := &fakeHistoryStore{turns: map[string][]HistoryTurn{"s": {
		{User: "q0", Assistant: "a0"},
		{User: "q1", Assistant: "a1"},
		{User: "q2", Assistant: "a2"},
	}}}
	runner := &fakeRunner{answer: "answer"}
	agent := newHistoryStoreAgent(store, "", runner)
	session := agent.GetSession("s")

	answer, err := session.Ask(context.Background(), "what about it?")
	if err != nil {
		t.Fatalf("Ask() error = %v", err)
	}

	runner.mu.Lock()
	req := runner.lastReq
	runner.mu.Unlock()
	if len(req.History) != 2 || req.History[0].User != "q1" || req.History[1].User != "q2" {
		t.Fatalf("history must be trimmed to the last MaxHistoryRounds turns, got %+v", req.History)
	}
	if req.ReasoningEffort != "high" {
		t.Fatalf("ReasoningEffort = %q, want high", req.ReasoningEffort)
	}
	if answer.Trace.RewrittenQuery != "q2 what about it?" {
		t.Fatalf("follow-up rewrite must use loaded history, got %q", answer.Trace.RewrittenQuery)
	}
	turns, ops := store.snapshot("s")
	if len(turns) != 4 || turns[3] != (HistoryTurn{User: "what about it?", Assistant: "answer"}) {
		t.Fatalf("store turns = %+v", turns)
	}
	if ops != "load,append" {
		t.Fatalf("store ops = %q", ops)
	}
	if len(session.history.Turns()) != 0 {
		t.Fatalf("in-process history must stay unused in external mode: %+v", session.history.Turns())
	}
	if got := memoryOperations(answer.Trace); got != "history_load:ok,history_append:ok" {
		t.Fatalf("trace memory ops = %q", got)
	}
}

func TestSessionExternalHistoryStreamAppends(t *testing.T) {
	t.Parallel()

	store := &fakeHistoryStore{}
	runner := &fakeStreamingRunner{askStreamFn: func(_ context.Context, _ graph.Request, emit graph.StreamEmitter) error {
		if err := emit(graph.Event{Type: graph.EventAnswerChunk, Content: "streamed", Step: 1}); err != nil {
			return err
		}
		return emit(graph.Event{Type: graph.EventDone, Step: 1})
	}}
	agent := newHistoryStoreAgent(store, "", runner)
	if err := agent.GetSession("s").AskStream(context.Background(), "hello", func(StreamEvent) error { return nil }); err != nil {
		t.Fatalf("AskStream() error = %v", err)
	}
	turns, _ := store.snapshot("s")
	if len(turns) != 1 || turns[0].Assistant != "streamed" {
		t.Fatalf("stream path must append the turn, got %+v", turns)
	}
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if runner.lastAskStreamReq.ReasoningEffort != "high" {
		t.Fatalf("stream request ReasoningEffort = %q, want high", runner.lastAskStreamReq.ReasoningEffort)
	}
}

func TestSessionExternalHistoryClear(t *testing.T) {
	t.Parallel()

	t.Run("idle session clears the store immediately", func(t *testing.T) {
		t.Parallel()
		store := &fakeHistoryStore{turns: map[string][]HistoryTurn{"s": {{User: "q", Assistant: "a"}}}}
		agent := newHistoryStoreAgent(store, "", &fakeRunner{answer: "x"})
		if err := agent.GetSession("s").ClearHistory(context.Background()); err != nil {
			t.Fatalf("ClearHistory() error = %v", err)
		}
		turns, ops := store.snapshot("s")
		if len(turns) != 0 || ops != "clear" {
			t.Fatalf("turns = %+v ops = %q", turns, ops)
		}
	})

	t.Run("clear during execution discards the in-flight turn", func(t *testing.T) {
		t.Parallel()
		store := &fakeHistoryStore{}
		agent := newHistoryStoreAgent(store, "", &fakeRunner{answer: "x"})
		session := agent.GetSession("s")
		session.mu.Lock()
		session.executing = true
		session.mu.Unlock()
		if err := session.ClearHistory(context.Background()); err != nil {
			t.Fatalf("ClearHistory() error = %v", err)
		}
		session.mu.Lock()
		session.executing = false
		session.mu.Unlock()
		if _, err := session.Ask(context.Background(), "q"); err != nil {
			t.Fatalf("Ask() error = %v", err)
		}
		turns, ops := store.snapshot("s")
		if len(turns) != 0 || ops != "load,clear" {
			t.Fatalf("turns = %+v ops = %q, want the pending clear to replace the append", turns, ops)
		}
	})

	t.Run("pending clear after a failed execution is flushed without changing the error", func(t *testing.T) {
		t.Parallel()
		store := &fakeHistoryStore{}
		runErr := errors.New("model down")
		agent := newHistoryStoreAgent(store, "", &fakeRunner{err: runErr})
		session := agent.GetSession("s")
		session.mu.Lock()
		session.executing = true
		session.mu.Unlock()
		if err := session.ClearHistory(context.Background()); err != nil {
			t.Fatalf("ClearHistory() error = %v", err)
		}
		session.mu.Lock()
		session.executing = false
		session.mu.Unlock()
		_, err := session.Ask(context.Background(), "q")
		if !errors.Is(err, runErr) {
			t.Fatalf("Ask() error = %v, want run error", err)
		}
		if _, ops := store.snapshot("s"); ops != "load,clear" {
			t.Fatalf("ops = %q, want the pending clear flushed after failure", ops)
		}
	})

	t.Run("session without agent keeps in-process semantics", func(t *testing.T) {
		t.Parallel()
		history := memory.NewHistory(8)
		history.Append("q", "a")
		session := &Session{history: history}
		if err := session.ClearHistory(context.Background()); err != nil || len(session.history.Turns()) != 0 {
			t.Fatalf("ClearHistory() = %v, turns = %d", err, len(session.history.Turns()))
		}
	})
}

func TestSessionExternalHistoryFailurePolicy(t *testing.T) {
	t.Parallel()

	storeErr := errors.New("redis down")
	tests := []struct {
		name     string
		store    *fakeHistoryStore
		policy   MemoryFailurePolicy
		wantErr  bool
		wantOps  string
		wantMems string
	}{
		{
			name:     "load failure fails closed by default",
			store:    &fakeHistoryStore{loadErr: storeErr},
			wantErr:  true,
			wantOps:  "load",
			wantMems: "history_load:err",
		},
		{
			name:     "load failure fails open with empty history",
			store:    &fakeHistoryStore{loadErr: storeErr},
			policy:   MemoryFailurePolicyFailOpen,
			wantOps:  "load,append",
			wantMems: "history_load:err,history_append:ok",
		},
		{
			name:     "append failure fails closed by default",
			store:    &fakeHistoryStore{appendErr: storeErr},
			wantErr:  true,
			wantOps:  "load,append",
			wantMems: "history_load:ok,history_append:err",
		},
		{
			name:     "append failure is ignored when failing open",
			store:    &fakeHistoryStore{appendErr: storeErr},
			policy:   MemoryFailurePolicyFailOpen,
			wantOps:  "load,append",
			wantMems: "history_load:ok,history_append:err",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			recorder := &recordingTraceRecorder{}
			agent := newHistoryStoreAgent(tt.store, tt.policy, &fakeRunner{answer: "x"})
			agent.cfg.TraceRecorder = recorder
			answer, err := agent.GetSession("f").Ask(context.Background(), "q")
			if tt.wantErr {
				if !errors.Is(err, storeErr) {
					t.Fatalf("Ask() error = %v, want store error", err)
				}
			} else if err != nil {
				t.Fatalf("Ask() error = %v", err)
			}
			if _, ops := tt.store.snapshot("f"); ops != tt.wantOps {
				t.Fatalf("ops = %q, want %q", ops, tt.wantOps)
			}
			trace := answer.Trace
			if trace == nil {
				trace = recorder.last()
			}
			if got := memoryOperations(trace); got != tt.wantMems {
				t.Fatalf("trace memory ops = %q, want %q", got, tt.wantMems)
			}
		})
	}
}

type recordingTraceRecorder struct {
	mu     sync.Mutex
	traces []ExecutionTrace
}

func (r *recordingTraceRecorder) OnExecutionTrace(_ context.Context, trace ExecutionTrace) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.traces = append(r.traces, trace)
}

func (r *recordingTraceRecorder) last() *ExecutionTrace {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.traces) == 0 {
		return nil
	}
	trace := r.traces[len(r.traces)-1]
	return &trace
}

func TestRedisHistoryStore(t *testing.T) {
	t.Parallel()

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()

	t.Run("constructor validation", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name   string
			client redis.Cmdable
			cfg    RedisHistoryStoreConfig
		}{
			{name: "nil client", cfg: RedisHistoryStoreConfig{}},
			{name: "negative ttl", client: client, cfg: RedisHistoryStoreConfig{TTL: -time.Second}},
			{name: "negative max rounds", client: client, cfg: RedisHistoryStoreConfig{MaxRounds: -1}},
		}
		for _, tt := range tests {
			if _, err := NewRedisHistoryStore(tt.client, tt.cfg); !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("%s: error = %v, want ErrInvalidConfig", tt.name, err)
			}
		}
	})

	t.Run("append trims to max rounds refreshes ttl and clear deletes", func(t *testing.T) {
		t.Parallel()
		store, err := NewRedisHistoryStore(client, RedisHistoryStoreConfig{MaxRounds: 2, TTL: time.Minute})
		if err != nil {
			t.Fatalf("NewRedisHistoryStore() error = %v", err)
		}
		for _, user := range []string{"a", "b", "c"} {
			if err := store.Append(ctx, "r1", HistoryTurn{User: user, Assistant: user + "!"}); err != nil {
				t.Fatalf("Append(%s) error = %v", user, err)
			}
		}
		turns, err := store.Load(ctx, "r1")
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if len(turns) != 2 || turns[0].User != "b" || turns[1].Assistant != "c!" {
			t.Fatalf("Load() = %+v, want last two turns in order", turns)
		}
		if ttl := server.TTL("ragagent:history:r1"); ttl <= 0 || ttl > time.Minute {
			t.Fatalf("ttl = %v, want within (0, 1m]", ttl)
		}
		if err := store.Clear(ctx, "r1"); err != nil {
			t.Fatalf("Clear() error = %v", err)
		}
		if turns, err := store.Load(ctx, "r1"); err != nil || len(turns) != 0 {
			t.Fatalf("Load() after clear = %+v, %v", turns, err)
		}
	})

	t.Run("no trimming or ttl when unset and custom prefix applies", func(t *testing.T) {
		t.Parallel()
		store, err := NewRedisHistoryStore(client, RedisHistoryStoreConfig{KeyPrefix: "custom"})
		if err != nil {
			t.Fatalf("NewRedisHistoryStore() error = %v", err)
		}
		for _, user := range []string{"a", "b", "c"} {
			if err := store.Append(ctx, "r2", HistoryTurn{User: user}); err != nil {
				t.Fatalf("Append(%s) error = %v", user, err)
			}
		}
		if turns, _ := store.Load(ctx, "r2"); len(turns) != 3 {
			t.Fatalf("Load() = %d turns, want 3", len(turns))
		}
		if ttl := server.TTL("custom:r2"); ttl != 0 {
			t.Fatalf("ttl = %v, want no expiry", ttl)
		}
	})

	t.Run("corrupted entry fails load", func(t *testing.T) {
		t.Parallel()
		store, _ := NewRedisHistoryStore(client, RedisHistoryStoreConfig{})
		if _, err := server.Push("ragagent:history:bad", "not json"); err != nil {
			t.Fatalf("push: %v", err)
		}
		if _, err := store.Load(ctx, "bad"); err == nil {
			t.Fatal("Load() error = nil, want decode error")
		}
	})
}

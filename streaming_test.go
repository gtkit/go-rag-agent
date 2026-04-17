package ragagent

import (
	"context"
	"errors"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"my-gtkit-package/go-rag-agent/internal/graph"
	"my-gtkit-package/go-rag-agent/internal/storage"
	"my-gtkit-package/go-rag-agent/internal/telemetry"
)

type fakeStreamingRunner struct {
	mu sync.Mutex

	askFn       func(context.Context, graph.Request) (string, error)
	askStreamFn func(context.Context, graph.Request, graph.StreamEmitter) error

	lastAskReq       graph.Request
	lastAskStreamReq graph.Request
}

func (f *fakeStreamingRunner) Ask(ctx context.Context, req graph.Request) (string, error) {
	f.mu.Lock()
	f.lastAskReq = req
	fn := f.askFn
	f.mu.Unlock()

	if fn == nil {
		return "", nil
	}
	return fn(ctx, req)
}

func (f *fakeStreamingRunner) AskStream(ctx context.Context, req graph.Request, emit graph.StreamEmitter) error {
	f.mu.Lock()
	f.lastAskStreamReq = req
	fn := f.askStreamFn
	f.mu.Unlock()

	if fn == nil {
		return nil
	}
	return fn(ctx, req, emit)
}

func TestSessionAskStreamSequenceAndRetrievalOwnership(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		searchErr           error
		streamEvents        []graph.Event
		streamErr           error
		wantErr             error
		wantEventTypes      []EventType
		wantRunnerCalled    bool
		wantHistoryLenDelta int
	}{
		{
			name: "success emits lifecycle citation chunks done and appends history",
			streamEvents: []graph.Event{
				{Type: graph.EventAnswerChunk, Content: "hello ", Step: 1},
				{Type: graph.EventAnswerChunk, Content: "world", Step: 1},
				{Type: graph.EventDone, Step: 2},
			},
			streamErr: nil,
			wantErr:   nil,
			wantEventTypes: []EventType{
				EventRetrieveStart,
				EventToolStart,
				EventRetrieveEnd,
				EventToolEnd,
				EventCitation,
				EventAnswerChunk,
				EventAnswerChunk,
				EventDone,
			},
			wantRunnerCalled:    true,
			wantHistoryLenDelta: 1,
		},
		{
			name:      "retrieve failure emits error and returns",
			searchErr: errors.New("store down"),
			wantErr:   errors.New("store down"),
			wantEventTypes: []EventType{
				EventRetrieveStart,
				EventToolStart,
				EventRetrieveEnd,
				EventToolEnd,
				EventError,
			},
			wantRunnerCalled:    false,
			wantHistoryLenDelta: 0,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store := &fakeStore{
				searchHits: []storage.SearchHit{
					{
						Chunk: storage.ChunkRecord{
							ChunkID:    "doc:0",
							SourcePath: "/tmp/doc.md",
							Title:      "doc",
							Text:       "evidence text for streaming",
							StartRune:  0,
							EndRune:    27,
						},
						Score: 0.99,
					},
				},
				searchErr: tc.searchErr,
			}
			embedder := &fakeEmbedder{defaultVec: []float32{1, 2, 3}}
			runner := &fakeStreamingRunner{
				askStreamFn: func(_ context.Context, _ graph.Request, emit graph.StreamEmitter) error {
					for _, event := range tc.streamEvents {
						if err := emit(event); err != nil {
							return err
						}
					}
					return tc.streamErr
				},
			}
			a := &Agent{
				cfg: Config{
					TopK:                5,
					SimilarityThreshold: 0.5,
					MaxHistoryRounds:    8,
				},
				store:    store,
				embedder: embedder,
				runner:   runner,
				sessions: make(map[string]*Session),
			}

			s := a.GetSession("stream-seq")
			beforeTurns := len(s.history.Turns())
			var gotEvents []StreamEvent

			err := s.AskStream(context.Background(), "how does stream work?", func(event StreamEvent) error {
				gotEvents = append(gotEvents, event)
				return nil
			})
			if tc.wantErr != nil {
				if !containsErr(err, tc.wantErr) {
					t.Fatalf("AskStream() error = %v, want contains %v", err, tc.wantErr)
				}
			} else if err != nil {
				t.Fatalf("AskStream() error = %v", err)
			}

			gotTypes := make([]EventType, 0, len(gotEvents))
			for _, event := range gotEvents {
				gotTypes = append(gotTypes, event.Type)
			}
			if !slices.Equal(gotTypes, tc.wantEventTypes) {
				t.Fatalf("event types = %v, want %v", gotTypes, tc.wantEventTypes)
			}

			if tc.wantRunnerCalled {
				runner.mu.Lock()
				lastReq := runner.lastAskStreamReq
				runner.mu.Unlock()
				if lastReq.EvidenceText == "" {
					t.Fatal("runner request EvidenceText should be set by root retrieval path")
				}
				embedder.mu.Lock()
				if len(embedder.lastTexts) == 0 {
					embedder.mu.Unlock()
					t.Fatal("embedder should be invoked by root retrieval path")
				}
				embedder.mu.Unlock()
				store.mu.Lock()
				searchCalls := store.searchCalls
				store.mu.Unlock()
				if searchCalls == 0 {
					t.Fatal("store search should be invoked by root retrieval path")
				}
			}

			afterTurns := len(s.history.Turns())
			if got := afterTurns - beforeTurns; got != tc.wantHistoryLenDelta {
				t.Fatalf("history delta = %d, want %d", got, tc.wantHistoryLenDelta)
			}
		})
	}
}

func TestSessionAskStreamRunnerFailureEmitsErrorAndModelTelemetry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{
			name: "runner error emits event error and model lifecycle telemetry",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			runnerErr := errors.New("model stream failed")
			recorder := &callbackRecorder{}
			runner := &fakeStreamingRunner{
				askStreamFn: func(_ context.Context, _ graph.Request, emit graph.StreamEmitter) error {
					if err := emit(graph.Event{Type: graph.EventAnswerChunk, Content: "partial", Step: 1}); err != nil {
						return err
					}
					return runnerErr
				},
			}
			a := &Agent{
				cfg: Config{
					TopK:                5,
					SimilarityThreshold: 0.5,
					ChatModel:           "chat-test",
					MaxHistoryRounds:    8,
				},
				store: &fakeStore{
					searchHits: []storage.SearchHit{
						{
							Chunk: storage.ChunkRecord{
								ChunkID:    "doc:0",
								SourcePath: "/tmp/doc.md",
								Title:      "doc",
								Text:       "evidence text",
							},
							Score: 0.99,
						},
					},
				},
				embedder:   &fakeEmbedder{defaultVec: []float32{1, 2, 3}},
				runner:     runner,
				dispatcher: telemetry.NewDispatcher([]telemetry.Callback{recorder}),
				sessions:   make(map[string]*Session),
			}
			s := a.GetSession("runner-fail")

			var gotEvents []StreamEvent
			err := s.AskStream(context.Background(), "stream fail", func(event StreamEvent) error {
				gotEvents = append(gotEvents, event)
				return nil
			})
			if !containsErr(err, runnerErr) {
				t.Fatalf("AskStream() error = %v, want contains %v", err, runnerErr)
			}

			gotTypes := make([]EventType, 0, len(gotEvents))
			for _, event := range gotEvents {
				gotTypes = append(gotTypes, event.Type)
			}
			wantTypes := []EventType{
				EventRetrieveStart,
				EventToolStart,
				EventRetrieveEnd,
				EventToolEnd,
				EventCitation,
				EventAnswerChunk,
				EventError,
			}
			if !slices.Equal(gotTypes, wantTypes) {
				t.Fatalf("event types = %v, want %v", gotTypes, wantTypes)
			}

			telemetryEvents := recorder.snapshot()
			pos := map[string]int{}
			for i, event := range telemetryEvents {
				if _, ok := pos[event]; !ok {
					pos[event] = i
				}
			}
			for _, must := range []string{"model_start", "model_end"} {
				if _, ok := pos[must]; !ok {
					t.Fatalf("missing telemetry %q in %v", must, telemetryEvents)
				}
			}
			if !(pos["model_start"] < pos["model_end"]) {
				t.Fatalf("model telemetry order invalid: %v", telemetryEvents)
			}
		})
	}
}

func TestSessionAskStreamEmitterPanicReturnsError(t *testing.T) {
	t.Parallel()

	runner := &fakeStreamingRunner{
		askStreamFn: func(_ context.Context, _ graph.Request, emit graph.StreamEmitter) error {
			if err := emit(graph.Event{Type: graph.EventAnswerChunk, Content: "chunk", Step: 1}); err != nil {
				return err
			}
			return emit(graph.Event{Type: graph.EventDone, Step: 2})
		},
	}
	a := &Agent{
		cfg: Config{
			TopK:                5,
			SimilarityThreshold: 0.5,
			MaxHistoryRounds:    8,
		},
		store: &fakeStore{
			searchHits: []storage.SearchHit{
				{
					Chunk: storage.ChunkRecord{
						ChunkID:    "doc:0",
						SourcePath: "/tmp/doc.md",
						Title:      "doc",
						Text:       "evidence text",
					},
					Score: 0.99,
				},
			},
		},
		embedder: &fakeEmbedder{defaultVec: []float32{1, 2, 3}},
		runner:   runner,
		sessions: make(map[string]*Session),
	}
	s := a.GetSession("panic-emitter")

	err := s.AskStream(context.Background(), "stream", func(event StreamEvent) error {
		if event.Type == EventAnswerChunk {
			panic("emitter panic")
		}
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "callback panic") {
		t.Fatalf("AskStream() error = %v, want callback panic error", err)
	}
	if got := len(s.history.Turns()); got != 0 {
		t.Fatalf("history turns = %d, want 0 after emitter panic", got)
	}
}

func TestSessionAskStreamSameSessionSerialization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{
			name: "ask and askstream do not execute runner concurrently for one session",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var active atomic.Int32
			var overlap atomic.Bool

			runner := &fakeStreamingRunner{
				askFn: func(_ context.Context, _ graph.Request) (string, error) {
					if active.Add(1) > 1 {
						overlap.Store(true)
					}
					time.Sleep(80 * time.Millisecond)
					active.Add(-1)
					return "sync answer", nil
				},
				askStreamFn: func(_ context.Context, _ graph.Request, emit graph.StreamEmitter) error {
					if active.Add(1) > 1 {
						overlap.Store(true)
					}
					time.Sleep(80 * time.Millisecond)
					active.Add(-1)
					if err := emit(graph.Event{Type: graph.EventAnswerChunk, Content: "stream answer"}); err != nil {
						return err
					}
					return emit(graph.Event{Type: graph.EventDone})
				},
			}
			a := &Agent{
				cfg: Config{
					TopK:                5,
					SimilarityThreshold: 0.5,
					MaxHistoryRounds:    8,
				},
				store: &fakeStore{
					searchHits: []storage.SearchHit{
						{
							Chunk: storage.ChunkRecord{
								ChunkID:    "doc:0",
								SourcePath: "/tmp/doc.md",
								Title:      "doc",
								Text:       "evidence text",
							},
							Score: 0.99,
						},
					},
				},
				embedder: &fakeEmbedder{defaultVec: []float32{1, 2, 3}},
				runner:   runner,
				sessions: make(map[string]*Session),
			}
			s := a.GetSession("serialize")

			errCh := make(chan error, 2)
			go func() {
				errCh <- s.AskStream(context.Background(), "stream question", func(StreamEvent) error { return nil })
			}()
			go func() {
				_, err := s.Ask(context.Background(), "sync question")
				errCh <- err
			}()

			for range 2 {
				select {
				case err := <-errCh:
					if err != nil {
						t.Fatalf("concurrent operation error = %v", err)
					}
				case <-time.After(3 * time.Second):
					t.Fatal("concurrent operations timed out")
				}
			}

			if overlap.Load() {
				t.Fatal("runner methods overlapped for same session; expected serialized execution")
			}
			if got := len(s.history.Turns()); got != 2 {
				t.Fatalf("history turns = %d, want 2", got)
			}
		})
	}
}

func TestSessionAskStreamQueuedRequestHonorsContextCancellation(t *testing.T) {
	t.Parallel()

	block := make(chan struct{})
	started := make(chan struct{}, 1)
	runner := &fakeStreamingRunner{
		askStreamFn: func(ctx context.Context, _ graph.Request, emit graph.StreamEmitter) error {
			select {
			case started <- struct{}{}:
			default:
			}
			select {
			case <-block:
				if err := emit(graph.Event{Type: graph.EventDone, Step: 1}); err != nil {
					return err
				}
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	}
	a := &Agent{
		cfg: Config{
			TopK:                5,
			SimilarityThreshold: 0.5,
			MaxHistoryRounds:    8,
		},
		store: &fakeStore{
			searchHits: []storage.SearchHit{
				{
					Chunk: storage.ChunkRecord{
						ChunkID:    "doc:0",
						SourcePath: "/tmp/doc.md",
						Title:      "doc",
						Text:       "evidence text",
					},
					Score: 0.99,
				},
			},
		},
		embedder: &fakeEmbedder{defaultVec: []float32{1, 2, 3}},
		runner:   runner,
		sessions: make(map[string]*Session),
	}
	s := a.GetSession("queued-cancel")

	firstDone := make(chan error, 1)
	go func() {
		firstDone <- s.AskStream(context.Background(), "first", func(StreamEvent) error { return nil })
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("first AskStream did not start")
	}

	ctx, cancel := context.WithCancel(context.Background())
	secondDone := make(chan error, 1)
	go func() {
		secondDone <- s.AskStream(ctx, "second", func(StreamEvent) error { return nil })
	}()

	cancel()

	select {
	case err := <-secondDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("queued AskStream error = %v, want %v", err, context.Canceled)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("queued AskStream did not honor context cancellation promptly")
	}

	close(block)

	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatalf("first AskStream error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first AskStream did not finish after unblock")
	}
}

func TestSessionAskStreamCancellationNoLeak(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{
			name: "askstream returns cancellation and no goroutine leak",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			started := make(chan struct{}, 1)
			var workerWG sync.WaitGroup

			runner := &fakeStreamingRunner{
				askStreamFn: func(ctx context.Context, _ graph.Request, _ graph.StreamEmitter) error {
					select {
					case started <- struct{}{}:
					default:
					}

					workerWG.Add(1)
					done := make(chan struct{})
					go func() {
						defer workerWG.Done()
						<-ctx.Done()
						close(done)
					}()

					<-done
					return ctx.Err()
				},
			}
			a := &Agent{
				cfg: Config{
					TopK:                5,
					SimilarityThreshold: 0.5,
					MaxHistoryRounds:    8,
				},
				store: &fakeStore{
					searchHits: []storage.SearchHit{
						{
							Chunk: storage.ChunkRecord{
								ChunkID:    "doc:0",
								SourcePath: "/tmp/doc.md",
								Title:      "doc",
								Text:       "evidence text",
							},
							Score: 0.99,
						},
					},
				},
				embedder: &fakeEmbedder{defaultVec: []float32{1, 2, 3}},
				runner:   runner,
				sessions: make(map[string]*Session),
			}
			s := a.GetSession("cancel")

			baseline := runtime.NumGoroutine()
			ctx, cancel := context.WithCancel(context.Background())

			errCh := make(chan error, 1)
			eventsCh := make(chan []StreamEvent, 1)
			go func() {
				var gotEvents []StreamEvent
				errCh <- s.AskStream(ctx, "cancel me", func(event StreamEvent) error {
					gotEvents = append(gotEvents, event)
					return nil
				})
				eventsCh <- gotEvents
			}()

			select {
			case <-started:
			case <-time.After(2 * time.Second):
				t.Fatal("runner AskStream did not start")
			}
			cancel()

			select {
			case err := <-errCh:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("AskStream() error = %v, want context.Canceled", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("AskStream() did not return after cancellation")
			}
			gotEvents := <-eventsCh
			if len(gotEvents) == 0 || gotEvents[len(gotEvents)-1].Type != EventError {
				t.Fatalf("expected final event type %q on cancellation, got %v", EventError, gotEvents)
			}

			workerDone := make(chan struct{})
			go func() {
				workerWG.Wait()
				close(workerDone)
			}()
			select {
			case <-workerDone:
			case <-time.After(2 * time.Second):
				t.Fatal("runner worker goroutine did not exit after cancellation")
			}

			deadline := time.Now().Add(2 * time.Second)
			for time.Now().Before(deadline) {
				if runtime.NumGoroutine() <= baseline+2 {
					return
				}
				time.Sleep(20 * time.Millisecond)
			}
			t.Fatalf("goroutines did not settle near baseline=%d, current=%d", baseline, runtime.NumGoroutine())
		})
	}
}

func TestSessionAskStreamEmitterCanMutateSessionStateWithoutDeadlock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		callback        func(*Session, StreamEvent) error
		assertAfterDone func(*testing.T, *Session)
	}{
		{
			name: "emitter clear history is applied after stream completion",
			callback: func(s *Session, event StreamEvent) error {
				if event.Type == EventAnswerChunk {
					return s.ClearHistory(context.Background())
				}
				return nil
			},
			assertAfterDone: func(t *testing.T, s *Session) {
				t.Helper()
				if got := len(s.history.Turns()); got != 0 {
					t.Fatalf("history turns = %d, want 0 after pending clear", got)
				}
			},
		},
		{
			name: "emitter close is applied after stream completion",
			callback: func(s *Session, event StreamEvent) error {
				if event.Type == EventAnswerChunk {
					return s.Close()
				}
				return nil
			},
			assertAfterDone: func(t *testing.T, s *Session) {
				t.Helper()
				if _, err := s.Ask(context.Background(), "next"); !errors.Is(err, ErrSessionClosed) {
					t.Fatalf("Ask() error after callback close = %v, want %v", err, ErrSessionClosed)
				}
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			runner := &fakeStreamingRunner{
				askStreamFn: func(_ context.Context, _ graph.Request, emit graph.StreamEmitter) error {
					if err := emit(graph.Event{Type: graph.EventAnswerChunk, Content: "chunk", Step: 1}); err != nil {
						return err
					}
					return emit(graph.Event{Type: graph.EventDone, Step: 2})
				},
			}
			a := &Agent{
				cfg: Config{
					TopK:                5,
					SimilarityThreshold: 0.5,
					MaxHistoryRounds:    8,
				},
				store: &fakeStore{
					searchHits: []storage.SearchHit{
						{
							Chunk: storage.ChunkRecord{
								ChunkID:    "doc:0",
								SourcePath: "/tmp/doc.md",
								Title:      "doc",
								Text:       "evidence text",
							},
							Score: 0.99,
						},
					},
				},
				embedder: &fakeEmbedder{defaultVec: []float32{1, 2, 3}},
				runner:   runner,
				sessions: make(map[string]*Session),
			}
			s := a.GetSession("callback-state")

			done := make(chan error, 1)
			go func() {
				done <- s.AskStream(context.Background(), "state mutate", func(event StreamEvent) error {
					return tc.callback(s, event)
				})
			}()

			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("AskStream() error = %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("AskStream() deadlocked while emitter mutated session state")
			}

			tc.assertAfterDone(t, s)
		})
	}
}

func TestSessionAskStreamCallbackReentryFailsFast(t *testing.T) {
	t.Parallel()

	runner := &fakeStreamingRunner{
		askStreamFn: func(_ context.Context, _ graph.Request, emit graph.StreamEmitter) error {
			if err := emit(graph.Event{Type: graph.EventAnswerChunk, Content: "chunk", Step: 1}); err != nil {
				return err
			}
			return emit(graph.Event{Type: graph.EventDone, Step: 2})
		},
	}
	a := &Agent{
		cfg: Config{
			TopK:                5,
			SimilarityThreshold: 0.5,
			MaxHistoryRounds:    8,
		},
		store: &fakeStore{
			searchHits: []storage.SearchHit{
				{
					Chunk: storage.ChunkRecord{
						ChunkID:    "doc:0",
						SourcePath: "/tmp/doc.md",
						Title:      "doc",
						Text:       "evidence text",
					},
					Score: 0.99,
				},
			},
		},
		embedder: &fakeEmbedder{defaultVec: []float32{1, 2, 3}},
		runner:   runner,
		sessions: make(map[string]*Session),
	}
	s := a.GetSession("callback-reentry")

	err := s.AskStream(context.Background(), "stream", func(event StreamEvent) error {
		if event.Type != EventAnswerChunk {
			return nil
		}
		_, callErr := s.Ask(context.Background(), "reenter")
		if !errors.Is(callErr, errSessionCallbackReentry) {
			t.Fatalf("reentrant Ask() error = %v, want %v", callErr, errSessionCallbackReentry)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("AskStream() error = %v", err)
	}
}

func TestSessionAskStreamCallbackAgentCloseFailsFast(t *testing.T) {
	t.Parallel()

	runner := &fakeStreamingRunner{
		askStreamFn: func(_ context.Context, _ graph.Request, emit graph.StreamEmitter) error {
			if err := emit(graph.Event{Type: graph.EventAnswerChunk, Content: "chunk", Step: 1}); err != nil {
				return err
			}
			return emit(graph.Event{Type: graph.EventDone, Step: 2})
		},
	}
	a := &Agent{
		cfg: Config{
			TopK:                5,
			SimilarityThreshold: 0.5,
			MaxHistoryRounds:    8,
		},
		store: &fakeStore{
			searchHits: []storage.SearchHit{
				{
					Chunk: storage.ChunkRecord{
						ChunkID:    "doc:0",
						SourcePath: "/tmp/doc.md",
						Title:      "doc",
						Text:       "evidence text",
					},
					Score: 0.99,
				},
			},
		},
		embedder: &fakeEmbedder{defaultVec: []float32{1, 2, 3}},
		runner:   runner,
		sessions: make(map[string]*Session),
	}
	s := a.GetSession("callback-close")

	err := s.AskStream(context.Background(), "stream", func(event StreamEvent) error {
		if event.Type != EventAnswerChunk {
			return nil
		}
		callErr := a.Close()
		if callErr == nil || !strings.Contains(callErr.Error(), "callback") {
			t.Fatalf("Agent.Close() error = %v, want callback-related error", callErr)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("AskStream() error = %v", err)
	}
}

func TestAskTelemetryCallbackReentryFailsFast(t *testing.T) {
	t.Parallel()

	var (
		closeErr error
		askErr   error
	)
	recorder := &telemetryReentryCallback{
		onRetrieveStart: func(a *Agent, s *Session) {
			closeErr = a.Close()
			_, askErr = s.Ask(context.Background(), "reenter")
		},
	}
	store := &fakeStore{
		searchHits: []storage.SearchHit{
			{
				Chunk: storage.ChunkRecord{
					ChunkID:    "doc:0",
					SourcePath: "/tmp/doc.md",
					Title:      "doc",
					Text:       "evidence text",
				},
				Score: 0.99,
			},
		},
	}
	a := &Agent{
		cfg: Config{
			TopK:                5,
			SimilarityThreshold: 0.5,
			ChatModel:           "chat-test",
			MaxHistoryRounds:    8,
		},
		store:      store,
		embedder:   &fakeEmbedder{defaultVec: []float32{1, 2, 3}},
		runner:     &fakeStreamingRunner{askFn: func(_ context.Context, _ graph.Request) (string, error) { return "ok", nil }},
		dispatcher: telemetry.NewDispatcher([]telemetry.Callback{recorder}),
		sessions:   make(map[string]*Session),
	}
	s := a.GetSession("telemetry-reentry")
	recorder.agent = a
	recorder.session = s

	answer, err := s.Ask(context.Background(), "what is this?")
	if err != nil {
		t.Fatalf("Ask() error = %v", err)
	}
	if answer.Text != "ok" {
		t.Fatalf("Ask() text = %q, want %q", answer.Text, "ok")
	}
	if closeErr == nil || !strings.Contains(closeErr.Error(), "callback") {
		t.Fatalf("Agent.Close() error = %v, want callback-related error", closeErr)
	}
	if !errors.Is(askErr, errSessionCallbackReentry) {
		t.Fatalf("reentrant Ask() error = %v, want %v", askErr, errSessionCallbackReentry)
	}
}

func TestTelemetryCallbackOnOneSessionDoesNotBlockOtherSession(t *testing.T) {
	t.Parallel()

	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var blocked atomic.Bool
	recorder := &telemetryReentryCallback{
		onRetrieveStart: func(_ *Agent, _ *Session) {
			if !blocked.CompareAndSwap(false, true) {
				return
			}
			select {
			case started <- struct{}{}:
			default:
			}
			<-release
		},
	}
	store := &fakeStore{
		searchHits: []storage.SearchHit{
			{
				Chunk: storage.ChunkRecord{
					ChunkID:    "doc:0",
					SourcePath: "/tmp/doc.md",
					Title:      "doc",
					Text:       "evidence text",
				},
				Score: 0.99,
			},
		},
	}
	a := &Agent{
		cfg: Config{
			TopK:                5,
			SimilarityThreshold: 0.5,
			ChatModel:           "chat-test",
			MaxHistoryRounds:    8,
		},
		store:      store,
		embedder:   &fakeEmbedder{defaultVec: []float32{1, 2, 3}},
		runner:     &fakeStreamingRunner{askFn: func(_ context.Context, _ graph.Request) (string, error) { return "ok", nil }},
		sessions:   make(map[string]*Session),
		dispatcher: telemetry.NewDispatcher([]telemetry.Callback{recorder}),
	}
	s1 := a.GetSession("telemetry-a")
	s2 := a.GetSession("telemetry-b")
	recorder.agent = a
	recorder.session = s1

	doneA := make(chan error, 1)
	go func() {
		_, err := s1.Ask(context.Background(), "what is this?")
		doneA <- err
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("first telemetry callback did not start")
	}

	doneB := make(chan error, 1)
	go func() {
		_, err := s2.Ask(context.Background(), "other session")
		doneB <- err
	}()

	select {
	case err := <-doneB:
		if err != nil {
			t.Fatalf("session B Ask() error = %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("session B Ask() was blocked by session A telemetry callback")
	}

	close(release)
	select {
	case err := <-doneA:
		if err != nil {
			t.Fatalf("session A Ask() error = %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("session A Ask() did not finish after releasing telemetry callback")
	}
}

type telemetryReentryCallback struct {
	agent           *Agent
	session         *Session
	onRetrieveStart func(a *Agent, s *Session)
}

func (c *telemetryReentryCallback) OnRetrieveStart(context.Context, string) {
	if c.onRetrieveStart != nil {
		c.onRetrieveStart(c.agent, c.session)
	}
}

func (c *telemetryReentryCallback) OnRetrieveEnd(context.Context, int, error) {}
func (c *telemetryReentryCallback) OnToolStart(context.Context, string)       {}
func (c *telemetryReentryCallback) OnToolEnd(context.Context, string, error)  {}
func (c *telemetryReentryCallback) OnModelStart(context.Context, string)      {}
func (c *telemetryReentryCallback) OnModelEnd(context.Context, string, error) {}

func containsErr(got error, want error) bool {
	if got == nil || want == nil {
		return false
	}
	return errors.Is(got, want) || strings.Contains(got.Error(), want.Error())
}

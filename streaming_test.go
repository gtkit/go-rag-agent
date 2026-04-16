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
			go func() {
				errCh <- s.AskStream(ctx, "cancel me", func(StreamEvent) error { return nil })
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

func containsErr(got error, want error) bool {
	if got == nil || want == nil {
		return false
	}
	return errors.Is(got, want) || strings.Contains(got.Error(), want.Error())
}

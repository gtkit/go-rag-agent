package ragagent

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"my-gtkit-package/go-rag-agent/internal/memory"
	"my-gtkit-package/go-rag-agent/internal/rag"
	"my-gtkit-package/go-rag-agent/internal/storage"
)

// Session stores one conversation state bound to an Agent.
type Session struct {
	agent        *Agent
	id           string
	execMu       sync.Mutex
	mu           sync.Mutex
	history      *memory.History
	closed       bool
	executing    bool
	emitting     bool
	pendingClear bool
	pendingClose bool

	// beforeAskLock is test-only and runs after operation admission, before execution lock.
	beforeAskLock func()
}

var errSessionCallbackReentry = errors.New("ragagent: session callback reentry is not supported")

// Ask executes the synchronous ask pipeline for this session.
func (s *Session) Ask(ctx context.Context, query string) (Answer, error) {
	if s.isEmittingCallback() {
		return Answer{}, errSessionCallbackReentry
	}
	if err := s.agent.beginOperation(); err != nil {
		return Answer{}, err
	}
	defer s.agent.endOperation()

	if s.beforeAskLock != nil {
		s.beforeAskLock()
	}

	s.execMu.Lock()
	defer s.execMu.Unlock()
	if err := s.beginExecution(); err != nil {
		return Answer{}, err
	}

	answer, err := s.agent.askLocked(ctx, s, query)
	s.endExecution(query, answer.Text, err == nil)
	if err != nil {
		return Answer{}, err
	}
	return answer, nil
}

// AskStream executes the streaming ask pipeline for this session.
func (s *Session) AskStream(ctx context.Context, query string, emit func(StreamEvent) error) error {
	if s.isEmittingCallback() {
		return errSessionCallbackReentry
	}
	if err := s.agent.beginOperation(); err != nil {
		return err
	}
	defer s.agent.endOperation()

	if s.beforeAskLock != nil {
		s.beforeAskLock()
	}

	s.execMu.Lock()
	defer s.execMu.Unlock()
	if err := s.beginExecution(); err != nil {
		return err
	}
	answerText, err := s.agent.askStreamLocked(ctx, s, query, emit)
	s.endExecution(query, answerText, err == nil)
	return err
}

// ClearHistory removes all stored turns for this session.
func (s *Session) ClearHistory(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}
	if s.closed {
		return ErrSessionClosed
	}
	if s.executing {
		s.pendingClear = true
		return nil
	}
	s.history.Clear()
	return nil
}

// Close marks this session closed and prevents further asks.
func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.executing {
		s.pendingClose = true
		return nil
	}
	s.closed = true
	return nil
}

func (s *Session) beginExecution() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrSessionClosed
	}
	s.executing = true
	return nil
}

func (s *Session) isEmittingCallback() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.emitting
}

func (s *Session) endExecution(query string, answer string, appendHistory bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if appendHistory {
		s.history.Append(query, answer)
	}
	if s.pendingClear {
		s.history.Clear()
		s.pendingClear = false
	}
	if s.pendingClose {
		s.closed = true
		s.pendingClose = false
	}
	s.executing = false
}

// AddKnowledge ingests one source by loading, chunking, embedding, and upserting records.
func (a *Agent) AddKnowledge(ctx context.Context, src KnowledgeSource) error {
	if err := a.beginOperation(); err != nil {
		return err
	}
	defer a.endOperation()

	if err := ctx.Err(); err != nil {
		return err
	}
	if src == nil {
		return fmt.Errorf("knowledge source is nil: %w", ErrUnsupportedSource)
	}

	files, err := src.Resolve(ctx)
	if err != nil {
		return err
	}

	records := make([]storage.ChunkRecord, 0)
	for _, file := range files {
		doc, err := rag.LoadFile(ctx, file.Path, file.Title, file.Metadata)
		if err != nil {
			return fmt.Errorf("load knowledge file %q: %w", file.Path, err)
		}

		chunks := a.chunker.Split(doc)
		if len(chunks) == 0 {
			continue
		}

		texts := make([]string, 0, len(chunks))
		for _, chunk := range chunks {
			texts = append(texts, chunk.Text)
		}
		embeddings, err := a.embedder.EmbedTexts(ctx, texts)
		if err != nil {
			return fmt.Errorf("embed knowledge chunks for %q: %w", file.Path, err)
		}
		if len(embeddings) != len(chunks) {
			return fmt.Errorf("embedding count %d does not match chunk count %d for %q", len(embeddings), len(chunks), file.Path)
		}

		for i, chunk := range chunks {
			records = append(records, storage.ChunkRecord{
				ChunkID:    chunk.ChunkID,
				ParentID:   chunk.ParentID,
				SourcePath: chunk.SourcePath,
				Title:      chunk.Title,
				Text:       chunk.Text,
				StartRune:  chunk.StartRune,
				EndRune:    chunk.EndRune,
				Metadata:   chunk.Metadata,
				Embedding:  embeddings[i],
			})
		}
	}

	if len(records) == 0 {
		return nil
	}
	if err := a.store.Upsert(ctx, records); err != nil {
		return fmt.Errorf("upsert knowledge chunks: %w", err)
	}
	return nil
}

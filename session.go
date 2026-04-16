package ragagent

import (
	"context"
	"fmt"
	"sync"

	"my-gtkit-package/go-rag-agent/internal/memory"
	"my-gtkit-package/go-rag-agent/internal/rag"
	"my-gtkit-package/go-rag-agent/internal/storage"
)

// Session stores one conversation state bound to an Agent.
type Session struct {
	agent   *Agent
	id      string
	mu      sync.Mutex
	history *memory.History
	closed  bool

	// beforeAskLock is test-only and runs after operation admission, before s.mu.Lock.
	beforeAskLock func()
}

// Ask executes the synchronous ask pipeline for this session.
func (s *Session) Ask(ctx context.Context, query string) (Answer, error) {
	if err := s.agent.beginOperation(); err != nil {
		return Answer{}, err
	}
	defer s.agent.endOperation()

	if s.beforeAskLock != nil {
		s.beforeAskLock()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return Answer{}, ErrSessionClosed
	}
	return s.agent.askLocked(ctx, s, query)
}

// AskStream executes the streaming ask pipeline for this session.
func (s *Session) AskStream(ctx context.Context, query string, emit func(StreamEvent) error) error {
	if err := s.agent.beginOperation(); err != nil {
		return err
	}
	defer s.agent.endOperation()

	if s.beforeAskLock != nil {
		s.beforeAskLock()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrSessionClosed
	}
	return s.agent.askStreamLocked(ctx, s, query, emit)
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
	s.history.Clear()
	return nil
}

// Close marks this session closed and prevents further asks.
func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closed = true
	return nil
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

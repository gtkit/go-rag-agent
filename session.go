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

// Session 保存一个绑定到 Agent 的会话状态。
type Session struct {
	agent        *Agent
	id           string
	execOnce     sync.Once
	execSlot     chan struct{}
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

// Ask 执行当前 Session 的同步问答流程。
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

	if err := s.acquireExecutionSlot(ctx); err != nil {
		return Answer{}, err
	}
	defer s.releaseExecutionSlot()
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

// AskStream 执行当前 Session 的流式问答流程。
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

	if err := s.acquireExecutionSlot(ctx); err != nil {
		return err
	}
	defer s.releaseExecutionSlot()
	if err := s.beginExecution(); err != nil {
		return err
	}
	answerText, err := s.agent.askStreamLocked(ctx, s, query, emit)
	s.endExecution(query, answerText, err == nil)
	return err
}

// ClearHistory 清空当前 Session 的历史对话。
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

// Close 将当前 Session 标记为关闭，后续不再接受问答。
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

func (s *Session) acquireExecutionSlot(ctx context.Context) error {
	slot := s.ensureExecutionSlot()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-slot:
		return nil
	}
}

func (s *Session) releaseExecutionSlot() {
	slot := s.ensureExecutionSlot()
	slot <- struct{}{}
}

func (s *Session) ensureExecutionSlot() chan struct{} {
	s.execOnce.Do(func() {
		s.execSlot = make(chan struct{}, 1)
		s.execSlot <- struct{}{}
	})
	return s.execSlot
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

// AddKnowledge 导入一个知识源，执行加载、分块、向量化和写入存储。
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
	if closer, ok := src.(interface{ Close() error }); ok {
		defer closer.Close()
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

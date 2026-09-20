package ragagent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gtkit/go-rag-agent/internal/memory"
	"github.com/gtkit/go-rag-agent/internal/storage"
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
	return s.AskWithOptions(ctx, query, QueryOptions{})
}

// AskWithOptions 执行带查询选项的同步问答流程。
func (s *Session) AskWithOptions(ctx context.Context, query string, opts QueryOptions) (Answer, error) {
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

	answer, err := s.agent.askLocked(ctx, s, query, opts)
	s.endExecution(query, answer.Text, err == nil)
	if err != nil {
		return Answer{}, s.joinPendingHistoryClear(ctx, err)
	}
	return answer, nil
}

// AskStructured 执行同步结构化问答，并将 JSON 结果反序列化到 target。
func (s *Session) AskStructured(ctx context.Context, query string, target any) (StructuredAnswer, error) {
	return s.AskStructuredWithOptions(ctx, query, QueryOptions{}, target)
}

// AskStructuredWithOptions 执行带查询选项的同步结构化问答，并将 JSON 结果反序列化到 target。
func (s *Session) AskStructuredWithOptions(ctx context.Context, query string, opts QueryOptions, target any) (StructuredAnswer, error) {
	if err := validateStructuredTarget(target); err != nil {
		return StructuredAnswer{}, err
	}
	if s.isEmittingCallback() {
		return StructuredAnswer{}, errSessionCallbackReentry
	}
	if err := s.agent.beginOperation(); err != nil {
		return StructuredAnswer{}, err
	}
	defer s.agent.endOperation()

	if s.beforeAskLock != nil {
		s.beforeAskLock()
	}

	if err := s.acquireExecutionSlot(ctx); err != nil {
		return StructuredAnswer{}, err
	}
	defer s.releaseExecutionSlot()
	if err := s.beginExecution(); err != nil {
		return StructuredAnswer{}, err
	}

	answer, err := s.agent.askStructuredLocked(ctx, s, query, opts, target)
	s.endExecution(query, answer.Answer.Text, err == nil)
	if err != nil {
		return answer, s.joinPendingHistoryClear(ctx, err)
	}
	return answer, nil
}

// AskStream 执行当前 Session 的流式问答流程。
func (s *Session) AskStream(ctx context.Context, query string, emit func(StreamEvent) error) error {
	return s.AskStreamWithOptions(ctx, query, QueryOptions{}, emit)
}

// AskStreamWithOptions 执行带查询选项的流式问答流程。
func (s *Session) AskStreamWithOptions(ctx context.Context, query string, opts QueryOptions, emit func(StreamEvent) error) error {
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
	answerText, err := s.agent.askStreamLocked(ctx, s, query, opts, emit)
	s.endExecution(query, answerText, err == nil)
	if err != nil {
		return s.joinPendingHistoryClear(ctx, err)
	}
	return nil
}

// ClearHistory 清空当前 Session 的历史对话。
func (s *Session) ClearHistory(ctx context.Context) error {
	store := s.historyStore()
	clearNow, err := s.markHistoryClear(ctx, store == nil)
	if err != nil || !clearNow {
		return err
	}
	// 外部存储的清空放在锁外执行，避免网络 I/O 阻塞同 Session 的状态操作。
	if err := store.Clear(ctx, s.id); err != nil {
		return fmt.Errorf("clear session history: %w", err)
	}
	return nil
}

// markHistoryClear 在锁内决定清空方式：执行中则挂起为 pendingClear；进程内模式直接清空；
// 外部存储模式返回 clearNow=true 交给调用方在锁外执行。
func (s *Session) markHistoryClear(ctx context.Context, local bool) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return false, err
	}
	if s.closed {
		return false, ErrSessionClosed
	}
	if s.executing {
		s.pendingClear = true
		return false, nil
	}
	if local {
		s.history.Clear()
		return false, nil
	}
	return true, nil
}

// historyStore 返回配置的外部历史存储；Session 未绑定 Agent 时视为进程内模式。
func (s *Session) historyStore() HistoryStore {
	if s.agent == nil {
		return nil
	}
	return s.agent.cfg.Memory.HistoryStore
}

// loadHistory 返回本次问答使用的历史：配置了 HistoryStore 时从外部加载并截取最近 MaxHistoryRounds 轮，
// 否则使用进程内历史。外部加载失败按 HistoryFailurePolicy 处理并记入 trace。
func (s *Session) loadHistory(ctx context.Context, trace *executionTraceBuilder) ([]memory.Turn, error) {
	store := s.historyStore()
	if store == nil {
		return s.history.Turns(), nil
	}
	startedAt := time.Now()
	stored, err := store.Load(ctx, s.id)
	trace.addMemory("history_load", startedAt, err)
	if err != nil {
		if s.agent.cfg.Memory.HistoryFailurePolicy == MemoryFailurePolicyFailOpen {
			return nil, nil
		}
		return nil, fmt.Errorf("load session history: %w", err)
	}
	if limit := s.agent.cfg.MaxHistoryRounds; limit > 0 && len(stored) > limit {
		stored = stored[len(stored)-limit:]
	}
	turns := make([]memory.Turn, 0, len(stored))
	for _, turn := range stored {
		turns = append(turns, memory.Turn{User: turn.User, Assistant: turn.Assistant})
	}
	return turns, nil
}

// commitHistory 在问答成功后把本轮写回外部存储；执行期间调用过 ClearHistory 时改为清空并丢弃本轮，
// 与进程内历史的 pendingClear 语义一致。进程内模式由 endExecution 负责，此处不做事。
func (s *Session) commitHistory(ctx context.Context, query string, answer string, trace *executionTraceBuilder) error {
	store := s.historyStore()
	if store == nil {
		return nil
	}
	s.mu.Lock()
	clear := s.pendingClear
	s.pendingClear = false
	s.mu.Unlock()

	startedAt := time.Now()
	operation := "history_append"
	var err error
	if clear {
		operation = "history_clear"
		err = store.Clear(ctx, s.id)
	} else {
		err = store.Append(ctx, s.id, HistoryTurn{User: query, Assistant: answer})
	}
	trace.addMemory(operation, startedAt, err)
	if err == nil || s.agent.cfg.Memory.HistoryFailurePolicy == MemoryFailurePolicyFailOpen {
		return nil
	}
	return fmt.Errorf("%s: %w", operation, err)
}

// joinPendingHistoryClear 在执行失败后处理挂起的 ClearHistory（外部存储模式）；
// 清空成功时原样返回执行错误，保持调用方对错误类型的判断不变。
func (s *Session) joinPendingHistoryClear(ctx context.Context, err error) error {
	if clearErr := s.flushPendingHistoryClear(ctx); clearErr != nil {
		return errors.Join(err, clearErr)
	}
	return err
}

func (s *Session) flushPendingHistoryClear(ctx context.Context) error {
	store := s.historyStore()
	if store == nil {
		return nil
	}
	s.mu.Lock()
	clear := s.pendingClear && !s.executing
	if clear {
		s.pendingClear = false
	}
	s.mu.Unlock()
	if !clear {
		return nil
	}
	if err := store.Clear(ctx, s.id); err != nil {
		return fmt.Errorf("clear session history: %w", err)
	}
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

	if s.historyStore() == nil {
		if appendHistory {
			s.history.Append(query, answer)
		}
		if s.pendingClear {
			s.history.Clear()
			s.pendingClear = false
		}
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
		defer func() {
			_ = closer.Close()
		}()
	}

	files, err := src.Resolve(ctx)
	if err != nil {
		return err
	}

	records := make([]storage.ChunkRecord, 0)
	currentSourcePaths := make([]string, 0, len(files))
	loader := a.loader
	if loader == nil {
		loader = NewFileDocumentLoader()
	}
	for _, file := range files {
		file, err = a.cfg.AccessBoundary.applyToKnowledgeFile(ctx, file)
		if err != nil {
			return fmt.Errorf("apply access boundary to knowledge file %q: %w", file.Path, err)
		}
		currentSourcePaths = append(currentSourcePaths, file.Path)

		doc, err := loadDocumentWithConverters(ctx, a.cfg.DocumentConverters, loader, file.Path, file.Title, file.Metadata, a.documentLoadOptions())
		if err != nil {
			return fmt.Errorf("load knowledge file %q: %w", file.Path, err)
		}

		chunks := a.chunker.Split(toInternalDocument(doc))
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
				Metadata:   applyNamespaceMetadata(chunk.Metadata, a.cfg.AccessBoundary.Namespace),
				Embedding:  embeddings[i],
			})
		}
	}

	if root, ok := knowledgeDirectoryRoot(src); ok {
		dirSync, err := a.ensureDirectorySync()
		if err != nil {
			return err
		}
		if err := dirSync.Sync(root, currentSourcePaths, func(stalePaths []string) error {
			if len(records) > 0 {
				if err := a.store.Upsert(ctx, records); err != nil {
					return fmt.Errorf("upsert knowledge chunks: %w", err)
				}
			}
			if len(stalePaths) == 0 {
				return nil
			}
			if err := a.store.DeleteBySourcePaths(ctx, stalePaths); err != nil {
				return fmt.Errorf("delete stale knowledge source paths: %w", err)
			}
			return nil
		}); err != nil {
			return err
		}
		return nil
	}

	if len(records) > 0 {
		if err := a.store.Upsert(ctx, records); err != nil {
			return fmt.Errorf("upsert knowledge chunks: %w", err)
		}
	}
	return nil
}

func knowledgeDirectoryRoot(src KnowledgeSource) (string, bool) {
	switch source := src.(type) {
	case dirSource:
		return source.path, true
	case *dirSource:
		return source.path, true
	default:
		return "", false
	}
}

func (a *Agent) documentLoadOptions() DocumentLoadOptions {
	return DocumentLoadOptions{
		PDFOCR:             a.cfg.PDFOCRBridge.extractor(),
		ImageText:          a.cfg.ImageTextBridge.extractor(),
		MinDirectTextRunes: a.cfg.PDFOCRBridge.MinDirectTextRunes,
	}
}

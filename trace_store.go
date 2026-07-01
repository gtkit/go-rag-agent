package ragagent

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"
)

// TraceStatus 表示一次执行的最终状态。
type TraceStatus string

const (
	// TraceStatusSuccess 表示执行成功并产出答案。
	TraceStatusSuccess TraceStatus = "success"
	// TraceStatusFailure 表示执行失败（非拒答类错误）。
	TraceStatusFailure TraceStatus = "failure"
	// TraceStatusRefused 表示执行因证据不足而拒答。
	TraceStatusRefused TraceStatus = "refused"
)

// 默认 query 摘要保留的最大 rune 数，避免持久化整段长 prompt。
const defaultTraceQuerySummaryRunes = 120

// StoredTrace 是一条可查询的持久化执行记录，承载从 ExecutionTrace 抽取的稳定字段。
type StoredTrace struct {
	SessionID             string
	QuerySummary          string
	Status                TraceStatus
	Stream                bool
	Refused               bool
	RefusalReason         RefusalReason
	StartedAt             time.Time
	FinishedAt            time.Time
	Latency               time.Duration
	ToolNames             []string
	ProviderCallCount     int
	InputTokens           int
	OutputTokens          int
	TotalTokens           int
	TotalEstimatedCostUSD float64
	CitationSources       []string
	Err                   string
}

// TraceQuery 描述一次 trace store 查询条件，零值字段表示不约束该维度。
type TraceQuery struct {
	// SessionID 非空时只返回该 session 的记录。
	SessionID string
	// Status 非空时只返回该最终状态的记录。
	Status TraceStatus
	// Since 非零时只返回 StartedAt 不早于该时刻的记录。
	Since time.Time
	// Until 非零时只返回 StartedAt 不晚于该时刻的记录。
	Until time.Time
	// Limit 大于 0 时只返回最近的 Limit 条（按写入顺序倒序）。
	Limit int
}

func (q TraceQuery) validate() error {
	if q.Limit < 0 {
		return fmt.Errorf("trace query: limit %d must not be negative", q.Limit)
	}
	if !q.Since.IsZero() && !q.Until.IsZero() && q.Until.Before(q.Since) {
		return fmt.Errorf("trace query: until %s is before since %s", q.Until, q.Since)
	}
	return nil
}

func (q TraceQuery) matches(record StoredTrace) bool {
	if q.SessionID != "" && record.SessionID != q.SessionID {
		return false
	}
	if q.Status != "" && record.Status != q.Status {
		return false
	}
	if !q.Since.IsZero() && record.StartedAt.Before(q.Since) {
		return false
	}
	if !q.Until.IsZero() && record.StartedAt.After(q.Until) {
		return false
	}
	return true
}

// TraceStore 是一个可查询的持久化 trace store：既实现 TraceRecorder 契约接收
// ExecutionTrace，又支持按条件查询历史执行记录。
type TraceStore interface {
	TraceRecorder
	// Query 按条件返回已持久化的执行记录；查询条件非法或底层存储失败时返回错误。
	Query(ctx context.Context, query TraceQuery) ([]StoredTrace, error)
}

// InMemoryTraceStore 是 TraceStore 的内存实现，带容量上限与 FIFO 淘汰，适合单进程嵌入。
// 它不提供跨进程持久化；需要持久化时由宿主注入自带后端的 TraceStore 实现。
type InMemoryTraceStore struct {
	mu       sync.RWMutex
	capacity int
	records  []StoredTrace
}

// NewInMemoryTraceStore 创建一个内存 trace store。capacity <= 0 时使用默认上限 1024。
func NewInMemoryTraceStore(capacity int) *InMemoryTraceStore {
	if capacity <= 0 {
		capacity = 1024
	}
	return &InMemoryTraceStore{capacity: capacity}
}

// OnExecutionTrace 把一次执行 trace 抽取为 StoredTrace 并写入，超出容量时淘汰最早记录。
func (s *InMemoryTraceStore) OnExecutionTrace(_ context.Context, trace ExecutionTrace) {
	if s == nil {
		return
	}
	record := newStoredTrace(trace)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, record)
	if len(s.records) > s.capacity {
		s.records = slices.Delete(s.records, 0, len(s.records)-s.capacity)
	}
}

// Query 按条件返回记录的副本，按写入顺序倒序（最近的在前）。
func (s *InMemoryTraceStore) Query(_ context.Context, query TraceQuery) ([]StoredTrace, error) {
	if s == nil {
		return nil, fmt.Errorf("trace store is nil")
	}
	if err := query.validate(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]StoredTrace, 0, len(s.records))
	for i := len(s.records) - 1; i >= 0; i-- {
		record := s.records[i]
		if !query.matches(record) {
			continue
		}
		out = append(out, cloneStoredTrace(record))
		if query.Limit > 0 && len(out) == query.Limit {
			break
		}
	}
	return out, nil
}

// Len 返回当前已存储的记录数，便于测试和容量监控。
func (s *InMemoryTraceStore) Len() int {
	if s == nil {
		return 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.records)
}

func newStoredTrace(trace ExecutionTrace) StoredTrace {
	record := StoredTrace{
		SessionID:             trace.SessionID,
		QuerySummary:          summarizeQuery(trace.Query, defaultTraceQuerySummaryRunes),
		Status:                deriveTraceStatus(trace),
		Stream:                trace.Stream,
		Refused:               trace.Refused,
		RefusalReason:         trace.RefusalReason,
		StartedAt:             trace.StartedAt,
		FinishedAt:            trace.FinishedAt,
		Latency:               trace.Duration,
		ProviderCallCount:     len(trace.ProviderCalls),
		TotalEstimatedCostUSD: trace.TotalEstimatedCostUSD,
	}
	for _, call := range trace.ProviderCalls {
		record.InputTokens += call.InputTokens
		record.OutputTokens += call.OutputTokens
		record.TotalTokens += call.TotalTokens
	}
	if len(trace.ToolCalls) > 0 {
		record.ToolNames = make([]string, 0, len(trace.ToolCalls))
		for _, tool := range trace.ToolCalls {
			record.ToolNames = append(record.ToolNames, tool.Name)
		}
	}
	if len(trace.Citations) > 0 {
		record.CitationSources = make([]string, 0, len(trace.Citations))
		for _, citation := range trace.Citations {
			record.CitationSources = append(record.CitationSources, citation.SourcePath)
		}
	}
	if trace.Err != nil {
		record.Err = trace.Err.Error()
	}
	return record
}

// deriveTraceStatus 按"拒答 > 失败 > 成功"的优先级推导最终状态。
func deriveTraceStatus(trace ExecutionTrace) TraceStatus {
	if trace.Refused {
		return TraceStatusRefused
	}
	if !trace.Success {
		return TraceStatusFailure
	}
	return TraceStatusSuccess
}

func summarizeQuery(query string, maxRunes int) string {
	runes := []rune(query)
	if len(runes) <= maxRunes {
		return query
	}
	return string(runes[:maxRunes]) + "…"
}

func cloneStoredTrace(record StoredTrace) StoredTrace {
	record.ToolNames = slices.Clone(record.ToolNames)
	record.CitationSources = slices.Clone(record.CitationSources)
	return record
}

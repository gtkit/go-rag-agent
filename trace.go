package ragagent

import (
	"context"
	"maps"
	"slices"
	"time"
)

// RefusalReason 描述一次问答因证据不足而拒答的结构化原因分类。
// 零值 RefusalNone 表示未拒答。
type RefusalReason string

const (
	// RefusalNone 表示本次执行没有拒答。
	RefusalNone RefusalReason = ""
	// RefusalNoEvidence 表示检索没有产出可用证据。
	// 注意：在默认 vector-only 检索下，被相似度阈值过滤掉的候选在 store 内部即被丢弃、
	// agent 不可观测，因此这类情况也会归类为 RefusalNoEvidence 而非 RefusalLowSimilarity。
	RefusalNoEvidence RefusalReason = "no_evidence"
	// RefusalLowSimilarity 表示检索到候选但其相似度均低于配置阈值而被全部过滤。
	// 仅在能观测到过滤前候选数的检索路径（如 hybrid）下才会被单独识别。
	RefusalLowSimilarity RefusalReason = "low_similarity"
	// RefusalCitationConflict 表示证据之间存在冲突而拒答。
	// 当前 runtime 不自动判定该类别，保留供调用方与评测 fixtures 标注使用。
	RefusalCitationConflict RefusalReason = "citation_conflict"
	// RefusalToolFailure 表示 fallback 工具执行受限或失败导致无法获得可用证据。
	RefusalToolFailure RefusalReason = "tool_failure"
)

// ToolTrace 描述一次工具调用的摘要信息。
type ToolTrace struct {
	Name       string
	StartedAt  time.Time
	FinishedAt time.Time
	Duration   time.Duration
	Err        error
}

// ProviderCallTrace 描述一次 provider 调用摘要。
type ProviderCallTrace struct {
	Provider         string
	Operation        string
	Model            string
	Attempts         int
	Duration         time.Duration
	ThrottleDelay    time.Duration
	CircuitState     string
	ErrorClass       string
	InputTokens      int
	OutputTokens     int
	TotalTokens      int
	EstimatedCostUSD float64
	Err              error
}

// MemoryTrace 描述一次 memory provider 操作摘要。
type MemoryTrace struct {
	Operation  string
	StartedAt  time.Time
	FinishedAt time.Time
	Duration   time.Duration
	Success    bool
	Err        error
}

// ExecutionTrace 描述一次同步或流式问答的聚合执行信息。
type ExecutionTrace struct {
	SessionID             string
	Query                 string
	RewrittenQuery        string
	Filter                RetrievalFilter
	StartedAt             time.Time
	FinishedAt            time.Time
	Duration              time.Duration
	Stream                bool
	Success               bool
	Err                   error
	Retrieval             RetrievalMetrics
	Model                 ModelMetrics
	ToolCalls             []ToolTrace
	ProviderCalls         []ProviderCallTrace
	Memory                []MemoryTrace
	TotalEstimatedCostUSD float64
	PromptCacheHit        bool
	Fallbacks             []FallbackEvent
	Citations             []Citation
	// Refused 标识本次执行是否因证据不足而拒答。
	Refused bool
	// RefusalReason 给出拒答的结构化原因；未拒答时为 RefusalNone。
	RefusalReason RefusalReason
}

// TraceRecorder 接收单次执行结束后的结构化 trace。
type TraceRecorder interface {
	OnExecutionTrace(ctx context.Context, trace ExecutionTrace)
}

type executionTraceBuilder struct {
	trace     ExecutionTrace
	toolIndex map[string]int
}

func newExecutionTraceBuilder(sessionID string, query string, rewrittenQuery string, filter RetrievalFilter, stream bool) *executionTraceBuilder {
	return &executionTraceBuilder{
		trace: ExecutionTrace{
			SessionID:      sessionID,
			Query:          query,
			RewrittenQuery: rewrittenQuery,
			Filter: RetrievalFilter{
				SourcePaths:    slices.Clone(filter.SourcePaths),
				SourcePrefixes: slices.Clone(filter.SourcePrefixes),
				Metadata:       maps.Clone(filter.Metadata),
			},
			StartedAt: time.Now(),
			Stream:    stream,
		},
		toolIndex: make(map[string]int),
	}
}

func (b *executionTraceBuilder) startTool(name string) {
	if b == nil {
		return
	}
	b.trace.ToolCalls = append(b.trace.ToolCalls, ToolTrace{
		Name:      name,
		StartedAt: time.Now(),
	})
	b.toolIndex[name] = len(b.trace.ToolCalls) - 1
}

func (b *executionTraceBuilder) endTool(name string, err error) {
	if b == nil {
		return
	}
	idx, ok := b.toolIndex[name]
	if !ok || idx >= len(b.trace.ToolCalls) {
		return
	}
	b.trace.ToolCalls[idx].FinishedAt = time.Now()
	b.trace.ToolCalls[idx].Duration = b.trace.ToolCalls[idx].FinishedAt.Sub(b.trace.ToolCalls[idx].StartedAt)
	b.trace.ToolCalls[idx].Err = err
}

func (b *executionTraceBuilder) setRetrieval(metrics RetrievalMetrics, fallbacks []FallbackEvent) {
	if b == nil {
		return
	}
	b.trace.Retrieval = metrics
	b.trace.Fallbacks = slices.Clone(fallbacks)
}

func (b *executionTraceBuilder) setModel(metrics ModelMetrics) {
	if b == nil {
		return
	}
	b.trace.Model = metrics
}

func (b *executionTraceBuilder) setCitations(citations []Citation) {
	if b == nil {
		return
	}
	b.trace.Citations = slices.Clone(citations)
}

func (b *executionTraceBuilder) addProviderCall(call ProviderCallTrace) {
	if b == nil {
		return
	}
	b.trace.ProviderCalls = append(b.trace.ProviderCalls, call)
	b.trace.TotalEstimatedCostUSD += call.EstimatedCostUSD
}

func (b *executionTraceBuilder) addMemory(operation string, startedAt time.Time, err error) {
	if b == nil {
		return
	}
	finishedAt := time.Now()
	b.trace.Memory = append(b.trace.Memory, MemoryTrace{
		Operation:  operation,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
		Duration:   finishedAt.Sub(startedAt),
		Success:    err == nil,
		Err:        err,
	})
}

// classifyEvidenceRefusal 在证据不足拒答时，依据检索指标区分低相似度与无证据。
// 仅当能观测到过滤前候选数（RawCandidateCount > 0）时才判定为低相似度，
// 否则保守归类为无证据。
func classifyEvidenceRefusal(metrics RetrievalMetrics) RefusalReason {
	if metrics.RawCandidateCount > 0 {
		return RefusalLowSimilarity
	}
	return RefusalNoEvidence
}

func (b *executionTraceBuilder) setRefusal(reason RefusalReason) {
	if b == nil || reason == RefusalNone {
		return
	}
	b.trace.Refused = true
	b.trace.RefusalReason = reason
}

func (b *executionTraceBuilder) setPromptCacheHit(hit bool) {
	if b == nil {
		return
	}
	b.trace.PromptCacheHit = b.trace.PromptCacheHit || hit
}

func (b *executionTraceBuilder) finish(err error) ExecutionTrace {
	if b == nil {
		return ExecutionTrace{}
	}
	b.trace.FinishedAt = time.Now()
	b.trace.Duration = b.trace.FinishedAt.Sub(b.trace.StartedAt)
	b.trace.Success = err == nil
	b.trace.Err = err
	return b.trace
}

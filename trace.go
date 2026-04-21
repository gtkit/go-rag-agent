package ragagent

import (
	"context"
	"maps"
	"slices"
	"time"
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
	ErrorClass       string
	InputTokens      int
	OutputTokens     int
	TotalTokens      int
	EstimatedCostUSD float64
	Err              error
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
	TotalEstimatedCostUSD float64
	Fallbacks             []FallbackEvent
	Citations             []Citation
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

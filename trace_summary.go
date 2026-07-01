package ragagent

import "time"

// ExecutionTraceSummary 描述一次执行的稳定摘要视图。
type ExecutionTraceSummary struct {
	SessionID             string
	Query                 string
	Stream                bool
	Success               bool
	Duration              time.Duration
	RetrievalHitCount     int
	ToolCallCount         int
	ProviderCallCount     int
	MemoryEventCount      int
	CitationCount         int
	FallbackCount         int
	PromptCacheHit        bool
	TotalEstimatedCostUSD float64
	// Refused 标识本次执行是否因证据不足而拒答。
	Refused bool
	// RefusalReason 给出拒答的结构化原因；未拒答时为 RefusalNone。
	RefusalReason RefusalReason
	Err           string
}

// SummarizeExecutionTrace 生成一个适合日志和回归消费的执行摘要。
func SummarizeExecutionTrace(trace ExecutionTrace) ExecutionTraceSummary {
	summary := ExecutionTraceSummary{
		SessionID:             trace.SessionID,
		Query:                 trace.Query,
		Stream:                trace.Stream,
		Success:               trace.Success,
		Duration:              trace.Duration,
		RetrievalHitCount:     trace.Retrieval.FinalHitCount,
		ToolCallCount:         len(trace.ToolCalls),
		ProviderCallCount:     len(trace.ProviderCalls),
		MemoryEventCount:      len(trace.Memory),
		CitationCount:         len(trace.Citations),
		FallbackCount:         len(trace.Fallbacks),
		PromptCacheHit:        trace.PromptCacheHit,
		TotalEstimatedCostUSD: trace.TotalEstimatedCostUSD,
		Refused:               trace.Refused,
		RefusalReason:         trace.RefusalReason,
	}
	if trace.Err != nil {
		summary.Err = trace.Err.Error()
	}
	return summary
}

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
	Err                   string
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
	}
	if trace.Err != nil {
		summary.Err = trace.Err.Error()
	}
	return summary
}

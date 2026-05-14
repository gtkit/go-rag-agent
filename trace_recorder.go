package ragagent

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"time"
)

// NewMultiTraceRecorder creates a TraceRecorder that fans out traces to all non-nil recorders.
func NewMultiTraceRecorder(recorders ...TraceRecorder) TraceRecorder {
	filtered := make([]TraceRecorder, 0, len(recorders))
	for _, recorder := range recorders {
		if recorder != nil {
			filtered = append(filtered, recorder)
		}
	}
	return multiTraceRecorder{recorders: filtered}
}

type multiTraceRecorder struct {
	recorders []TraceRecorder
}

func (r multiTraceRecorder) OnExecutionTrace(ctx context.Context, trace ExecutionTrace) {
	for _, recorder := range r.recorders {
		func() {
			defer func() {
				_ = recover()
			}()
			recorder.OnExecutionTrace(ctx, trace)
		}()
	}
}

// JSONLTraceRecorder writes one sanitized JSON object per execution trace.
type JSONLTraceRecorder struct {
	mu sync.Mutex
	w  io.Writer
}

// NewJSONLTraceRecorder creates a TraceRecorder that writes sanitized JSONL summaries.
func NewJSONLTraceRecorder(w io.Writer) TraceRecorder {
	return &JSONLTraceRecorder{w: w}
}

// OnExecutionTrace writes one sanitized summary line.
func (r *JSONLTraceRecorder) OnExecutionTrace(_ context.Context, trace ExecutionTrace) {
	if r == nil || r.w == nil {
		return
	}
	record := map[string]any{
		"session_id":          trace.SessionID,
		"stream":              trace.Stream,
		"success":             trace.Success,
		"duration_ms":         trace.Duration.Milliseconds(),
		"retrieval_hit_count": trace.Retrieval.FinalHitCount,
		"tool_call_count":     len(trace.ToolCalls),
		"provider_call_count": len(trace.ProviderCalls),
		"memory_event_count":  len(trace.Memory),
		"citation_count":      len(trace.Citations),
		"fallback_count":      len(trace.Fallbacks),
		"prompt_cache_hit":    trace.PromptCacheHit,
		"recorded_at":         time.Now().UTC().Format(time.RFC3339Nano),
	}
	if trace.Err != nil {
		record["error"] = trace.Err.Error()
	}
	data, err := json.Marshal(record)
	if err != nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	_, _ = r.w.Write(append(data, '\n'))
}

type loggerTraceRecorder struct {
	logger Logger
}

// NewLoggerTraceRecorder creates a TraceRecorder backed by the package Logger interface.
func NewLoggerTraceRecorder(logger Logger) TraceRecorder {
	return loggerTraceRecorder{logger: logger}
}

func (r loggerTraceRecorder) OnExecutionTrace(_ context.Context, trace ExecutionTrace) {
	if r.logger == nil {
		return
	}
	for _, fallback := range trace.Fallbacks {
		r.logger.Warn("ragagent trace fallback",
			"session_id", trace.SessionID,
			"stage", fallback.Stage,
			"fallback_to", fallback.FallbackTo,
			"error", fallback.Err,
		)
	}
	kv := []any{
		"session_id", trace.SessionID,
		"stream", trace.Stream,
		"success", trace.Success,
		"duration", trace.Duration,
		"tool_calls", len(trace.ToolCalls),
		"provider_calls", len(trace.ProviderCalls),
		"memory_events", len(trace.Memory),
		"citations", len(trace.Citations),
		"error", trace.Err,
	}
	if trace.Err != nil || !trace.Success {
		r.logger.Error("ragagent trace complete", kv...)
		return
	}
	r.logger.Info("ragagent trace complete", kv...)
}

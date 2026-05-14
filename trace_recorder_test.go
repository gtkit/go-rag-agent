package ragagent

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

type traceRecorderFunc func(context.Context, ExecutionTrace)

func (f traceRecorderFunc) OnExecutionTrace(ctx context.Context, trace ExecutionTrace) {
	f(ctx, trace)
}

func TestMultiTraceRecorderFanOutAndPanicIsolation(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	calls := make([]string, 0, 2)
	recorder := NewMultiTraceRecorder(
		nil,
		traceRecorderFunc(func(context.Context, ExecutionTrace) {
			mu.Lock()
			defer mu.Unlock()
			calls = append(calls, "first")
		}),
		traceRecorderFunc(func(context.Context, ExecutionTrace) {
			panic("boom")
		}),
		traceRecorderFunc(func(context.Context, ExecutionTrace) {
			mu.Lock()
			defer mu.Unlock()
			calls = append(calls, "last")
		}),
	)

	recorder.OnExecutionTrace(context.Background(), ExecutionTrace{SessionID: "s1"})

	mu.Lock()
	defer mu.Unlock()
	if got, want := strings.Join(calls, ","), "first,last"; got != want {
		t.Fatalf("calls = %q, want %q", got, want)
	}
}

func TestJSONLTraceRecorderWritesSafeSummary(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	recorder := NewJSONLTraceRecorder(&buf)
	trace := ExecutionTrace{
		SessionID:  "s1",
		Query:      "what is rag",
		StartedAt:  time.Unix(10, 0),
		FinishedAt: time.Unix(11, 0),
		Duration:   time.Second,
		Success:    true,
		ToolCalls:  []ToolTrace{{Name: "search_docs"}},
		ProviderCalls: []ProviderCallTrace{{
			Provider: "openai",
			Model:    "gpt",
		}},
		Citations: []Citation{{SourcePath: "/secret/path.md"}},
	}

	recorder.OnExecutionTrace(context.Background(), trace)

	line := strings.TrimSpace(buf.String())
	if line == "" {
		t.Fatal("JSONL output is empty")
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatalf("JSONL output is not valid JSON: %v\n%s", err, line)
	}
	for _, key := range []string{"session_id", "success", "duration_ms", "tool_call_count", "provider_call_count", "citation_count"} {
		if _, ok := got[key]; !ok {
			t.Fatalf("JSONL output missing key %q: %v", key, got)
		}
	}
	for _, forbidden := range []string{`"prompt":`, `"evidence":`, "api_key", "/secret/path.md"} {
		if strings.Contains(line, forbidden) {
			t.Fatalf("JSONL output leaked %q: %s", forbidden, line)
		}
	}
}

type capturingLogger struct {
	mu      sync.Mutex
	infos   []string
	warns   []string
	errors  []string
	entries []map[string]any
}

func (l *capturingLogger) Debug(msg string, kv ...any) {}

func (l *capturingLogger) Info(msg string, kv ...any) {
	l.capture(&l.infos, msg, kv...)
}

func (l *capturingLogger) Warn(msg string, kv ...any) {
	l.capture(&l.warns, msg, kv...)
}

func (l *capturingLogger) Error(msg string, kv ...any) {
	l.capture(&l.errors, msg, kv...)
}

func (l *capturingLogger) capture(bucket *[]string, msg string, kv ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	*bucket = append(*bucket, msg)
	entry := map[string]any{"msg": msg}
	for i := 0; i+1 < len(kv); i += 2 {
		key, _ := kv[i].(string)
		entry[key] = kv[i+1]
	}
	l.entries = append(l.entries, entry)
}

func TestLoggerTraceRecorderUsesExistingLogger(t *testing.T) {
	t.Parallel()

	logger := &capturingLogger{}
	recorder := NewLoggerTraceRecorder(logger)
	recorder.OnExecutionTrace(context.Background(), ExecutionTrace{
		SessionID: "s1",
		Success:   false,
		Err:       context.DeadlineExceeded,
		Fallbacks: []FallbackEvent{{Stage: FallbackStageHybrid, FallbackTo: FallbackTargetVectorOnly}},
	})

	logger.mu.Lock()
	defer logger.mu.Unlock()
	if len(logger.warns) != 1 {
		t.Fatalf("warn count = %d, want 1", len(logger.warns))
	}
	if len(logger.errors) != 1 {
		t.Fatalf("error count = %d, want 1", len(logger.errors))
	}
	if len(logger.infos) != 0 {
		t.Fatalf("info count = %d, want 0 for failed trace", len(logger.infos))
	}
}

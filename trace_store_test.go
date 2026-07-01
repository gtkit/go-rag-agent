package ragagent

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestInMemoryTraceStoreDeriveStatusAndDetail(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	store := NewInMemoryTraceStore(0)
	store.OnExecutionTrace(context.Background(), ExecutionTrace{
		SessionID:             "s1",
		Query:                 "订单 1001 的物流状态",
		StartedAt:             base,
		FinishedAt:            base.Add(2 * time.Second),
		Duration:              2 * time.Second,
		Success:               true,
		ProviderCalls:         []ProviderCallTrace{{InputTokens: 10, OutputTokens: 5, TotalTokens: 15}},
		TotalEstimatedCostUSD: 0.002,
		ToolCalls:             []ToolTrace{{Name: "retrieve_evidence"}},
		Citations:             []Citation{{SourcePath: "/kb/orders.md"}},
	})

	got, err := store.Query(context.Background(), TraceQuery{SessionID: "s1"})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Query() len = %d, want 1", len(got))
	}
	record := got[0]
	if record.Status != TraceStatusSuccess {
		t.Fatalf("Status = %q, want %q", record.Status, TraceStatusSuccess)
	}
	if record.TotalTokens != 15 || record.InputTokens != 10 || record.OutputTokens != 5 {
		t.Fatalf("tokens = (%d,%d,%d), want (10,5,15)", record.InputTokens, record.OutputTokens, record.TotalTokens)
	}
	if record.Latency != 2*time.Second {
		t.Fatalf("Latency = %v, want 2s", record.Latency)
	}
	if len(record.ToolNames) != 1 || record.ToolNames[0] != "retrieve_evidence" {
		t.Fatalf("ToolNames = %v, want [retrieve_evidence]", record.ToolNames)
	}
	if len(record.CitationSources) != 1 || record.CitationSources[0] != "/kb/orders.md" {
		t.Fatalf("CitationSources = %v, want [/kb/orders.md]", record.CitationSources)
	}
}

func TestInMemoryTraceStoreStatusDerivation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		trace ExecutionTrace
		want  TraceStatus
	}{
		{name: "成功", trace: ExecutionTrace{Success: true}, want: TraceStatusSuccess},
		{name: "失败", trace: ExecutionTrace{Success: false, Err: errors.New("boom")}, want: TraceStatusFailure},
		{name: "拒答优先于失败", trace: ExecutionTrace{Success: false, Refused: true, RefusalReason: RefusalNoEvidence, Err: ErrEvidenceInsufficient}, want: TraceStatusRefused},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := deriveTraceStatus(tt.trace); got != tt.want {
				t.Fatalf("deriveTraceStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestInMemoryTraceStoreQueryFilters(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	store := NewInMemoryTraceStore(0)
	store.OnExecutionTrace(context.Background(), ExecutionTrace{SessionID: "a", StartedAt: base, Success: true})
	store.OnExecutionTrace(context.Background(), ExecutionTrace{SessionID: "b", StartedAt: base.Add(time.Minute), Refused: true, RefusalReason: RefusalNoEvidence})
	store.OnExecutionTrace(context.Background(), ExecutionTrace{SessionID: "a", StartedAt: base.Add(2 * time.Minute), Success: false, Err: errors.New("x")})

	tests := []struct {
		name      string
		query     TraceQuery
		wantCount int
	}{
		{name: "按session", query: TraceQuery{SessionID: "a"}, wantCount: 2},
		{name: "按拒答状态", query: TraceQuery{Status: TraceStatusRefused}, wantCount: 1},
		{name: "按失败状态", query: TraceQuery{Status: TraceStatusFailure}, wantCount: 1},
		{name: "按时间窗", query: TraceQuery{Since: base.Add(30 * time.Second), Until: base.Add(90 * time.Second)}, wantCount: 1},
		{name: "限制条数", query: TraceQuery{Limit: 1}, wantCount: 1},
		{name: "无约束返回全部", query: TraceQuery{}, wantCount: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := store.Query(context.Background(), tt.query)
			if err != nil {
				t.Fatalf("Query() error = %v", err)
			}
			if len(got) != tt.wantCount {
				t.Fatalf("Query(%+v) count = %d, want %d", tt.query, len(got), tt.wantCount)
			}
		})
	}
}

func TestInMemoryTraceStoreQueryOrderingNewestFirst(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	store := NewInMemoryTraceStore(0)
	store.OnExecutionTrace(context.Background(), ExecutionTrace{SessionID: "a", Query: "first", StartedAt: base, Success: true})
	store.OnExecutionTrace(context.Background(), ExecutionTrace{SessionID: "a", Query: "second", StartedAt: base.Add(time.Minute), Success: true})

	got, err := store.Query(context.Background(), TraceQuery{SessionID: "a", Limit: 1})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(got) != 1 || got[0].QuerySummary != "second" {
		t.Fatalf("Query() newest = %+v, want QuerySummary=second", got)
	}
}

func TestInMemoryTraceStoreCapacityEviction(t *testing.T) {
	t.Parallel()

	store := NewInMemoryTraceStore(2)
	for _, id := range []string{"s1", "s2", "s3"} {
		store.OnExecutionTrace(context.Background(), ExecutionTrace{SessionID: id, Success: true})
	}
	if store.Len() != 2 {
		t.Fatalf("Len() = %d, want 2", store.Len())
	}
	if got, _ := store.Query(context.Background(), TraceQuery{SessionID: "s1"}); len(got) != 0 {
		t.Fatalf("oldest record s1 should be evicted, got %d", len(got))
	}
	if got, _ := store.Query(context.Background(), TraceQuery{SessionID: "s3"}); len(got) != 1 {
		t.Fatalf("newest record s3 should be kept, got %d", len(got))
	}
}

func TestInMemoryTraceStoreQueryValidationError(t *testing.T) {
	t.Parallel()

	store := NewInMemoryTraceStore(0)
	if _, err := store.Query(context.Background(), TraceQuery{Limit: -1}); err == nil {
		t.Fatal("Query() with negative limit should return error")
	}
	base := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	if _, err := store.Query(context.Background(), TraceQuery{Since: base.Add(time.Hour), Until: base}); err == nil {
		t.Fatal("Query() with until before since should return error")
	}
}

func TestInMemoryTraceStoreConcurrentWriteAndQuery(t *testing.T) {
	t.Parallel()

	store := NewInMemoryTraceStore(256)
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			store.OnExecutionTrace(context.Background(), ExecutionTrace{SessionID: "s", Success: true})
			_, _ = store.Query(context.Background(), TraceQuery{SessionID: "s"})
		})
	}
	wg.Wait()
	if store.Len() == 0 {
		t.Fatal("expected records after concurrent writes")
	}
}

// 确认 InMemoryTraceStore 满足 TraceStore 与 TraceRecorder 接口。
var (
	_ TraceStore    = (*InMemoryTraceStore)(nil)
	_ TraceRecorder = (*InMemoryTraceStore)(nil)
)

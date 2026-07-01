package ragagent

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestClassifyEvidenceRefusal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		metrics RetrievalMetrics
		want    RefusalReason
	}{
		{
			name:    "无过滤前候选归类为无证据",
			metrics: RetrievalMetrics{RawCandidateCount: 0},
			want:    RefusalNoEvidence,
		},
		{
			name:    "有过滤前候选归类为低相似度",
			metrics: RetrievalMetrics{RawCandidateCount: 3},
			want:    RefusalLowSimilarity,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := classifyEvidenceRefusal(tt.metrics); got != tt.want {
				t.Fatalf("classifyEvidenceRefusal(%+v) = %q, want %q", tt.metrics, got, tt.want)
			}
		})
	}
}

func TestExecutionTraceBuilderSetRefusal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		reason      RefusalReason
		wantRefused bool
		wantReason  RefusalReason
	}{
		{name: "无证据", reason: RefusalNoEvidence, wantRefused: true, wantReason: RefusalNoEvidence},
		{name: "低相似度", reason: RefusalLowSimilarity, wantRefused: true, wantReason: RefusalLowSimilarity},
		{name: "工具失败", reason: RefusalToolFailure, wantRefused: true, wantReason: RefusalToolFailure},
		{name: "RefusalNone不记录", reason: RefusalNone, wantRefused: false, wantReason: RefusalNone},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := newExecutionTraceBuilder("s1", "q", "q", RetrievalFilter{}, false)
			b.setRefusal(tt.reason)
			trace := b.finish(nil)
			if trace.Refused != tt.wantRefused {
				t.Fatalf("trace.Refused = %v, want %v", trace.Refused, tt.wantRefused)
			}
			if trace.RefusalReason != tt.wantReason {
				t.Fatalf("trace.RefusalReason = %q, want %q", trace.RefusalReason, tt.wantReason)
			}
			summary := SummarizeExecutionTrace(trace)
			if summary.Refused != tt.wantRefused || summary.RefusalReason != tt.wantReason {
				t.Fatalf("summary refusal = (%v,%q), want (%v,%q)", summary.Refused, summary.RefusalReason, tt.wantRefused, tt.wantReason)
			}
		})
	}
}

func TestAskRecordsNoEvidenceRefusalInTrace(t *testing.T) {
	t.Parallel()

	recorder := &traceRecorderStub{}
	a, err := New(Config{
		ChatModel: "gpt-4o-mini",
		Runtime: RuntimeComponents{
			ChatModel: stubRuntimeChatModel{},
			Embedder:  stubRuntimeEmbedder{},
		},
		Storage: StorageComponents{
			VectorStore: stubStorageVectorStore{},
		},
		TraceRecorder:    recorder,
		TopK:             1,
		ChunkSize:        64,
		ChunkOverlap:     0,
		MaxHistoryRounds: 8,
		RequestTimeout:   time.Second,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() {
		if cerr := a.Close(); cerr != nil {
			t.Fatalf("Close() error = %v", cerr)
		}
	})

	_, err = a.GetSession("refusal").Ask(context.Background(), "无关问题")
	if !errors.Is(err, ErrEvidenceInsufficient) {
		t.Fatalf("Ask() error = %v, want errors.Is(..., %v)", err, ErrEvidenceInsufficient)
	}

	traces := recorder.snapshot()
	if len(traces) != 1 {
		t.Fatalf("recorded traces = %d, want 1", len(traces))
	}
	got := traces[0]
	if !got.Refused {
		t.Fatalf("trace.Refused = false, want true")
	}
	if got.RefusalReason != RefusalNoEvidence {
		t.Fatalf("trace.RefusalReason = %q, want %q", got.RefusalReason, RefusalNoEvidence)
	}
}

func TestAskDoesNotRecordRefusalOnSuccess(t *testing.T) {
	t.Parallel()

	recorder := &traceRecorderStub{}
	a, err := New(Config{
		ChatModel: "gpt-4o-mini",
		Runtime: RuntimeComponents{
			ChatModel: stubRuntimeChatModel{},
			Embedder:  stubRuntimeEmbedder{},
		},
		Storage: StorageComponents{
			VectorStore: &capturingRootVectorStoreStub{
				hits: []SearchHit{
					{
						Chunk: ChunkRecord{
							ChunkID:    "doc:0",
							ParentID:   "doc",
							SourcePath: "/doc.md",
							Title:      "doc",
							Text:       "可用证据",
							StartRune:  0,
							EndRune:    12,
						},
						Score: 0.99,
					},
				},
			},
		},
		TraceRecorder:    recorder,
		TopK:             1,
		ChunkSize:        64,
		ChunkOverlap:     0,
		MaxHistoryRounds: 8,
		RequestTimeout:   time.Second,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() {
		if cerr := a.Close(); cerr != nil {
			t.Fatalf("Close() error = %v", cerr)
		}
	})

	if _, err := a.GetSession("ok").Ask(context.Background(), "问题"); err != nil {
		t.Fatalf("Ask() error = %v", err)
	}

	traces := recorder.snapshot()
	if len(traces) != 1 {
		t.Fatalf("recorded traces = %d, want 1", len(traces))
	}
	got := traces[0]
	if got.Refused {
		t.Fatalf("trace.Refused = true, want false")
	}
	if got.RefusalReason != RefusalNone {
		t.Fatalf("trace.RefusalReason = %q, want %q", got.RefusalReason, RefusalNone)
	}
}

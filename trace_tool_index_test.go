package ragagent

import (
	"errors"
	"testing"
)

func TestExecutionTraceBuilderToolCalls(t *testing.T) {
	t.Parallel()

	type step struct {
		start bool
		name  string
		err   error
	}
	failed := errors.New("failed")
	tests := []struct {
		name      string
		steps     []step
		wantNames []string
		wantErrs  []bool
		wantZero  []bool
	}{
		{
			name:      "sequential same-name calls stay separate",
			steps:     []step{{true, "echo", nil}, {false, "echo", nil}, {true, "echo", nil}, {false, "echo", failed}},
			wantNames: []string{"echo", "echo"},
			wantErrs:  []bool{false, true},
			wantZero:  []bool{false, false},
		},
		{
			name:      "nested same-name calls pair last-in-first-out",
			steps:     []step{{true, "echo", nil}, {true, "echo", nil}, {false, "echo", nil}, {false, "echo", failed}},
			wantNames: []string{"echo", "echo"},
			wantErrs:  []bool{true, false},
			wantZero:  []bool{false, false},
		},
		{
			name:      "end without start records a zero-duration failure",
			steps:     []step{{false, "missing", failed}, {true, "echo", nil}, {false, "echo", nil}},
			wantNames: []string{"missing", "echo"},
			wantErrs:  []bool{true, false},
			wantZero:  []bool{true, false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			builder := newExecutionTraceBuilder("s", "q", "q", RetrievalFilter{}, false)
			for _, s := range tt.steps {
				if s.start {
					builder.startTool(s.name)
				} else {
					builder.endTool(s.name, s.err)
				}
			}
			trace := builder.finish(nil)
			if len(trace.ToolCalls) != len(tt.wantNames) {
				t.Fatalf("tool calls = %+v, want %d entries", trace.ToolCalls, len(tt.wantNames))
			}
			for i, call := range trace.ToolCalls {
				if call.Name != tt.wantNames[i] {
					t.Fatalf("call %d name = %q, want %q", i, call.Name, tt.wantNames[i])
				}
				if (call.Err != nil) != tt.wantErrs[i] {
					t.Fatalf("call %d err = %v, want error %v", i, call.Err, tt.wantErrs[i])
				}
				if call.FinishedAt.IsZero() || call.FinishedAt.Before(call.StartedAt) {
					t.Fatalf("call %d has invalid timestamps: %+v", i, call)
				}
				if tt.wantZero[i] && (call.Duration != 0 || !call.StartedAt.Equal(call.FinishedAt)) {
					t.Fatalf("call %d must be zero-duration: %+v", i, call)
				}
			}
		})
	}

	t.Run("nil builder is a no-op", func(t *testing.T) {
		t.Parallel()
		var builder *executionTraceBuilder
		builder.startTool("x")
		builder.endTool("x", nil)
	})
}

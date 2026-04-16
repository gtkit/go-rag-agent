package memory

import (
	"slices"
	"testing"
)

func TestHistoryAppendTrimsOldTurns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		maxRounds   int
		appends     []Turn
		wantTurns   []Turn
		wantQueries []string
	}{
		{
			name:      "keeps only newest turns when capacity exceeded",
			maxRounds: 2,
			appends: []Turn{
				{User: "u1", Assistant: "a1"},
				{User: "u2", Assistant: "a2"},
				{User: "u3", Assistant: "a3"},
			},
			wantTurns: []Turn{
				{User: "u2", Assistant: "a2"},
				{User: "u3", Assistant: "a3"},
			},
			wantQueries: []string{"u2", "u3"},
		},
		{
			name:      "supports zero capacity",
			maxRounds: 0,
			appends: []Turn{
				{User: "u1", Assistant: "a1"},
			},
			wantTurns:   []Turn{},
			wantQueries: []string{},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := NewHistory(tc.maxRounds)
			for _, turn := range tc.appends {
				h.Append(turn.User, turn.Assistant)
			}

			gotTurns := h.Turns()
			if !slices.Equal(gotTurns, tc.wantTurns) {
				t.Fatalf("Turns() = %v, want %v", gotTurns, tc.wantTurns)
			}

			gotQueries := h.LastUserQueries()
			if !slices.Equal(gotQueries, tc.wantQueries) {
				t.Fatalf("LastUserQueries() = %v, want %v", gotQueries, tc.wantQueries)
			}
		})
	}
}

func TestHistoryTurnsReturnsCopyAndClear(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		initialTurns   []Turn
		mutateCopyTo   Turn
		wantAfterCopy  []Turn
		wantAfterClear []Turn
	}{
		{
			name: "mutating returned slice does not affect internal history and clear resets",
			initialTurns: []Turn{
				{User: "u1", Assistant: "a1"},
				{User: "u2", Assistant: "a2"},
			},
			mutateCopyTo: Turn{User: "mutated", Assistant: "mutated"},
			wantAfterCopy: []Turn{
				{User: "u1", Assistant: "a1"},
				{User: "u2", Assistant: "a2"},
			},
			wantAfterClear: []Turn{},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := NewHistory(len(tc.initialTurns))
			for _, turn := range tc.initialTurns {
				h.Append(turn.User, turn.Assistant)
			}

			copied := h.Turns()
			if len(copied) > 0 {
				copied[0] = tc.mutateCopyTo
			}

			if got := h.Turns(); !slices.Equal(got, tc.wantAfterCopy) {
				t.Fatalf("Turns() after mutating copy = %v, want %v", got, tc.wantAfterCopy)
			}

			h.Clear()
			if got := h.Turns(); !slices.Equal(got, tc.wantAfterClear) {
				t.Fatalf("Turns() after Clear() = %v, want %v", got, tc.wantAfterClear)
			}
		})
	}
}

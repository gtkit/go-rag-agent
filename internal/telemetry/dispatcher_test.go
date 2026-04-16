package telemetry

import (
	"context"
	"errors"
	"testing"
)

type callbackRecorder struct {
	retrieveStart int
	retrieveEnd   int
	toolStart     int
	toolEnd       int
	modelStart    int
	modelEnd      int
}

func (r *callbackRecorder) OnRetrieveStart(context.Context, string) { r.retrieveStart++ }

func (r *callbackRecorder) OnRetrieveEnd(context.Context, int, error) { r.retrieveEnd++ }

func (r *callbackRecorder) OnToolStart(context.Context, string) { r.toolStart++ }

func (r *callbackRecorder) OnToolEnd(context.Context, string, error) { r.toolEnd++ }

func (r *callbackRecorder) OnModelStart(context.Context, string) { r.modelStart++ }

func (r *callbackRecorder) OnModelEnd(context.Context, string, error) { r.modelEnd++ }

func TestDispatcherFanOut(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{
			name: "calls each hook on all callbacks",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cb1 := &callbackRecorder{}
			cb2 := &callbackRecorder{}
			dispatcher := NewDispatcher([]Callback{cb1, cb2})

			ctx := context.Background()
			sampleErr := errors.New("sample")

			dispatcher.OnRetrieveStart(ctx, "q")
			dispatcher.OnRetrieveEnd(ctx, 3, sampleErr)
			dispatcher.OnToolStart(ctx, "search")
			dispatcher.OnToolEnd(ctx, "search", sampleErr)
			dispatcher.OnModelStart(ctx, "gpt")
			dispatcher.OnModelEnd(ctx, "gpt", sampleErr)

			if cb1.retrieveStart != 1 || cb2.retrieveStart != 1 {
				t.Fatalf("OnRetrieveStart fan-out failed: cb1=%d cb2=%d", cb1.retrieveStart, cb2.retrieveStart)
			}
			if cb1.retrieveEnd != 1 || cb2.retrieveEnd != 1 {
				t.Fatalf("OnRetrieveEnd fan-out failed: cb1=%d cb2=%d", cb1.retrieveEnd, cb2.retrieveEnd)
			}
			if cb1.toolStart != 1 || cb2.toolStart != 1 {
				t.Fatalf("OnToolStart fan-out failed: cb1=%d cb2=%d", cb1.toolStart, cb2.toolStart)
			}
			if cb1.toolEnd != 1 || cb2.toolEnd != 1 {
				t.Fatalf("OnToolEnd fan-out failed: cb1=%d cb2=%d", cb1.toolEnd, cb2.toolEnd)
			}
			if cb1.modelStart != 1 || cb2.modelStart != 1 {
				t.Fatalf("OnModelStart fan-out failed: cb1=%d cb2=%d", cb1.modelStart, cb2.modelStart)
			}
			if cb1.modelEnd != 1 || cb2.modelEnd != 1 {
				t.Fatalf("OnModelEnd fan-out failed: cb1=%d cb2=%d", cb1.modelEnd, cb2.modelEnd)
			}
		})
	}
}

func TestDispatcherSkipsNilCallbacks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{
			name: "nil callbacks do not panic and valid callbacks still fire",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			valid := &callbackRecorder{}
			dispatcher := NewDispatcher([]Callback{nil, valid, nil})

			assertNotPanics(t, func() {
				dispatcher.OnRetrieveStart(context.Background(), "q")
			})

			if valid.retrieveStart != 1 {
				t.Fatalf("valid callback retrieveStart = %d, want 1", valid.retrieveStart)
			}
		})
	}
}

func TestNewDispatcherCopiesInputSlice(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{
			name: "mutating input callback slice does not affect dispatcher",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			valid := &callbackRecorder{}
			input := []Callback{valid}
			dispatcher := NewDispatcher(input)
			input[0] = nil

			dispatcher.OnToolStart(context.Background(), "search")
			if valid.toolStart != 1 {
				t.Fatalf("valid callback toolStart = %d, want 1", valid.toolStart)
			}
		})
	}
}

func assertNotPanics(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("unexpected panic: %v", r)
		}
	}()
	fn()
}

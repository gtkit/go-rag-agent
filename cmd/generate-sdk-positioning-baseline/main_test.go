package main

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		generate   func(context.Context, string) error
		wantOutDir string
		wantOut    []string
		wantErr    bool
	}{
		{
			name: "writes generated paths for explicit output directory",
			args: []string{"-out", "custom-baseline"},
			generate: func(_ context.Context, _ string) error {
				return nil
			},
			wantOutDir: "custom-baseline",
			wantOut: []string{
				filepath.Join("custom-baseline", "eval-report.json"),
				filepath.Join("custom-baseline", "trace-summary.json"),
			},
		},
		{
			name: "returns generator error",
			args: []string{"-out", "custom-baseline"},
			generate: func(_ context.Context, _ string) error {
				return errors.New("disk full")
			},
			wantOutDir: "custom-baseline",
			wantErr:    true,
		},
		{
			name: "rejects invalid flag",
			args: []string{"-unknown"},
			generate: func(_ context.Context, _ string) error {
				t.Fatal("generator should not be called")
				return nil
			},
			wantErr: true,
		},
		{
			name: "returns stdout write error",
			args: []string{"-out", "custom-baseline"},
			generate: func(_ context.Context, _ string) error {
				return nil
			},
			wantOutDir: "custom-baseline",
			wantErr:    true,
		},
		{
			name: "returns second stdout write error",
			args: []string{"-out", "custom-baseline"},
			generate: func(_ context.Context, _ string) error {
				return nil
			},
			wantOutDir: "custom-baseline",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldGenerate := generateSDKPositioningBaseline
			t.Cleanup(func() {
				generateSDKPositioningBaseline = oldGenerate
			})

			var gotOutDir string
			generateSDKPositioningBaseline = func(ctx context.Context, outDir string) error {
				if ctx == nil {
					t.Fatal("context is nil")
				}
				gotOutDir = outDir
				return tt.generate(ctx, outDir)
			}

			var stdout bytes.Buffer
			var stderr bytes.Buffer
			var output writer = &stdout
			if tt.name == "returns stdout write error" {
				output = errWriter{}
			}
			if tt.name == "returns second stdout write error" {
				output = &failOnWriteN{n: 2}
			}
			err := run(context.Background(), tt.args, output, &stderr)
			if (err != nil) != tt.wantErr {
				t.Fatalf("run() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantOutDir != "" && gotOutDir != tt.wantOutDir {
				t.Fatalf("outDir = %q, want %q", gotOutDir, tt.wantOutDir)
			}
			for _, want := range tt.wantOut {
				if !strings.Contains(stdout.String(), want) {
					t.Fatalf("stdout = %q, want containing %q", stdout.String(), want)
				}
			}
		})
	}
}

type writer interface {
	Write([]byte) (int, error)
}

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

type failOnWriteN struct {
	n     int
	count int
}

func (w *failOnWriteN) Write(data []byte) (int, error) {
	w.count++
	if w.count == w.n {
		return 0, errors.New("write failed")
	}
	return len(data), nil
}

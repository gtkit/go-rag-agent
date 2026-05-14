package ragagent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type structuredToolFunc struct {
	name        string
	description string
	schema      ToolSchema
	run         func(context.Context, map[string]any) (ToolResult, error)
}

func (f structuredToolFunc) Name() string { return f.name }

func (f structuredToolFunc) Description() string { return f.description }

func (f structuredToolFunc) Schema() ToolSchema { return f.schema }

func (f structuredToolFunc) RunStructured(ctx context.Context, args map[string]any) (ToolResult, error) {
	return f.run(ctx, args)
}

func TestValidateToolArguments(t *testing.T) {
	t.Parallel()

	schema := ToolSchema{
		Properties: map[string]ToolParameterSchema{
			"query": {Type: ToolParameterString, MinLength: 2, MaxLength: 8},
			"limit": {Type: ToolParameterNumber, Minimum: ptrFloat64(1), Maximum: ptrFloat64(10)},
			"mode":  {Type: ToolParameterString, Enum: []string{"fast", "deep"}},
			"tags":  {Type: ToolParameterArray},
		},
		Required: []string{"query", "limit"},
	}

	tests := []struct {
		name    string
		args    map[string]any
		wantErr string
	}{
		{
			name: "accepts valid arguments",
			args: map[string]any{
				"query": "golang",
				"limit": float64(3),
				"mode":  "fast",
				"tags":  []any{"rag"},
			},
		},
		{
			name:    "rejects missing required argument",
			args:    map[string]any{"limit": float64(3)},
			wantErr: "query",
		},
		{
			name:    "rejects wrong type",
			args:    map[string]any{"query": "go", "limit": "3"},
			wantErr: "limit",
		},
		{
			name:    "rejects enum mismatch",
			args:    map[string]any{"query": "go", "limit": float64(3), "mode": "slow"},
			wantErr: "mode",
		},
		{
			name:    "rejects string length boundary",
			args:    map[string]any{"query": "g", "limit": float64(3)},
			wantErr: "query",
		},
		{
			name:    "rejects numeric boundary",
			args:    map[string]any{"query": "go", "limit": float64(11)},
			wantErr: "limit",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateToolArguments(schema, tt.args)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateToolArguments() error = %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("ValidateToolArguments() error = nil, want non-nil")
			}
			if !errors.Is(err, ErrToolArgumentInvalid) {
				t.Fatalf("ValidateToolArguments() error = %v, want ErrToolArgumentInvalid", err)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ValidateToolArguments() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestStructuredToolAdapterRegistersAndRuns(t *testing.T) {
	t.Parallel()

	tool := structuredToolFunc{
		name:        "search_docs",
		description: "search docs",
		schema: ToolSchema{
			Properties: map[string]ToolParameterSchema{
				"query": {Type: ToolParameterString, MinLength: 1},
			},
			Required: []string{"query"},
		},
		run: func(_ context.Context, args map[string]any) (ToolResult, error) {
			return ToolResult{
				Text:     "found " + args["query"].(string),
				JSON:     map[string]any{"ok": true},
				Metadata: map[string]string{"source": "test"},
			}, nil
		},
	}

	adapter := NewStructuredToolAdapter(tool)
	registry := NewToolRegistry(adapter)
	registered, ok := registry.Lookup("search_docs")
	if !ok {
		t.Fatal("Lookup(search_docs) ok = false, want true")
	}
	got, err := registered.Run(context.Background(), `{"query":"rag"}`)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(got, "found rag") {
		t.Fatalf("Run() = %q, want containing structured text", got)
	}
	if !strings.Contains(got, `"ok":true`) {
		t.Fatalf("Run() = %q, want containing structured JSON", got)
	}
}

func TestStructuredToolAdapterRejectsInvalidArgumentsBeforeRun(t *testing.T) {
	t.Parallel()

	called := false
	adapter := NewStructuredToolAdapter(structuredToolFunc{
		name:        "search_docs",
		description: "search docs",
		schema: ToolSchema{
			Properties: map[string]ToolParameterSchema{
				"query": {Type: ToolParameterString},
			},
			Required: []string{"query"},
		},
		run: func(context.Context, map[string]any) (ToolResult, error) {
			called = true
			return ToolResult{}, nil
		},
	})

	_, err := adapter.Run(context.Background(), `{}`)
	if err == nil {
		t.Fatal("Run() error = nil, want non-nil")
	}
	if called {
		t.Fatal("structured tool was called despite invalid arguments")
	}
	if !errors.Is(err, ErrToolArgumentInvalid) {
		t.Fatalf("Run() error = %v, want ErrToolArgumentInvalid", err)
	}
}

func TestLegacyToolRegistryCompatibility(t *testing.T) {
	t.Parallel()

	registry := NewToolRegistry(registryTestTool{name: "legacy", description: "legacy", result: "ok"})
	tool, ok := registry.Lookup("legacy")
	if !ok {
		t.Fatal("Lookup(legacy) ok = false, want true")
	}
	got, err := tool.Run(context.Background(), "plain text")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got != "ok" {
		t.Fatalf("Run() = %q, want ok", got)
	}
}

func ptrFloat64(v float64) *float64 {
	return &v
}

func TestToolSchemaMarshalJSON(t *testing.T) {
	t.Parallel()

	schema := ToolSchema{
		Properties: map[string]ToolParameterSchema{
			"query": {Type: ToolParameterString, Description: "query", MinLength: 1},
		},
		Required: []string{"query"},
	}
	data, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	got := string(data)
	for _, want := range []string{`"type":"object"`, `"query"`, `"required":["query"]`} {
		if !strings.Contains(got, want) {
			t.Fatalf("schema JSON = %s, want containing %s", got, want)
		}
	}
}

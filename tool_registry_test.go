package ragagent

import (
	"context"
	"errors"
	"slices"
	"testing"
)

type registryTestTool struct {
	name        string
	description string
	result      string
	err         error
}

func (t registryTestTool) Name() string {
	return t.name
}

func (t registryTestTool) Description() string {
	return t.description
}

func (t registryTestTool) Run(context.Context, string) (string, error) {
	return t.result, t.err
}

func TestToolRegistryRegisterAndLookup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		tools     []Tool
		wantNames []string
		wantFound string
	}{
		{
			name: "keeps registration order and supports lookup",
			tools: []Tool{
				registryTestTool{name: "search_internal", description: "internal", result: "internal"},
				registryTestTool{name: "search_web", description: "web", result: "web"},
			},
			wantNames: []string{"search_internal", "search_web"},
			wantFound: "search_web",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			registry := NewToolRegistry()
			for _, tool := range tc.tools {
				if err := registry.Register(tool); err != nil {
					t.Fatalf("Register(%q) error = %v", tool.Name(), err)
				}
			}

			gotTools := registry.Tools()
			gotNames := make([]string, 0, len(gotTools))
			for _, tool := range gotTools {
				gotNames = append(gotNames, tool.Name())
			}
			if !slices.Equal(gotNames, tc.wantNames) {
				t.Fatalf("Tools() names = %v, want %v", gotNames, tc.wantNames)
			}

			got, ok := registry.Lookup(tc.wantFound)
			if !ok {
				t.Fatalf("Lookup(%q) ok = false, want true", tc.wantFound)
			}
			if got.Name() != tc.wantFound {
				t.Fatalf("Lookup(%q).Name() = %q, want %q", tc.wantFound, got.Name(), tc.wantFound)
			}
		})
	}
}

func TestToolRegistryRejectsDuplicates(t *testing.T) {
	t.Parallel()

	registry := NewToolRegistry()
	tool := registryTestTool{name: "search_web", description: "web"}

	if err := registry.Register(tool); err != nil {
		t.Fatalf("first Register() error = %v", err)
	}
	err := registry.Register(tool)
	if err == nil {
		t.Fatal("second Register() error = nil, want non-nil")
	}
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("second Register() error = %v, want errors.Is(..., %v)", err, ErrInvalidConfig)
	}
}

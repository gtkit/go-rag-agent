package ragagent

import (
	"context"
	"fmt"
)

// Tool 定义根包公开的工具契约。
type Tool interface {
	Name() string
	Description() string
	Run(ctx context.Context, input string) (string, error)
}

// ToolRegistry 保存按名称注册且保持顺序的工具集合。
type ToolRegistry struct {
	tools map[string]Tool
	order []string
}

// NewToolRegistry 创建一个空注册表，并按顺序注册输入工具。
func NewToolRegistry(tools ...Tool) *ToolRegistry {
	registry := &ToolRegistry{
		tools: make(map[string]Tool, len(tools)),
	}
	for _, tool := range tools {
		if tool == nil {
			continue
		}
		_ = registry.Register(tool)
	}
	return registry
}

// Register 注册一个工具；工具名称重复时返回错误。
func (r *ToolRegistry) Register(tool Tool) error {
	if tool == nil {
		return fmt.Errorf("tool is nil: %w", ErrInvalidConfig)
	}
	name := tool.Name()
	if name == "" {
		return fmt.Errorf("tool name is required: %w", ErrInvalidConfig)
	}
	if r.tools == nil {
		r.tools = make(map[string]Tool)
	}
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("tool %q is already registered: %w", name, ErrInvalidConfig)
	}
	r.tools[name] = tool
	r.order = append(r.order, name)
	return nil
}

func (r *ToolRegistry) registerOrReplace(tool Tool) error {
	if tool == nil {
		return fmt.Errorf("tool is nil: %w", ErrInvalidConfig)
	}
	name := tool.Name()
	if name == "" {
		return fmt.Errorf("tool name is required: %w", ErrInvalidConfig)
	}
	if r.tools == nil {
		r.tools = make(map[string]Tool)
	}
	if _, exists := r.tools[name]; !exists {
		r.order = append(r.order, name)
	}
	r.tools[name] = tool
	return nil
}

// Lookup 返回指定名称的工具。
func (r *ToolRegistry) Lookup(name string) (Tool, bool) {
	if r == nil || r.tools == nil {
		return nil, false
	}
	tool, ok := r.tools[name]
	return tool, ok
}

// Tools 返回保持注册顺序的工具切片副本。
func (r *ToolRegistry) Tools() []Tool {
	if r == nil || len(r.order) == 0 {
		return nil
	}
	out := make([]Tool, 0, len(r.order))
	for _, name := range r.order {
		out = append(out, r.tools[name])
	}
	return out
}

func (r *ToolRegistry) clone() *ToolRegistry {
	if r == nil {
		return NewToolRegistry()
	}
	cloned := &ToolRegistry{
		tools: make(map[string]Tool, len(r.tools)),
		order: append([]string(nil), r.order...),
	}
	for name, tool := range r.tools {
		cloned.tools[name] = tool
	}
	return cloned
}

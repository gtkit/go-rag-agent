package ragagent

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"
	"unicode/utf8"
)

// ToolParameterType identifies the supported structured tool argument type.
type ToolParameterType string

const (
	// ToolParameterString validates a JSON string.
	ToolParameterString ToolParameterType = "string"
	// ToolParameterNumber validates a JSON number.
	ToolParameterNumber ToolParameterType = "number"
	// ToolParameterBoolean validates a JSON boolean.
	ToolParameterBoolean ToolParameterType = "boolean"
	// ToolParameterObject validates a JSON object.
	ToolParameterObject ToolParameterType = "object"
	// ToolParameterArray validates a JSON array.
	ToolParameterArray ToolParameterType = "array"
)

// ToolParameterSchema describes one structured tool argument.
type ToolParameterSchema struct {
	Type        ToolParameterType `json:"type"`
	Description string            `json:"description,omitempty"`
	Enum        []string          `json:"enum,omitempty"`
	MinLength   int               `json:"minLength,omitempty"`
	MaxLength   int               `json:"maxLength,omitempty"`
	Minimum     *float64          `json:"minimum,omitempty"`
	Maximum     *float64          `json:"maximum,omitempty"`
}

// ToolSchema describes a structured tool input object.
type ToolSchema struct {
	Description string                         `json:"description,omitempty"`
	Properties  map[string]ToolParameterSchema `json:"properties,omitempty"`
	Required    []string                       `json:"required,omitempty"`
}

// MarshalJSON emits a JSON Schema-compatible object schema.
func (s ToolSchema) MarshalJSON() ([]byte, error) {
	type toolSchemaJSON struct {
		Type        string                         `json:"type"`
		Description string                         `json:"description,omitempty"`
		Properties  map[string]ToolParameterSchema `json:"properties,omitempty"`
		Required    []string                       `json:"required,omitempty"`
	}
	return json.Marshal(toolSchemaJSON{
		Type:        "object",
		Description: s.Description,
		Properties:  s.Properties,
		Required:    s.Required,
	})
}

// StructuredTool is a tool with machine-readable argument schema.
type StructuredTool interface {
	Name() string
	Description() string
	Schema() ToolSchema
	RunStructured(ctx context.Context, args map[string]any) (ToolResult, error)
}

// ToolResult is the normalized result returned by a StructuredTool.
type ToolResult struct {
	Text      string
	JSON      any
	Retryable bool
	Metadata  map[string]string
}

// ValidateToolArguments validates decoded JSON arguments against a ToolSchema.
func ValidateToolArguments(schema ToolSchema, args map[string]any) error {
	for _, field := range schema.Required {
		if _, ok := args[field]; !ok {
			return fmt.Errorf("%w: required argument %q is missing", ErrToolArgumentInvalid, field)
		}
	}
	for name, param := range schema.Properties {
		value, ok := args[name]
		if !ok || value == nil {
			continue
		}
		if err := validateToolArgument(name, param, value); err != nil {
			return err
		}
	}
	return nil
}

func validateToolArgument(name string, param ToolParameterSchema, value any) error {
	switch param.Type {
	case "", ToolParameterString:
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("%w: argument %q must be string", ErrToolArgumentInvalid, name)
		}
		if param.MinLength > 0 && utf8.RuneCountInString(text) < param.MinLength {
			return fmt.Errorf("%w: argument %q length must be >= %d", ErrToolArgumentInvalid, name, param.MinLength)
		}
		if param.MaxLength > 0 && utf8.RuneCountInString(text) > param.MaxLength {
			return fmt.Errorf("%w: argument %q length must be <= %d", ErrToolArgumentInvalid, name, param.MaxLength)
		}
		if len(param.Enum) > 0 && !slices.Contains(param.Enum, text) {
			return fmt.Errorf("%w: argument %q must be one of %s", ErrToolArgumentInvalid, name, strings.Join(param.Enum, ","))
		}
	case ToolParameterNumber:
		number, ok := jsonNumberValue(value)
		if !ok || math.IsNaN(number) || math.IsInf(number, 0) {
			return fmt.Errorf("%w: argument %q must be number", ErrToolArgumentInvalid, name)
		}
		if param.Minimum != nil && number < *param.Minimum {
			return fmt.Errorf("%w: argument %q must be >= %v", ErrToolArgumentInvalid, name, *param.Minimum)
		}
		if param.Maximum != nil && number > *param.Maximum {
			return fmt.Errorf("%w: argument %q must be <= %v", ErrToolArgumentInvalid, name, *param.Maximum)
		}
	case ToolParameterBoolean:
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%w: argument %q must be boolean", ErrToolArgumentInvalid, name)
		}
	case ToolParameterObject:
		if _, ok := value.(map[string]any); !ok {
			return fmt.Errorf("%w: argument %q must be object", ErrToolArgumentInvalid, name)
		}
	case ToolParameterArray:
		if _, ok := value.([]any); !ok {
			return fmt.Errorf("%w: argument %q must be array", ErrToolArgumentInvalid, name)
		}
	default:
		return fmt.Errorf("%w: argument %q has unsupported schema type %q", ErrToolArgumentInvalid, name, param.Type)
	}
	return nil
}

func jsonNumberValue(value any) (float64, bool) {
	switch n := value.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		v, err := n.Float64()
		return v, err == nil
	default:
		return 0, false
	}
}

// StructuredToolAdapter adapts a StructuredTool to the legacy Tool interface.
type StructuredToolAdapter struct {
	tool StructuredTool
}

// NewStructuredToolAdapter creates a legacy Tool wrapper around a StructuredTool.
func NewStructuredToolAdapter(tool StructuredTool) *StructuredToolAdapter {
	return &StructuredToolAdapter{tool: tool}
}

// Name returns the wrapped tool name.
func (a *StructuredToolAdapter) Name() string {
	if a == nil || a.tool == nil {
		return ""
	}
	return a.tool.Name()
}

// Description returns the wrapped tool description.
func (a *StructuredToolAdapter) Description() string {
	if a == nil || a.tool == nil {
		return ""
	}
	return a.tool.Description()
}

// Structured returns the wrapped structured tool.
func (a *StructuredToolAdapter) Structured() StructuredTool {
	if a == nil {
		return nil
	}
	return a.tool
}

// Run decodes JSON arguments, validates them, and runs the structured tool.
func (a *StructuredToolAdapter) Run(ctx context.Context, input string) (string, error) {
	if a == nil || a.tool == nil {
		return "", fmt.Errorf("structured tool is required: %w", ErrInvalidConfig)
	}
	var args map[string]any
	if strings.TrimSpace(input) == "" {
		args = map[string]any{}
	} else if err := json.Unmarshal([]byte(input), &args); err != nil {
		return "", fmt.Errorf("%w: decode arguments for tool %q: %v", ErrToolArgumentInvalid, a.tool.Name(), err)
	}
	if err := ValidateToolArguments(a.tool.Schema(), args); err != nil {
		return "", err
	}
	result, err := a.tool.RunStructured(ctx, maps.Clone(args))
	if err != nil {
		return "", fmt.Errorf("run structured tool %q: %w", a.tool.Name(), err)
	}
	return formatToolResult(result), nil
}

func formatToolResult(result ToolResult) string {
	parts := make([]string, 0, 2)
	if strings.TrimSpace(result.Text) != "" {
		parts = append(parts, strings.TrimSpace(result.Text))
	}
	if result.JSON != nil {
		data, err := json.Marshal(result.JSON)
		if err == nil && len(data) > 0 {
			parts = append(parts, string(data))
		}
	}
	return strings.Join(parts, "\n")
}

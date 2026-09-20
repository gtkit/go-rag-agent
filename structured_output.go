package ragagent

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/gtkit/go-rag-agent/internal/llm"
)

// StructuredAnswer 表示一次结构化问答结果。
type StructuredAnswer struct {
	Answer  Answer
	RawJSON string
}

func validateStructuredTarget(target any) error {
	if target == nil {
		return fmt.Errorf("structured target is nil: %w", ErrStructuredOutputTarget)
	}
	value := reflect.ValueOf(target)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		return fmt.Errorf("structured target must be a non-nil pointer: %w", ErrStructuredOutputTarget)
	}
	return nil
}

func buildStructuredInstruction(target any) (string, error) {
	if err := validateStructuredTarget(target); err != nil {
		return "", err
	}
	targetType := reflect.TypeOf(target).Elem()
	return "Return only valid JSON. Do not wrap the response in markdown fences or add commentary.\nExpected JSON shape:\n" + describeStructuredType(targetType), nil
}

func describeStructuredType(t reflect.Type) string {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Struct:
		fields := make([]string, 0, t.NumField())
		for field := range t.Fields() {
			if !field.IsExported() {
				continue
			}
			name := field.Name
			if tag, ok := field.Tag.Lookup("json"); ok {
				tagName, _, _ := strings.Cut(tag, ",")
				switch tagName {
				case "-":
					continue
				case "":
				default:
					name = tagName
				}
			}
			fields = append(fields, fmt.Sprintf("  %q: %s", name, describeStructuredType(field.Type)))
		}
		return "{\n" + strings.Join(fields, ",\n") + "\n}"
	case reflect.Slice, reflect.Array:
		return "[" + describeStructuredType(t.Elem()) + "]"
	case reflect.Map:
		if t.Key().Kind() == reflect.String {
			return "{string: " + describeStructuredType(t.Elem()) + "}"
		}
		return "object"
	case reflect.String:
		return "string"
	case reflect.Bool:
		return "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "integer"
	case reflect.Float32, reflect.Float64:
		return "number"
	default:
		return "value"
	}
}

func extractStructuredJSON(input string) (string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", fmt.Errorf("structured output is empty: %w", ErrStructuredOutputInvalid)
	}
	if json.Valid([]byte(trimmed)) {
		return trimmed, nil
	}
	if fenced, ok := extractFencedJSON(trimmed); ok && json.Valid([]byte(fenced)) {
		return fenced, nil
	}
	if balanced, ok := extractBalancedJSON(trimmed); ok && json.Valid([]byte(balanced)) {
		return balanced, nil
	}
	return "", fmt.Errorf("structured output does not contain valid JSON: %w", ErrStructuredOutputInvalid)
}

func extractFencedJSON(input string) (string, bool) {
	if !strings.HasPrefix(input, "```") {
		return "", false
	}
	firstNewline := strings.IndexByte(input, '\n')
	if firstNewline < 0 {
		return "", false
	}
	lastFence := strings.LastIndex(input, "```")
	if lastFence <= firstNewline {
		return "", false
	}
	content := strings.TrimSpace(input[firstNewline:lastFence])
	return content, content != ""
}

func extractBalancedJSON(input string) (string, bool) {
	start := strings.IndexAny(input, "{[")
	if start < 0 {
		return "", false
	}
	open := input[start]
	close := byte('}')
	if open == '[' {
		close = ']'
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(input); i++ {
		ch := input[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return strings.TrimSpace(input[start : i+1]), true
			}
		}
	}
	return "", false
}

func unmarshalStructuredJSON(raw string, target any) error {
	if err := json.Unmarshal([]byte(raw), target); err != nil {
		return fmt.Errorf("unmarshal structured output: %w: %w", err, ErrStructuredOutputInvalid)
	}
	return nil
}

// structuredResponseFormat 按配置为 target 构造原生响应格式；prompt 模式返回 nil。
// 调用前 target 已通过 validateStructuredTarget。
func structuredResponseFormat(format StructuredOutputFormat, target any) *llm.ResponseFormat {
	switch format {
	case StructuredOutputJSONSchema:
		targetType := reflect.TypeOf(target).Elem()
		name := targetType.Name()
		if name == "" {
			name = "response"
		}
		return &llm.ResponseFormat{
			Type:   llm.ResponseFormatJSONSchema,
			Name:   name,
			Schema: jsonSchemaForType(targetType, map[reflect.Type]bool{}),
		}
	case StructuredOutputPrompt:
		return nil
	default:
		return &llm.ResponseFormat{Type: llm.ResponseFormatJSONObject}
	}
}

var timeType = reflect.TypeFor[time.Time]()

// jsonSchemaForType 把 Go 类型反射为 JSON Schema。visiting 记录当前递归栈上的结构体，
// 自引用类型在第二次遇到时退化为无约束对象，避免无限递归。
func jsonSchemaForType(t reflect.Type, visiting map[reflect.Type]bool) map[string]any {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == timeType {
		return map[string]any{"type": "string", "format": "date-time"}
	}
	switch t.Kind() {
	case reflect.Struct:
		if visiting[t] {
			return map[string]any{"type": "object"}
		}
		visiting[t] = true
		defer delete(visiting, t)
		properties := map[string]any{}
		required := make([]string, 0, t.NumField())
		for field := range t.Fields() {
			if !field.IsExported() {
				continue
			}
			name := field.Name
			optional := field.Type.Kind() == reflect.Pointer
			if tag, ok := field.Tag.Lookup("json"); ok {
				parts := strings.Split(tag, ",")
				if parts[0] == "-" {
					continue
				}
				if parts[0] != "" {
					name = parts[0]
				}
				for _, opt := range parts[1:] {
					if opt == "omitempty" {
						optional = true
					}
				}
			}
			properties[name] = jsonSchemaForType(field.Type, visiting)
			if !optional {
				required = append(required, name)
			}
		}
		schema := map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
		if len(required) > 0 {
			schema["required"] = required
		}
		return schema
	case reflect.Slice, reflect.Array:
		if t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.Uint8 {
			return map[string]any{"type": "string"}
		}
		return map[string]any{"type": "array", "items": jsonSchemaForType(t.Elem(), visiting)}
	case reflect.Map:
		if t.Key().Kind() == reflect.String {
			return map[string]any{"type": "object", "additionalProperties": jsonSchemaForType(t.Elem(), visiting)}
		}
		return map[string]any{"type": "object"}
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}
	default:
		return map[string]any{}
	}
}

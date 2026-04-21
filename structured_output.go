package ragagent

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
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
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			if !field.IsExported() {
				continue
			}
			name := field.Name
			if tag, ok := field.Tag.Lookup("json"); ok {
				tagName := strings.Split(tag, ",")[0]
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

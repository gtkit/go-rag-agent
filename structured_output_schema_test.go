package ragagent

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"
)

type schemaNode struct {
	Name     string             `json:"name"`
	Optional *int               `json:"optional"`
	Tags     []string           `json:"tags,omitempty"`
	Scores   map[string]float64 `json:"scores"`
	When     time.Time          `json:"when"`
	Raw      []byte             `json:"raw"`
	Enabled  bool               `json:"enabled"`
	Count    uint8              `json:"count"`
	Any      any                `json:"any"`
	Ignored  string             `json:"-"`
	// 未导出字段必须被 schema 生成器跳过。
	_        string
	Children []*schemaNode  `json:"children"`
	Lookup   map[int]string `json:"lookup"`
}

func TestJSONSchemaForType(t *testing.T) {
	t.Parallel()

	schema := jsonSchemaForType(reflect.TypeFor[schemaNode](), map[reflect.Type]bool{})
	data, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}
	if decoded["type"] != "object" || decoded["additionalProperties"] != false {
		t.Fatalf("root schema = %v", decoded)
	}
	properties := decoded["properties"].(map[string]any)
	if _, ok := properties["Ignored"]; ok {
		t.Fatal("json:\"-\" field must be skipped")
	}
	if _, ok := properties["_"]; ok {
		t.Fatal("unexported field must be skipped")
	}

	tests := []struct {
		field string
		want  map[string]any
	}{
		{field: "name", want: map[string]any{"type": "string"}},
		{field: "optional", want: map[string]any{"type": "integer"}},
		{field: "tags", want: map[string]any{"type": "array", "items": map[string]any{"type": "string"}}},
		{field: "scores", want: map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "number"}}},
		{field: "when", want: map[string]any{"type": "string", "format": "date-time"}},
		{field: "raw", want: map[string]any{"type": "string"}},
		{field: "enabled", want: map[string]any{"type": "boolean"}},
		{field: "count", want: map[string]any{"type": "integer"}},
		{field: "any", want: map[string]any{}},
		{field: "lookup", want: map[string]any{"type": "object"}},
	}
	for _, tt := range tests {
		got, err := json.Marshal(properties[tt.field])
		if err != nil {
			t.Fatalf("marshal %s: %v", tt.field, err)
		}
		want, _ := json.Marshal(tt.want)
		if string(got) != string(want) {
			t.Fatalf("property %s = %s, want %s", tt.field, got, want)
		}
	}

	required := decoded["required"].([]any)
	requiredSet := map[string]bool{}
	for _, name := range required {
		requiredSet[name.(string)] = true
	}
	for _, name := range []string{"name", "scores", "when", "raw", "enabled", "count", "any", "children", "lookup"} {
		if !requiredSet[name] {
			t.Fatalf("field %s must be required, got %v", name, required)
		}
	}
	for _, name := range []string{"optional", "tags"} {
		if requiredSet[name] {
			t.Fatalf("pointer or omitempty field %s must be optional", name)
		}
	}

	// 自引用类型：children 的 items 在递归栈上再次遇到 schemaNode 时退化为无约束对象。
	children := properties["children"].(map[string]any)
	items := children["items"].(map[string]any)
	if items["type"] != "object" || items["properties"] != nil {
		t.Fatalf("self-referential items = %v, want plain object", items)
	}
}

func TestStructuredResponseFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		format     StructuredOutputFormat
		target     any
		wantNil    bool
		wantType   ResponseFormatType
		wantName   string
		wantSchema bool
	}{
		{name: "empty defaults to json_object", format: "", target: &structuredSummary{}, wantType: ResponseFormatJSONObject},
		{name: "json_object", format: StructuredOutputJSONObject, target: &structuredSummary{}, wantType: ResponseFormatJSONObject},
		{name: "json_schema names the type", format: StructuredOutputJSONSchema, target: &structuredSummary{}, wantType: ResponseFormatJSONSchema, wantName: "structuredSummary", wantSchema: true},
		{name: "json_schema anonymous type falls back to response", format: StructuredOutputJSONSchema, target: &struct{ A int }{}, wantType: ResponseFormatJSONSchema, wantName: "response", wantSchema: true},
		{name: "prompt disables native format", format: StructuredOutputPrompt, target: &structuredSummary{}, wantNil: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := structuredResponseFormat(tt.format, tt.target)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("structuredResponseFormat() = %+v, want nil", got)
				}
				return
			}
			if got == nil || got.Type != tt.wantType || got.Name != tt.wantName {
				t.Fatalf("structuredResponseFormat() = %+v, want type %s name %q", got, tt.wantType, tt.wantName)
			}
			if tt.wantSchema != (got.Schema != nil) {
				t.Fatalf("schema presence = %v, want %v", got.Schema != nil, tt.wantSchema)
			}
		})
	}
}

func TestConfigValidateStructuredOutputFormat(t *testing.T) {
	t.Parallel()

	base := func() Config {
		return Config{
			Runtime: RuntimeComponents{ChatModel: &sequencedRootChatModel{}, Embedder: &fakeEmbedder{defaultVec: []float32{1}}},
		}
	}
	tests := []struct {
		name    string
		format  StructuredOutputFormat
		wantErr bool
	}{
		{name: "empty uses default", format: ""},
		{name: "json_object", format: StructuredOutputJSONObject},
		{name: "json_schema", format: StructuredOutputJSONSchema},
		{name: "prompt", format: StructuredOutputPrompt},
		{name: "unknown value rejected", format: "yaml", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := base()
			cfg.StructuredOutputFormat = tt.format
			err := cfg.Validate()
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidConfig) {
					t.Fatalf("Validate() error = %v, want ErrInvalidConfig", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if got := cfg.withDefaults().StructuredOutputFormat; tt.format == "" && got != StructuredOutputJSONObject {
				t.Fatalf("default format = %q, want json_object", got)
			}
		})
	}
}

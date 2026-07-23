package mcp_tools_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/sigpanic/goink/internal/mcp_tools"
)

// ── 单元测试：SchemaOf 内联切片元素 ──────────────────────────

type nestedItem struct {
	Count int    `json:"count" jsonschema:"required,description=数量"`
	Label string `json:"label" jsonschema:"description=标签"`
}

type outerItem struct {
	Name    string       `json:"name" jsonschema:"required,description=名称"`
	Items   []nestedItem `json:"items" jsonschema:"description=嵌套子项"`
	Visible bool         `json:"visible" jsonschema:"description=是否可见"`
}

func TestSchemaOfNoRefsOrDefs(t *testing.T) {
	tests := []struct {
		name string
		v    any
	}{
		{"flat_struct", &struct {
			Name string `json:"name" jsonschema:"required,description=名称"`
		}{}},
		{"slice_of_structs", &struct {
			Entries []struct {
				ID    int    `json:"id" jsonschema:"required,description=ID"`
				Title string `json:"title" jsonschema:"required,description=标题"`
			} `json:"entries" jsonschema:"required,description=条目列表"`
		}{}},
		{"nested_slices", &struct {
			Outer []outerItem `json:"outer" jsonschema:"required,description=外层列表"`
		}{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := mcp_tools.SchemaOf(tt.v)
			s := string(raw)
			if strings.Contains(s, `"$ref"`) {
				t.Errorf("SchemaOf output contains $ref:\n%s", s)
			}
			if strings.Contains(s, `"$defs"`) {
				t.Errorf("SchemaOf output contains $defs:\n%s", s)
			}

			// 验证是合法 JSON
			var m map[string]any
			if err := json.Unmarshal(raw, &m); err != nil {
				t.Errorf("SchemaOf output is not valid JSON: %v", err)
			}
		})
	}
}

// nested_slices 的 items 深层应该也有 properties
func TestSchemaOfNestedSliceHasProperties(t *testing.T) {
	raw := mcp_tools.SchemaOf(&struct {
		Outer []outerItem `json:"outer" jsonschema:"required,description=外层列表"`
	}{})

	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}

	props, _ := m["properties"].(map[string]any)
	outer, _ := props["outer"].(map[string]any)
	items, _ := outer["items"].(map[string]any)
	itemProps, _ := items["properties"].(map[string]any)
	if _, ok := itemProps["name"]; !ok {
		t.Error("outer items missing 'name' property")
	}

	// 外层 item 的 items 字段是数组，需深入一层到 items
	nestedArray, _ := itemProps["items"].(map[string]any)
	nestedItems, _ := nestedArray["items"].(map[string]any)
	nestedItemProps, _ := nestedItems["properties"].(map[string]any)
	if _, ok := nestedItemProps["count"]; !ok {
		t.Error("nested items missing 'count' property")
	}
}

// ── 集成测试：所有注册工具 Schema 无 $ref/$defs ──────────────

func TestAllToolsSchemaNoRefsOrDefs(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	reg := mcp_tools.NewRegistry(logger)
	mcp_tools.RegisterAllTools(reg)

	for _, tool := range reg.List() {
		t.Run(tool.Name(), func(t *testing.T) {
			raw := tool.JSONSchema()
			s := string(raw)

			if s == "null" || s == "" {
				t.Skip("tool has no parameters schema")
			}

			if strings.Contains(s, `"$ref"`) {
				t.Errorf("tool %q schema contains $ref:\n%s", tool.Name(), s)
			}
			if strings.Contains(s, `"$defs"`) {
				t.Errorf("tool %q schema contains $defs:\n%s", tool.Name(), s)
			}

			var m map[string]any
			if err := json.Unmarshal(raw, &m); err != nil {
				t.Errorf("tool %q schema is not valid JSON: %v", tool.Name(), err)
			}
		})
	}
}

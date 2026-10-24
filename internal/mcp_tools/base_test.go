package mcp_tools

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"slices"
	"strings"
	"testing"
)

type systemErrorTestArgs struct{}

type systemErrorTestTool struct {
	err    error
	result *ToolResult
}

func (t systemErrorTestTool) Name() string           { return "system_error_test" }
func (t systemErrorTestTool) Description() string    { return "test only" }
func (t systemErrorTestTool) Category() ToolCategory { return CategoryWritingAssistant }
func (t systemErrorTestTool) JSONSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}
func (t systemErrorTestTool) ExposeToLLM() bool { return false }
func (t systemErrorTestTool) NewArgs() any      { return &systemErrorTestArgs{} }
func (t systemErrorTestTool) Execute(context.Context, any, ToolContext) (*ToolResult, error) {
	return t.result, t.err
}

func TestRegistryDefinitions(t *testing.T) {
	const schema = `{ "type": "object", "properties": {"id": {"type": "integer", "minimum": 9007199254740993}}, "required": ["id"], "additionalProperties": false }`
	reg := NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	for _, tool := range []catalogTestTool{
		{name: "z_tool", description: "最后一个工具", exposed: true},
		{name: "a_tool", description: "第一个工具", exposed: true},
		{name: "hidden_tool", description: "按需调用工具"},
	} {
		tool.schema = json.RawMessage(schema)
		reg.Register(tool)
	}
	for _, tt := range []struct {
		name    string
		allowed map[string]bool
		want    []string
	}{
		{name: "unrestricted", want: []string{"a_tool", "z_tool"}},
		{name: "empty allowlist", allowed: map[string]bool{}},
		{name: "filtered", allowed: map[string]bool{"a_tool": true, "z_tool": false, "hidden_tool": true, "missing": true}, want: []string{"a_tool"}},
		{name: "unknown tool", allowed: map[string]bool{"missing": true}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var names []string
			for _, definition := range reg.Definitions(tt.allowed) {
				names = append(names, definition.Name)
				tool, ok := reg.Get(definition.Name)
				if !ok || definition.Description != tool.Description() || string(definition.InputSchema) != schema {
					t.Fatalf("tool declaration lost its description or schema: %+v", definition)
				}
			}
			if !slices.Equal(names, tt.want) {
				t.Fatalf("visible tools = %v, want %v", names, tt.want)
			}
		})
	}
}

func TestRegistryDefinitionsOwnSchemaBytes(t *testing.T) {
	const original = `{"type":"object","properties":{}}`
	schema := json.RawMessage(original)
	reg := NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	reg.Register(catalogTestTool{name: "test_tool", schema: schema, exposed: true})
	first, second := reg.Definitions(nil), reg.Definitions(nil)
	if len(first) != 1 || len(second) != 1 {
		t.Fatal("expected one tool declaration per snapshot")
	}
	first[0].InputSchema[0] = '['
	if string(schema) != original || string(second[0].InputSchema) != original || string(reg.Definitions(nil)[0].InputSchema) != original {
		t.Fatal("changing a declaration modified the tool schema or another snapshot")
	}
}

func TestRegistryExecute_CompactsToolResultError(t *testing.T) {
	reg := NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	detail := "文件已被修改，请重新读取\n" + strings.Repeat("x", maxSystemErrorDetailRunes+1)
	reg.Register(systemErrorTestTool{result: &ToolResult{Success: false, Error: detail}})

	result := reg.Execute(context.Background(), "system_error_test", []byte(`{}`), ToolContext{}, nil)
	if result.Success {
		t.Fatal("expected failure")
	}
	if result.ErrKind != ErrKindBusiness {
		t.Errorf("ErrKind = %q, want business kind", result.ErrKind)
	}
	if strings.Contains(result.Error, "\n") {
		t.Errorf("Error contains newline: %q", result.Error)
	}
	if !strings.HasPrefix(result.Error, "文件已被修改，请重新读取 x") {
		t.Errorf("Error = %q, want compacted ToolResult detail", result.Error)
	}
	if !strings.HasSuffix(result.Error, "…") {
		t.Errorf("Error = %q, want truncation marker", result.Error)
	}
}

func TestRegistryExecute_SystemErrorIncludesCompactDetail(t *testing.T) {
	reg := NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	detail := "正文已回退，标题未更新: database is locked\n" + strings.Repeat("x", maxSystemErrorDetailRunes+1)
	reg.Register(systemErrorTestTool{err: errors.New(detail)})

	result := reg.Execute(context.Background(), "system_error_test", []byte(`{}`), ToolContext{}, nil)
	if result.Success {
		t.Fatal("expected system error")
	}
	if result.ErrKind != ErrKindSystem {
		t.Errorf("ErrKind = %q, want %q", result.ErrKind, ErrKindSystem)
	}
	if !strings.HasPrefix(result.Error, "工具执行失败：正文已回退，标题未更新: database is locked x") {
		t.Errorf("Error = %q, want compact detail with operation state", result.Error)
	}
	if strings.Contains(result.Error, "\n") {
		t.Errorf("Error contains newline: %q", result.Error)
	}
	if !strings.HasSuffix(result.Error, "…") {
		t.Errorf("Error = %q, want truncation marker", result.Error)
	}
	if got := len([]rune(strings.TrimPrefix(result.Error, "工具执行失败："))); got != maxSystemErrorDetailRunes+1 {
		t.Errorf("detail rune length = %d, want %d", got, maxSystemErrorDetailRunes+1)
	}
}

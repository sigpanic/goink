package mcp_tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/sigpanic/goink/internal/agentcfg"
	"github.com/stretchr/testify/require"
)

type catalogTestTool struct {
	runMCPTestTool
	name, description string
	schema            json.RawMessage
	exposed           bool
}

func (t catalogTestTool) Name() string        { return t.name }
func (t catalogTestTool) Description() string { return t.description }
func (t catalogTestTool) ExposeToLLM() bool   { return t.exposed }
func (t catalogTestTool) JSONSchema() json.RawMessage {
	if t.schema != nil {
		return t.schema
	}
	return t.runMCPTestTool.JSONSchema()
}

func readCatalogForTest(t *testing.T, r *Registry, args ReadArgs) *ToolResult {
	t.Helper()
	raw, err := json.Marshal(args)
	require.NoError(t, err)
	return r.Execute(context.Background(), "read", raw, ToolContext{}, agentcfg.Allowlist(agentcfg.MainAgent))
}

func TestReadCatalogIndex(t *testing.T) {
	r := NewRegistry(slog.Default())
	RegisterRWTools(r)
	RegisterRunMCPTools(r)
	result := readCatalogForTest(t, r, ReadArgs{Path: "mcp/index.md"})
	require.True(t, result.Success, result.Error)
	require.Contains(t, result.Data["content"], "暂无可用")

	r.Register(catalogTestTool{name: "z_tool", description: "最后一个工具\n完整的详细说明"})
	r.Register(catalogTestTool{name: "a_tool", description: "  第一个工具\t简述  \n不应出现在索引的详细说明"})
	r.Register(catalogTestTool{name: "direct_tool", description: "直接展示的工具", exposed: true})
	r.Register(catalogTestTool{name: "long_tool", description: strings.Repeat("中", 150)})
	result = readCatalogForTest(t, r, ReadArgs{Path: "mcp/index.md"})
	require.True(t, result.Success, result.Error)
	content := result.Data["content"].(string)
	require.Contains(t, content, "1|# 拓展工具")
	require.Contains(t, content, "`a_tool` — 第一个工具 简述；schema：`mcp/schemas/a_tool.json`")
	require.Contains(t, content, strings.Repeat("中", 120)+"…")
	require.Less(t, strings.Index(content, "`a_tool`"), strings.Index(content, "`z_tool`"))
	require.NotContains(t, content, "完整的详细说明")
	require.NotContains(t, content, "不应出现在索引")
	require.NotContains(t, content, "direct_tool")
	require.NotContains(t, content, "`run_mcp`")
	require.NotContains(t, content, "\"properties\"")

	page := readCatalogForTest(t, r, ReadArgs{Path: "mcp/index.md", StartLine: 4, EndLine: 4})
	require.True(t, page.Success, page.Error)
	require.Equal(t, "4|- `a_tool` — 第一个工具 简述；schema：`mcp/schemas/a_tool.json`", page.Data["content"])
	require.Equal(t, result.Data["total_lines"], page.Data["total_lines"])
	require.Equal(t, true, page.Data["truncated"])
	plain := false
	page = readCatalogForTest(t, r, ReadArgs{Path: "mcp/index.md", StartLine: 4, EndLine: 4, IncludeLines: &plain})
	require.True(t, page.Success, page.Error)
	require.True(t, strings.HasPrefix(page.Data["content"].(string), "- `a_tool`"))
}

func TestReadCatalogSchemaAndCallUseSameName(t *testing.T) {
	r := NewRegistry(slog.Default())
	RegisterAllTools(r)
	r.Register(catalogTestTool{
		name: "get_volumes", description: "查询卷\n完整说明保留在 schema 中。",
		runMCPTestTool: runMCPTestTool{execute: func(_ context.Context, args any, _ ToolContext) (*ToolResult, error) {
			return &ToolResult{Success: true, Data: map[string]any{"name": args.(*runMCPTestArgs).Name}}, nil
		}},
	})
	result := readCatalogForTest(t, r, ReadArgs{Path: "mcp/schemas/get_volumes.json"})
	require.True(t, result.Success, result.Error)
	var document struct {
		ToolInfo
		InputSchema json.RawMessage `json:"inputSchema"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Data["content"].(string)), &document))
	require.Equal(t, "get_volumes", document.Name)
	require.Equal(t, "查询卷\n完整说明保留在 schema 中。", document.Description)
	require.Equal(t, string(CategoryWritingAssistant), document.Category)
	require.JSONEq(t, string(SchemaOf(runMCPTestArgs{})), string(document.InputSchema))
	require.NotContains(t, result.Data, "truncated")
	raw, err := json.Marshal(RunMCPArgs{ToolName: document.Name, Args: map[string]json.RawMessage{"name": json.RawMessage(`"test"`)}})
	require.NoError(t, err)
	called := r.Execute(context.Background(), "run_mcp", raw, ToolContext{}, agentcfg.Allowlist(agentcfg.MainAgent))
	require.True(t, called.Success, called.Error)
	require.Equal(t, "test", called.Data["name"])

	r.Register(catalogTestTool{name: "get_volumes", exposed: true})
	result = readCatalogForTest(t, r, ReadArgs{Path: "mcp/schemas/get_volumes.json"})
	require.False(t, result.Success, "重新注册为直接工具后，不应读取旧 schema")
	index := readCatalogForTest(t, r, ReadArgs{Path: "mcp/index.md"})
	require.NotContains(t, index.Data["content"], "get_volumes")
	called = r.Execute(context.Background(), "run_mcp", raw, ToolContext{}, agentcfg.Allowlist(agentcfg.MainAgent))
	require.False(t, called.Success)
}

func TestReadCatalogIndexUsesReadLineLimit(t *testing.T) {
	r := NewRegistry(slog.Default())
	RegisterRWTools(r)
	for i := range 2010 {
		r.Register(catalogTestTool{name: fmt.Sprintf("tool_%04d", i), description: "测试工具"})
	}
	first := readCatalogForTest(t, r, ReadArgs{Path: "mcp/index.md"})
	require.True(t, first.Success, first.Error)
	require.Equal(t, 2000, first.Data["end_line"])
	require.Equal(t, 2013, first.Data["total_lines"])
	require.Equal(t, true, first.Data["truncated"])
	last := readCatalogForTest(t, r, ReadArgs{Path: "mcp/index.md", StartLine: 2001, EndLine: 2020})
	require.True(t, last.Success, last.Error)
	require.Equal(t, 2013, last.Data["end_line"])
	require.Contains(t, last.Data["content"], "`tool_2009`")
	require.NotContains(t, last.Data, "truncated")
}

func TestReadCatalogSchemaRendering(t *testing.T) {
	r := NewRegistry(slog.Default())
	RegisterRWTools(r)
	r.Register(catalogTestTool{name: "test_tool", schema: json.RawMessage(`{
  "type": "object",
  "properties": {"name": {"type": "string"}}
}`)})
	path := "mcp/schemas/test_tool.json"
	result := readCatalogForTest(t, r, ReadArgs{Path: path})
	require.True(t, result.Success, result.Error)
	require.True(t, json.Valid([]byte(result.Data["content"].(string))))
	require.NotContains(t, result.Data["content"], "\n")
	require.Contains(t, result.Data["content"], `"inputSchema":{"type":"object","properties":{"name":{"type":"string"}}}`)
	require.Equal(t, map[string]any{"path": path, "content": result.Data["content"]}, result.Data)

	withLines := true
	for _, args := range []ReadArgs{
		{Path: path, IncludeLines: &withLines},
		{Path: path, StartLine: 2, EndLine: 4},
	} {
		got := readCatalogForTest(t, r, args)
		require.True(t, got.Success, got.Error)
		require.Equal(t, result.Data, got.Data, "schema 始终完整返回，不进入行范围渲染")
	}
}

func TestReadCatalogOversizedSchema(t *testing.T) {
	r := NewRegistry(slog.Default())
	RegisterRWTools(r)
	tool := catalogTestTool{name: "huge_tool"}
	r.Register(tool)
	path := "mcp/schemas/huge_tool.json"
	content, _, err := r.readCatalog(path)
	require.NoError(t, err)
	tool.description = strings.Repeat("a", maxCatalogSchemaBytes-len(content))
	r.Register(tool)
	result := readCatalogForTest(t, r, ReadArgs{Path: path})
	require.True(t, result.Success, result.Error)
	require.Len(t, result.Data["content"], maxCatalogSchemaBytes)

	tool.description += "a"
	r.Register(tool)
	for _, args := range []ReadArgs{
		{Path: path},
		{Path: path, StartLine: 1, EndLine: 20},
	} {
		result := readCatalogForTest(t, r, args)
		require.False(t, result.Success)
		require.Contains(t, result.Error, "超过大小上限")
		require.Empty(t, result.Data, "超限时不应返回内容，指定行范围也不能绕过上限")
	}
}

func TestReadCatalogRejectsInvalidPathsAndEdit(t *testing.T) {
	r := NewRegistry(slog.Default())
	RegisterAllTools(r)
	r.Register(catalogTestTool{name: "get_volumes"})
	for _, path := range []string{
		"mcp/schemas/missing.json", "mcp/schemas/read.json", "mcp/schemas/run_mcp.json",
		"mcp/../goink.md", "mcp/schemas/../get_volumes.json", "mcp/schemas/../../goink.md",
		"mcp/schemas/%2e%2e%2fget_volumes.json", "mcp/schemas/get_volumes.json/extra",
		"mcp/schemas/get_volumes.json?x=1", "mcp/schemas/get_volumes.json#fragment",
		"mcp/schemas/get_volumes.JSON", "mcp/schemas/get_volumes", "mcp/schemas/.json",
		`mcp/schemas/..\get_volumes.json`, "mcp//index.md", "mcp/./index.md", "mcp/index.md/", "mcp/",
	} {
		t.Run(path, func(t *testing.T) {
			result := readCatalogForTest(t, r, ReadArgs{Path: path})
			require.False(t, result.Success)
			require.Equal(t, ErrKindBusiness, result.ErrKind)
			require.NotEmpty(t, result.Error)
		})
	}
	for _, path := range []string{"mcp/index.md", "mcp/schemas/get_volumes.json"} {
		content := "修改"
		raw, err := json.Marshal(EditArgs{Path: path, ChangeType: "full_replace", NewContent: &content, Reason: "测试"})
		require.NoError(t, err)
		result := r.Execute(context.Background(), "edit", raw, ToolContext{}, agentcfg.Allowlist(agentcfg.MainAgent))
		require.False(t, result.Success)
		require.Contains(t, result.Error, "无效文件路径")
	}
}

func TestReadCatalogDependencyErrors(t *testing.T) {
	r := NewRegistry(slog.Default())
	RegisterRWTools(r)
	r.Register(catalogTestTool{name: "broken_tool", schema: json.RawMessage(`{"type":`)})
	result := readCatalogForTest(t, r, ReadArgs{Path: "mcp/schemas/broken_tool.json"})
	require.False(t, result.Success)
	require.Equal(t, ErrKindSystem, result.ErrKind)
	require.Contains(t, result.Error, "编码工具 broken_tool 的 schema")
	read := &ReadTool{}
	result, err := read.Execute(context.Background(), &ReadArgs{Path: "mcp/index.md"}, ToolContext{})
	require.Nil(t, result)
	require.ErrorContains(t, err, "工具注册表未初始化")
}

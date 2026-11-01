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

func searchMCPForTest(t *testing.T, r *Registry, queries ...string) *ToolResult {
	t.Helper()
	raw, err := json.Marshal(SearchMCPArgs{Queries: queries})
	require.NoError(t, err)
	return r.Execute(context.Background(), "search_mcp", raw, ToolContext{}, agentcfg.Allowlist(agentcfg.MainAgent))
}

func catalogNames(tools []Tool) []string {
	names := make([]string, len(tools))
	for i, tool := range tools {
		names[i] = tool.Name()
	}
	return names
}

func TestSearchCatalogKeywordMatching(t *testing.T) {
	r := NewRegistry(slog.Default())
	r.Register(catalogTestTool{name: "get_volumes", description: "查询小说分卷信息。\n返回卷纲和排序。"})
	r.Register(catalogTestTool{name: "edit_volumes", description: "修改小说分卷名称。"})
	r.Register(catalogTestTool{name: "credit_tool", description: "信用信息。"})
	r.Register(catalogTestTool{name: "get_characters", description: "查看角色信息。"})
	for _, tt := range []struct {
		query string
		want  []string
	}{
		{"分卷查询", []string{"get_volumes", "edit_volumes"}},
		{"卷纲", []string{"get_volumes"}},
		{"  GET__VOLUMES  ", []string{"get_volumes", "edit_volumes", "get_characters"}},
		{"EDIT", []string{"edit_volumes"}},
		{"信用", []string{"credit_tool"}},
		{"[.*]", []string{}},
		{"卷", []string{}},
		{"天气预测", []string{}},
	} {
		t.Run(tt.query, func(t *testing.T) {
			require.Equal(t, tt.want, catalogNames(r.searchCatalog([]string{tt.query})))
		})
	}
}

func TestSearchCatalogRankingAndQueryDeduplication(t *testing.T) {
	r := NewRegistry(slog.Default())
	r.Register(catalogTestTool{name: "z_tool", description: "分卷查询"})
	r.Register(catalogTestTool{name: "a_tool", description: "查询小说分卷信息"})
	require.Equal(t, []string{"z_tool", "a_tool"}, catalogNames(r.searchCatalog([]string{"分卷查询"})))

	r = NewRegistry(slog.Default())
	r.Register(catalogTestTool{name: "z_tool", description: "(volume archive)."})
	r.Register(catalogTestTool{name: "a_tool", description: "archive volume"})
	require.Equal(t, []string{"z_tool", "a_tool"}, catalogNames(r.searchCatalog([]string{"volume archive"})))

	r = NewRegistry(slog.Default())
	r.Register(catalogTestTool{name: "z_volume", description: "其他能力"})
	r.Register(catalogTestTool{name: "a_tool", description: "volume"})
	require.Equal(t, []string{"z_volume", "a_tool"}, catalogNames(r.searchCatalog([]string{"volume"})))

	r = NewRegistry(slog.Default())
	r.Register(catalogTestTool{name: "a_tool", description: "alpha"})
	r.Register(catalogTestTool{name: "b_tool", description: "beta"})
	require.Equal(t, []string{"a_tool", "b_tool"}, catalogNames(r.searchCatalog([]string{"beta", "alpha", "beta", "beta"})))

	r = NewRegistry(slog.Default())
	for i := 7; i >= 0; i-- {
		r.Register(catalogTestTool{name: fmt.Sprintf("tool_%d", i), description: "查询分卷"})
	}
	require.Equal(t, []string{"tool_0", "tool_1", "tool_2", "tool_3", "tool_4"}, catalogNames(r.searchCatalog([]string{"分卷"})))
}

func TestSearchMCPDiscoverReadAndExecute(t *testing.T) {
	r := NewRegistry(slog.Default())
	RegisterRWTools(r)
	RegisterRunMCPTools(r)
	RegisterSearchMCPTools(r)
	r.Register(catalogTestTool{
		name: "get_volumes", description: "查询小说分卷信息。\n详细参数只在 schema 中说明。",
		runMCPTestTool: runMCPTestTool{execute: func(_ context.Context, args any, _ ToolContext) (*ToolResult, error) {
			return &ToolResult{Success: true, Data: map[string]any{"name": args.(*runMCPTestArgs).Name}}, nil
		}},
	})
	r.Register(catalogTestTool{name: "direct_volume_tool", description: "查询小说分卷信息。", exposed: true})
	result := searchMCPForTest(t, r, "分卷查询")
	require.True(t, result.Success, result.Error)
	content := result.Data["content"].(string)
	require.Contains(t, content, "`get_volumes` — 查询小说分卷信息。；schema：`mcp/schemas/get_volumes.json`")
	require.NotContains(t, content, "direct_volume_tool")
	require.NotContains(t, content, "详细参数")
	require.NotContains(t, content, "inputSchema")

	lines := strings.Split(content, "\n")
	entry := lines[len(lines)-1]
	fields := strings.Split(entry, "`")
	require.Len(t, fields, 5)
	name, path := fields[1], fields[3]
	schema := readCatalogForTest(t, r, ReadArgs{Path: path})
	require.True(t, schema.Success, schema.Error)
	var document struct {
		ToolInfo
		InputSchema json.RawMessage `json:"inputSchema"`
	}
	require.NoError(t, json.Unmarshal([]byte(schema.Data["content"].(string)), &document))
	require.Equal(t, name, document.Name)
	require.JSONEq(t, string(SchemaOf(runMCPTestArgs{})), string(document.InputSchema))
	raw, err := json.Marshal(RunMCPArgs{ToolName: name, Args: map[string]json.RawMessage{"name": json.RawMessage(`"test"`)}})
	require.NoError(t, err)
	called := r.Execute(context.Background(), "run_mcp", raw, ToolContext{}, agentcfg.Allowlist(agentcfg.MainAgent))
	require.True(t, called.Success, called.Error)
	require.Equal(t, "test", called.Data["name"])

	r.Register(catalogTestTool{name: "get_volumes", exposed: true})
	result = searchMCPForTest(t, r, "分卷查询")
	require.True(t, result.Success, result.Error)
	require.Contains(t, result.Data["content"], "未找到")
	require.Contains(t, result.Data["content"], "mcp/index.md")
}

func TestSearchMCPValidatesQueriesAndAgentAccess(t *testing.T) {
	r := NewRegistry(slog.Default())
	RegisterSearchMCPTools(r)
	for _, raw := range []string{
		`{}`, `{"queries":null}`, `{"queries":[]}`, `{"queries":"分卷"}`,
		`{"queries":[""]}`, `{"queries":[" "]}`, `{"queries":["分卷",""]}`,
		`{"queries":["1","2","3","4","5","6"]}`, `{"queries":["分卷"],"novel_id":1}`,
	} {
		result := r.Execute(context.Background(), "search_mcp", json.RawMessage(raw), ToolContext{}, agentcfg.Allowlist(agentcfg.MainAgent))
		require.False(t, result.Success, raw)
	}
	result := searchMCPForTest(t, r, "查询", "分卷", "volume", "建卷", "卷名")
	require.True(t, result.Success, result.Error)
	for _, role := range []agentcfg.AgentType{agentcfg.ReviewAgent, agentcfg.MemoryAgent} {
		result := r.Execute(context.Background(), "search_mcp", json.RawMessage(`{"queries":["分卷"]}`), ToolContext{}, agentcfg.Allowlist(role))
		require.False(t, result.Success)
		require.Contains(t, result.Error, "工具禁止使用")
		require.Empty(t, r.Definitions(agentcfg.Allowlist(role)))
	}
	tools := r.Definitions(agentcfg.Allowlist(agentcfg.MainAgent))
	require.Len(t, tools, 1)
	require.Equal(t, "search_mcp", tools[0].Name)
	var schema struct {
		Required   []string
		Properties map[string]struct {
			Type     string
			MinItems int
			MaxItems int
		}
	}
	require.NoError(t, json.Unmarshal(tools[0].InputSchema, &schema))
	require.Equal(t, []string{"queries"}, schema.Required)
	require.Equal(t, "array", schema.Properties["queries"].Type)
	require.Equal(t, 1, schema.Properties["queries"].MinItems)
	require.Equal(t, 5, schema.Properties["queries"].MaxItems)
}

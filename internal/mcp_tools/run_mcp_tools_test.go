package mcp_tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/sigpanic/goink/internal/agentcfg"
	"github.com/sigpanic/goink/internal/approval"
	"github.com/stretchr/testify/require"
)

type runMCPTestArgs struct {
	Name string `json:"name" validate:"required"`
	ID   int64  `json:"id" validate:"omitempty,min=1"`
	Refs []struct {
		ID int64 `json:"id"`
	} `json:"refs"`
}

type runMCPTestTool struct {
	newArgs func() any
	execute func(context.Context, any, ToolContext) (*ToolResult, error)
}

func (runMCPTestTool) Name() string                { return "low_frequency" }
func (runMCPTestTool) Description() string         { return "测试拓展工具" }
func (runMCPTestTool) Category() ToolCategory      { return CategoryWritingAssistant }
func (runMCPTestTool) JSONSchema() json.RawMessage { return SchemaOf(runMCPTestArgs{}) }
func (runMCPTestTool) ExposeToLLM() bool           { return false }
func (t runMCPTestTool) NewArgs() any {
	if t.newArgs != nil {
		return t.newArgs()
	}
	return &runMCPTestArgs{}
}
func (t runMCPTestTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	return t.execute(ctx, args, tc)
}

func TestRunMCPPreservesArgumentsContextAndResult(t *testing.T) {
	reg, tc := setupExecutionActivityTest(t)
	RegisterRunMCPTools(reg)
	tc.NovelID, tc.ToolID = 7, "outer-call"
	tc.Approver = approval.NewService(tc.Logger, "auto")
	emitted := false
	tc.EmitApproval = func(id, kind string, payload map[string]any) {
		require.Equal(t, tc.ToolID, id)
		require.Equal(t, "test", kind)
		require.Equal(t, map[string]any{"novel_id": tc.NovelID}, payload)
		emitted = true
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	want := &ToolResult{
		Success: true, Data: map[string]any{"content": "完成"},
		Metadata: map[string]any{"target": "low_frequency"},
		Inject:   []InjectMessage{{Role: "user", Content: "后续上下文"}},
	}
	const inner = `{"name":"test","id":9007199254740993,"refs":[{"id":9007199254740995}]}`
	reg.Register(runMCPTestTool{execute: func(gotCtx context.Context, args any, gotTC ToolContext) (*ToolResult, error) {
		require.Equal(t, ctx, gotCtx)
		require.Same(t, tc.DB, gotTC.DB)
		require.Same(t, tc.Logger, gotTC.Logger)
		require.Same(t, tc.Approver, gotTC.Approver)
		require.Equal(t, tc.NovelID, gotTC.NovelID)
		require.Equal(t, tc.ToolID, gotTC.ToolID)
		require.JSONEq(t, inner, string(gotTC.RawArgs))
		parsed := args.(*runMCPTestArgs)
		require.Equal(t, int64(9007199254740993), parsed.ID)
		require.Equal(t, int64(9007199254740995), parsed.Refs[0].ID)
		gotTC.EmitApproval(gotTC.ToolID, "test", map[string]any{"novel_id": gotTC.NovelID})
		decision, err := gotTC.Approver.RequestApproval(gotCtx, gotTC.ToolID, nil)
		require.NoError(t, err)
		require.True(t, decision.Approved)
		requireToolActivity(t, tc.DB, 1)
		return want, nil
	}})
	result := reg.Execute(ctx, "run_mcp", json.RawMessage(`{"tool_name":"low_frequency","args":`+inner+`}`), tc, agentcfg.Allowlist(agentcfg.MainAgent))
	require.Same(t, want, result)
	require.True(t, emitted)
	requireToolActivity(t, tc.DB, 1)
}

func TestRunMCPRejectsInvalidCalls(t *testing.T) {
	reg, tc := setupExecutionActivityTest(t)
	RegisterAllTools(reg)
	reg.Register(runMCPTestTool{execute: func(context.Context, any, ToolContext) (*ToolResult, error) {
		t.Fatal("被拒绝的目标不应执行")
		return nil, nil
	}})
	for _, tt := range []struct {
		name, args, message string
	}{
		{"unknown", `{"tool_name":"missing","args":{}}`, "工具不存在"},
		{"external", `{"tool_name":"server.search","args":{}}`, "工具不存在"},
		{"direct tool", `{"tool_name":"read","args":{"path":"goink.md"}}`, "不支持按需调用"},
		{"recursive", `{"tool_name":"run_mcp","args":{}}`, "不支持按需调用"},
		{"missing name", `{"args":{}}`, "参数校验失败"},
		{"missing args", `{"tool_name":"low_frequency"}`, "参数校验失败"},
		{"null args", `{"tool_name":"low_frequency","args":null}`, "参数校验失败"},
		{"array args", `{"tool_name":"low_frequency","args":[]}`, "参数格式不正确"},
		{"scalar args", `{"tool_name":"low_frequency","args":3}`, "参数格式不正确"},
		{"outer unknown field", `{"tool_name":"low_frequency","args":{},"novel_id":99}`, "参数格式不正确"},
		{"inner required field", `{"tool_name":"low_frequency","args":{}}`, "参数校验失败"},
		{"inner unknown field", `{"tool_name":"low_frequency","args":{"name":"test","novel_id":99}}`, "参数格式不正确"},
		{"inner field type", `{"tool_name":"low_frequency","args":{"name":"test","id":"wrong"}}`, "参数格式不正确"},
		{"inner validation", `{"tool_name":"low_frequency","args":{"name":"test","id":-1}}`, "参数校验失败"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result := reg.Execute(context.Background(), "run_mcp", json.RawMessage(tt.args), tc, agentcfg.Allowlist(agentcfg.MainAgent))
			require.False(t, result.Success)
			require.Contains(t, result.Error, tt.message)
		})
	}
	for _, role := range []agentcfg.AgentType{agentcfg.ReviewAgent, agentcfg.MemoryAgent} {
		result := reg.Execute(context.Background(), "run_mcp", json.RawMessage(`{"tool_name":"low_frequency","args":{"name":"test"}}`), tc, agentcfg.Allowlist(role))
		require.False(t, result.Success)
		require.Contains(t, result.Error, "工具禁止使用")
	}
	requireToolActivity(t, tc.DB, 0)
}

func TestRunMCPAcceptsEmptyObjectAndExposesOnlyEntry(t *testing.T) {
	reg, tc := setupExecutionActivityTest(t)
	RegisterRunMCPTools(reg)
	reg.Register(runMCPTestTool{
		newArgs: func() any { return &struct{}{} },
		execute: func(context.Context, any, ToolContext) (*ToolResult, error) {
			return &ToolResult{Success: true}, nil
		},
	})
	tools := reg.Definitions(agentcfg.Allowlist(agentcfg.MainAgent))
	require.Len(t, tools, 1)
	require.Equal(t, "run_mcp", tools[0].Name)
	var schema map[string]any
	require.NoError(t, json.Unmarshal(tools[0].InputSchema, &schema))
	require.ElementsMatch(t, []any{"tool_name", "args"}, schema["required"])
	argsSchema := schema["properties"].(map[string]any)["args"].(map[string]any)
	require.Equal(t, "object", argsSchema["type"])
	require.NotEqual(t, false, argsSchema["additionalProperties"])
	require.Len(t, reg.Definitions(nil), 1, "拓展工具不直接向模型展示")
	result := reg.Execute(context.Background(), "run_mcp", json.RawMessage(`{"tool_name":"low_frequency","args":{}}`), tc, agentcfg.Allowlist(agentcfg.MainAgent))
	require.True(t, result.Success, result.Error)
}

func TestRunMCPPreservesFailuresAndCountsOnlyTarget(t *testing.T) {
	for _, outcome := range []string{"business", "system", "panic"} {
		t.Run(outcome, func(t *testing.T) {
			reg, tc := setupExecutionActivityTest(t)
			RegisterRunMCPTools(reg)
			want := &ToolResult{Success: false, Error: "业务拒绝", Data: map[string]any{"reason": "test"}}
			reg.Register(activityTestTool{run: func() (*ToolResult, error) {
				switch outcome {
				case "system":
					return nil, errors.New("依赖不可用")
				case "panic":
					panic("test panic")
				default:
					return want, nil
				}
			}})
			result := reg.Execute(context.Background(), "run_mcp", json.RawMessage(`{"tool_name":"activity_test","args":{"name":"test"}}`), tc, nil)
			require.False(t, result.Success)
			if outcome == "business" {
				require.Same(t, want, result)
			} else {
				require.Equal(t, ErrKindSystem, result.ErrKind)
			}
			requireToolActivity(t, tc.DB, 1)
		})
	}
}

func TestRunMCPPropagatesCancellation(t *testing.T) {
	reg, tc := setupExecutionActivityTest(t)
	RegisterRunMCPTools(reg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	reg.Register(runMCPTestTool{execute: func(ctx context.Context, _ any, _ ToolContext) (*ToolResult, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}})
	done := make(chan *ToolResult, 1)
	go func() {
		done <- reg.Execute(ctx, "run_mcp", json.RawMessage(`{"tool_name":"low_frequency","args":{"name":"test"}}`), tc, nil)
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("目标工具未开始执行")
	}
	cancel()
	select {
	case result := <-done:
		require.False(t, result.Success)
		require.Equal(t, ErrKindSystem, result.ErrKind)
		require.Contains(t, result.Error, context.Canceled.Error())
	case <-time.After(3 * time.Second):
		t.Fatal("取消未传递到目标工具")
	}
	requireToolActivity(t, tc.DB, 1)
}

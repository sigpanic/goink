package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/sigpanic/goink/internal/activity"
	"github.com/sigpanic/goink/internal/llm"
	"github.com/sigpanic/goink/internal/llm/adapter"
	"github.com/sigpanic/goink/internal/mcp_tools"
	"github.com/sigpanic/goink/internal/session"
)

// 使用合成的 Goink 事件运行真实 Agent 循环，不连接 Wails 或外部模型。
type responseStateScript struct {
	t        *testing.T
	streams  [][]llm.StreamEvent
	requests []RunOptions
	contexts []context.Context
}

func (s *responseStateScript) open(ctx context.Context, opts RunOptions, _ []llm.ToolDefinition, _ *llm.CallOptions) <-chan llm.StreamEvent {
	s.t.Helper()
	i := len(s.requests)
	require.Less(s.t, i, len(s.streams), "unexpected LLM request")
	s.requests = append(s.requests, opts)
	s.contexts = append(s.contexts, ctx)
	ch := make(chan llm.StreamEvent, len(s.streams[i]))
	for _, event := range s.streams[i] {
		ch <- event
	}
	close(ch)
	return ch
}

func stateTestAgent(t *testing.T) (*Agent, RunOptions) {
	t.Helper()
	a := newCompressTestAgent(t, "state")
	require.NoError(t, a.db.AutoMigrate(&activity.DailyActivity{}))
	a.registry = mcp_tools.NewRegistry(a.logger)
	var err error
	a.llm, err = llm.NewClient(&llm.UserLLMConfig{Version: llm.ConfigVersion, Adapters: map[adapter.Protocol]llm.AdapterConfig{}}, a.logger)
	require.NoError(t, err)
	return a, RunOptions{
		SessionID: "state", TurnID: 1, ActiveVersion: 1, AgentType: "main", MaxTurns: 3,
		AdapterName: adapter.ProtocolAnthropicMessages, ProviderName: "relay", Model: &llm.ModelInfo{ID: "model"},
		Messages: []llm.Message{{Role: "user", Content: "继续"}},
	}
}

func stateFixture(marker string, turnComplete bool) *llm.ResponseState {
	return &llm.ResponseState{
		Origin:   llm.MessageOrigin{AdapterName: adapter.ProtocolAnthropicMessages, ProviderName: "relay", ProviderType: "anthropic", Model: "model"},
		Complete: true, TurnComplete: turnComplete, StopReason: "test",
		Payload: json.RawMessage(fmt.Sprintf(`{"signature":%q,"unknown":[9007199254740993,{}]}`, marker)),
	}
}

func stateRows(t *testing.T, a *Agent) []session.Message {
	t.Helper()
	var rows []session.Message
	require.NoError(t, a.db.Order("id").Find(&rows).Error)
	return rows
}

func TestRunPersistsResponseStateWithoutText(t *testing.T) {
	for _, agentType := range []string{"main", "review"} {
		t.Run(agentType, func(t *testing.T) {
			a, opts := stateTestAgent(t)
			opts.AgentType = agentType
			if agentType != "main" {
				opts.SubTaskID = "subtask"
			}
			state := stateFixture("private-empty", true)
			script := responseStateScript{t: t, streams: [][]llm.StreamEvent{{{Type: llm.EventResponseState, State: state}}}}
			result, err := a.run(context.Background(), opts, script.open, func(AgentEvent) {
				t.Error("response state must not be published as a UI event")
			})
			require.NoError(t, err)
			require.Empty(t, result.FinalText)
			rows := stateRows(t, a)
			require.Len(t, rows, 1)
			require.Equal(t, opts.SubTaskID, rows[0].SubTaskID)
			require.Equal(t, agentType, rows[0].AgentType)
			require.Equal(t, agentType == "main", rows[0].ToAPI)
			require.True(t, rows[0].ToFrontend)
			message, err := rows[0].ToLLMMessage()
			require.NoError(t, err)
			require.Equal(t, state, message.State)
			require.ErrorIs(t, script.contexts[0].Err(), context.Canceled)
		})
	}
}

type stateToolArgs struct {
	Chapter int64 `json:"chapter"`
}

type stateTestTool struct {
	execute func(context.Context, any, mcp_tools.ToolContext) (*mcp_tools.ToolResult, error)
}

func (stateTestTool) Name() string                     { return "state_tool" }
func (stateTestTool) Description() string              { return "测试工具" }
func (stateTestTool) Category() mcp_tools.ToolCategory { return mcp_tools.CategoryWritingAssistant }
func (stateTestTool) JSONSchema() json.RawMessage      { return mcp_tools.SchemaOf(stateToolArgs{}) }
func (stateTestTool) ExposeToLLM() bool                { return true }
func (stateTestTool) NewArgs() any                     { return &stateToolArgs{} }
func (s stateTestTool) Execute(ctx context.Context, args any, tc mcp_tools.ToolContext) (*mcp_tools.ToolResult, error) {
	return s.execute(ctx, args, tc)
}

func stateToolEvent(id string) llm.StreamEvent {
	return llm.StreamEvent{Type: llm.EventToolCallEnd, Delta: &llm.ToolCallDelta{
		ToolID: id, ToolName: "state_tool", ArgumentsJSON: json.RawMessage(`{"chapter":9007199254740993}`),
	}}
}

func TestRunReplaysToolResponseStateOnNextRequest(t *testing.T) {
	a, opts := stateTestAgent(t)
	var executed int
	a.registry.Register(stateTestTool{execute: func(_ context.Context, args any, _ mcp_tools.ToolContext) (*mcp_tools.ToolResult, error) {
		executed++
		require.EqualValues(t, 9007199254740993, args.(*stateToolArgs).Chapter)
		return &mcp_tools.ToolResult{Success: true}, nil
	}})
	waiting := stateFixture("private-tool-state", false)
	final := stateFixture("private-final-state", true)
	script := responseStateScript{t: t, streams: [][]llm.StreamEvent{
		{{Type: llm.EventContent, Data: "调用工具"}, {Type: llm.EventResponseState, State: waiting}, stateToolEvent("call_1"), stateToolEvent("call_2")},
		{{Type: llm.EventContent, Data: "完成"}, {Type: llm.EventResponseState, State: final}},
	}}
	var published []AgentEvent
	result, err := a.run(context.Background(), opts, script.open, func(e AgentEvent) { published = append(published, e) })
	require.NoError(t, err)
	require.Equal(t, "完成", result.FinalText)
	require.Equal(t, 2, executed)
	require.Len(t, script.requests, 2)
	history := script.requests[1].Messages
	require.Len(t, history, 4)
	require.Equal(t, waiting, history[1].State)
	require.False(t, history[1].State.TurnComplete)
	require.Len(t, history[1].ToolCalls, 2)
	require.Equal(t, "call_1", history[2].ToolCallID)
	require.Equal(t, "call_2", history[3].ToolCallID)
	rows := stateRows(t, a)
	require.Len(t, rows, 4)
	require.Contains(t, rows[0].ExtraMetadata, `"tool_displays"`)
	loaded := renderForAPI(t, rows)
	require.Equal(t, history[1:], loaded[:3])
	require.Equal(t, final, loaded[3].State)
	for _, ctx := range script.contexts {
		require.ErrorIs(t, ctx.Err(), context.Canceled)
	}
	visible, err := json.Marshal(published)
	require.NoError(t, err)
	require.NotContains(t, string(visible), "private-tool-state")
	require.NotContains(t, string(visible), "private-final-state")
}

func TestRunRetryDoesNotReuseResponseState(t *testing.T) {
	for _, withState := range []bool{false, true} {
		t.Run(fmt.Sprint(withState), func(t *testing.T) {
			a, opts := stateTestAgent(t)
			var final *llm.ResponseState
			next := []llm.StreamEvent{{Type: llm.EventContent, Data: "完成"}}
			if withState {
				final = stateFixture("private-retry-success", true)
				next = append(next, llm.StreamEvent{Type: llm.EventResponseState, State: final})
			}
			script := responseStateScript{t: t, streams: [][]llm.StreamEvent{
				{{Type: llm.EventContent, Data: "丢弃的 partial"}, {Type: llm.EventResponseState, State: stateFixture("private-discarded", true)},
					{Type: llm.EventError, Error: &llm.APIError{StatusCode: 429, Message: "retry", Retryable: true, RetryAfter: time.Millisecond}}},
				next,
			}}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			var retries int
			result, err := a.run(ctx, opts, script.open, func(e AgentEvent) {
				if e.Type == EventRetrying {
					retries++
				}
			})
			require.NoError(t, err)
			require.Equal(t, "完成", result.FinalText)
			require.Equal(t, 1, retries)
			require.Len(t, script.requests, 2)
			require.Len(t, script.requests[1].Messages, 1)
			rows := stateRows(t, a)
			require.Len(t, rows, 1)
			message, err := rows[0].ToLLMMessage()
			require.NoError(t, err)
			require.Equal(t, final, message.State)
			require.Equal(t, "完成", message.Content)
		})
	}
}

func TestRunPartialResponseDoesNotKeepState(t *testing.T) {
	for _, mode := range []string{"cancel", "error", "incomplete"} {
		t.Run(mode, func(t *testing.T) {
			a, opts := stateTestAgent(t)
			state := stateFixture("private-partial", true)
			state.Complete = mode != "incomplete"
			events := []llm.StreamEvent{{Type: llm.EventResponseState, State: state}, {Type: llm.EventContent, Data: "partial"}}
			if mode == "error" {
				events = append(events, llm.StreamEvent{Type: llm.EventError, Error: &llm.APIError{StatusCode: 400, Message: "failed"}})
			}
			script := responseStateScript{t: t, streams: [][]llm.StreamEvent{events}}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			_, err := a.run(ctx, opts, script.open, func(e AgentEvent) {
				if mode == "cancel" && e.Type == EventContent {
					cancel()
				}
			})
			switch mode {
			case "cancel":
				require.ErrorIs(t, err, context.Canceled)
			case "error":
				require.Error(t, err)
			default:
				require.NoError(t, err)
			}
			rows := stateRows(t, a)
			require.Len(t, rows, 1)
			message, err := rows[0].ToLLMMessage()
			require.NoError(t, err)
			require.Equal(t, "partial", message.Content)
			require.Nil(t, message.State)
			require.ErrorIs(t, script.contexts[0].Err(), context.Canceled)
		})
	}
}

func TestRunInterruptedToolGroupKeepsTriggeringResultWithoutState(t *testing.T) {
	a, opts := stateTestAgent(t)
	var executed int
	a.registry.Register(stateTestTool{execute: func(context.Context, any, mcp_tools.ToolContext) (*mcp_tools.ToolResult, error) {
		executed++
		return &mcp_tools.ToolResult{Success: false, Error: "参数失败", ErrKind: mcp_tools.ErrKindArgs}, nil
	}})
	events := []llm.StreamEvent{{Type: llm.EventResponseState, State: stateFixture("private-interrupted", false)}}
	for i := 1; i <= 6; i++ {
		events = append(events, stateToolEvent(fmt.Sprint(i)))
	}
	script := responseStateScript{t: t, streams: [][]llm.StreamEvent{events}}
	_, err := a.run(context.Background(), opts, script.open, func(AgentEvent) {})
	var interrupted *ToolFailureInterrupt
	require.ErrorAs(t, err, &interrupted)
	require.Equal(t, 5, executed)
	rows := stateRows(t, a)
	require.Len(t, rows, 7)
	loaded := renderForAPI(t, rows)
	require.Nil(t, loaded[0].State)
	require.Len(t, loaded[0].ToolCalls, 6)
	for i := 1; i <= 6; i++ {
		require.Equal(t, fmt.Sprint(i), loaded[i].ToolCallID)
	}
	require.Contains(t, loaded[5].Content, "参数失败")
	require.Contains(t, loaded[6].Content, "操作被中断")
	require.ErrorIs(t, script.contexts[0].Err(), context.Canceled)
}

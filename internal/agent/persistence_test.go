package agent

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/llm"
	"github.com/sigpanic/goink/internal/mcp_tools"
)

func failMessageWrite(t *testing.T, a *Agent, at int) error {
	t.Helper()
	failure := errors.New("injected message write failure")
	attempts := 0
	require.NoError(t, a.db.Callback().Create().Before("gorm:create").Register("test:fail_message_write", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "messages" {
			attempts++
			if attempts == at {
				tx.AddError(failure)
			}
		}
	}))
	return failure
}

func TestAppendMsgFailureKeepsHistoryAndTokens(t *testing.T) {
	for _, tt := range []struct {
		name    string
		message llm.Message
		extra   map[string]any
		write   bool
		wantErr string
	}{
		{name: "metadata", message: llm.Message{Role: "assistant"}, extra: map[string]any{"bad": make(chan int)}, wantErr: "encode metadata"},
		{name: "tool arguments", message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "bad", Name: "read", Arguments: json.RawMessage(`{`)}}}, wantErr: "invalid tool arguments JSON"},
		{name: "database", message: llm.Message{Role: "assistant", Content: "未保存的回复"}, write: true, wantErr: "persist assistant message"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a := newCompressTestAgent(t, "failure")
			history := []llm.Message{{Role: "user", Content: "已有历史"}}
			opts := RunOptions{SessionID: "failure", ActiveVersion: 1, AgentType: "main", Messages: history}
			tokens := a.InitRunningTokens(history)
			before := maps.Clone(tokens)
			var failure error
			if tt.write {
				failure = failMessageWrite(t, a, 1)
			}
			err := a.appendMsg(tt.message, tt.extra, &opts, tokens)
			require.ErrorContains(t, err, tt.wantErr)
			if failure != nil {
				require.ErrorIs(t, err, failure)
			}
			require.Equal(t, history, opts.Messages)
			require.Equal(t, before, tokens)
			require.Empty(t, stateRows(t, a))
		})
	}
}

func TestRunStopsOnMessageWriteFailure(t *testing.T) {
	content := llm.StreamEvent{Type: llm.EventContent, Data: "partial"}
	toolEvents := []llm.StreamEvent{content, stateToolEvent("call_1")}
	modelErr := &llm.APIError{StatusCode: 400, Message: "model failure"}
	argsErr := &llm.APIError{Kind: "tool_args_invalid", Message: "invalid arguments", Retryable: true}
	for _, tt := range []struct {
		name      string
		failAt    int
		events    []llm.StreamEvent
		cause     error
		cancel    bool
		agentType string
	}{
		{name: "final response", failAt: 1, events: []llm.StreamEvent{content}},
		{name: "subagent final response", failAt: 1, events: []llm.StreamEvent{content}, agentType: "review"},
		{name: "state only", failAt: 1, events: []llm.StreamEvent{{Type: llm.EventResponseState, State: stateFixture("unsaved", true)}}},
		{name: "assistant tool calls", failAt: 1, events: toolEvents},
		{name: "tool result", failAt: 2, events: toolEvents},
		{name: "injection", failAt: 3, events: toolEvents},
		{name: "retry reminder", failAt: 1, events: []llm.StreamEvent{{Type: llm.EventError, Error: argsErr}}, cause: argsErr},
		{name: "partial on error", failAt: 1, events: []llm.StreamEvent{content, {Type: llm.EventError, Error: modelErr}}, cause: modelErr},
		{name: "partial on cancel", failAt: 1, events: []llm.StreamEvent{content}, cancel: true, cause: context.Canceled},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a, opts := stateTestAgent(t)
			if tt.agentType != "" {
				opts.AgentType = tt.agentType
				opts.SubTaskID = "subtask"
			}
			failure := failMessageWrite(t, a, tt.failAt)
			a.registry.Register(stateTestTool{execute: func(context.Context, any, mcp_tools.ToolContext) (*mcp_tools.ToolResult, error) {
				return &mcp_tools.ToolResult{Success: true, Inject: []mcp_tools.InjectMessage{
					{Role: "user", Content: "第一条提醒"}, {Role: "user", Content: "第二条提醒"},
				}}, nil
			}})
			script := responseStateScript{t: t, streams: [][]llm.StreamEvent{tt.events}}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			result, err := a.run(ctx, opts, script.open, func(e AgentEvent) {
				require.NotEqual(t, EventRetrying, e.Type, "a failed message write must not retry the model request")
				if tt.cancel && e.Type == EventContent {
					cancel()
				}
			})
			require.ErrorIs(t, err, failure)
			if tt.cause != nil {
				require.ErrorIs(t, err, tt.cause)
			}
			if len(tt.events) > 0 && tt.events[0].Type == llm.EventContent {
				require.Equal(t, "partial", result.FinalText)
			}
			require.Len(t, script.requests, 1)
			require.Len(t, stateRows(t, a), tt.failAt-1, "stop writing after the first failure without rolling back earlier messages")
			require.ErrorIs(t, script.contexts[0].Err(), context.Canceled)
		})
	}
}

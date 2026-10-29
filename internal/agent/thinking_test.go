package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sigpanic/goink/internal/llm"
	"github.com/sigpanic/goink/internal/mcp_tools"
)

func TestRunPersistsThinkingPresenceWithoutText(t *testing.T) {
	for _, kind := range []string{"main", "review"} {
		t.Run(kind, func(t *testing.T) {
			a, opts := stateTestAgent(t)
			opts.AgentType = kind
			script := responseStateScript{t: t, streams: [][]llm.StreamEvent{{
				{Type: llm.EventThinking},
				{Type: llm.EventThinkingDone},
				{Type: llm.EventContent, Data: "完成"},
			}}}
			var phases []AgentEventType
			_, err := a.run(context.Background(), opts, script.open, func(event AgentEvent) {
				phases = append(phases, event.Type)
			})
			require.NoError(t, err)
			require.Equal(t, []AgentEventType{EventThinking, EventThinkingDone, EventContent}, phases)
			rows := stateRows(t, a)
			require.Len(t, rows, 1)
			require.True(t, rows[0].HasThinking)
			require.Empty(t, rows[0].ThinkingContent)
			message, err := rows[0].ToLLMMessage()
			require.NoError(t, err)
			require.True(t, message.HasThinking)
			require.Equal(t, "完成", message.Content)
		})
	}
}

func TestThinkingPresenceDoesNotLeakAcrossToolLoop(t *testing.T) {
	a, opts := stateTestAgent(t)
	a.registry.Register(stateTestTool{execute: func(context.Context, any, mcp_tools.ToolContext) (*mcp_tools.ToolResult, error) {
		return &mcp_tools.ToolResult{Success: true}, nil
	}})
	script := responseStateScript{t: t, streams: [][]llm.StreamEvent{
		{{Type: llm.EventThinking}, {Type: llm.EventThinkingDone}, stateToolEvent("call")},
		{{Type: llm.EventContent, Data: "完成"}},
	}}
	_, err := a.run(context.Background(), opts, script.open, func(AgentEvent) {})
	require.NoError(t, err)
	rows := stateRows(t, a)
	require.Len(t, rows, 3)
	require.True(t, rows[0].HasThinking)
	require.False(t, rows[1].HasThinking)
	require.False(t, rows[2].HasThinking)
	require.True(t, script.requests[1].Messages[1].HasThinking)
}

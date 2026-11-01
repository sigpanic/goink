package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sigpanic/goink/internal/llm"
	"github.com/sigpanic/goink/internal/mcp_tools"
)

func TestRunContextWindowGuard(t *testing.T) {
	for _, tc := range []struct {
		name       string
		copies     int
		windowSize int
		wantError  bool
	}{
		{"exactly 95 percent", 19, 20, true},
		{"full window", 1, 1, true},
		{"history exceeds smaller model", 2, 1, true},
		{"below compression threshold", 1, 2, false},
		{"unknown window", 1, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, opts := stateTestAgent(t)
			message := opts.Messages[0]
			tokens, err := llm.CountMessageTokens(message)
			require.NoError(t, err)
			require.Positive(t, tokens)
			opts.Model.ContextWindow = tokens * tc.windowSize
			opts.Messages = make([]llm.Message, tc.copies)
			for i := range opts.Messages {
				opts.Messages[i] = message
			}
			script := responseStateScript{t: t, streams: [][]llm.StreamEvent{{{Type: llm.EventContent, Data: "完成"}}}}
			var published []AgentEvent
			result, err := a.run(context.Background(), opts, script.open, func(event AgentEvent) {
				published = append(published, event)
			})
			if tc.wantError {
				require.ErrorContains(t, err, "95%")
				require.Contains(t, FriendlyError(err), "请换用上下文窗口更大的模型压缩后再继续")
				require.Empty(t, script.requests)
				require.Empty(t, published, "must stop before starting compression")
				require.Empty(t, stateRows(t, a))
				require.Zero(t, result.TurnCount)
			} else {
				require.NoError(t, err)
				require.Len(t, script.requests, 1)
				require.Equal(t, "完成", result.FinalText)
			}
		})
	}
}

func TestRunContextWindowGuardAfterToolResult(t *testing.T) {
	a, opts := stateTestAgent(t)
	opts.Model.ContextWindow = sumRunningTokens(a.InitRunningTokens(opts.Messages)) * 4
	var executed int
	a.registry.Register(stateTestTool{execute: func(context.Context, any, mcp_tools.ToolContext) (*mcp_tools.ToolResult, error) {
		executed++
		return &mcp_tools.ToolResult{Success: true, Data: map[string]any{"content": strings.Repeat("章节内容", 500)}}, nil
	}})
	script := responseStateScript{t: t, streams: [][]llm.StreamEvent{{stateToolEvent("call-1")}}}
	var published []AgentEvent
	result, err := a.run(context.Background(), opts, script.open, func(event AgentEvent) {
		published = append(published, event)
	})
	require.ErrorContains(t, err, "请换用上下文窗口更大的模型压缩后再继续")
	require.Equal(t, 1, executed)
	require.Len(t, script.requests, 1, "must not request again after a large tool result")
	require.Equal(t, 1, result.TurnCount)
	for _, event := range published {
		require.NotEqual(t, EventCompression, event.Type)
	}
	rows := stateRows(t, a)
	require.Len(t, rows, 2)
	require.Equal(t, "assistant", rows[0].Role)
	require.Equal(t, "tool", rows[1].Role)
	require.Contains(t, rows[1].Content, "章节内容")
}

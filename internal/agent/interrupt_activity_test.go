package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sigpanic/goink/internal/activity"
	"github.com/sigpanic/goink/internal/llm"
)

func TestInterruptedPendingToolsDoNotCountActivity(t *testing.T) {
	a := newCompressTestAgent(t, "activity")
	require.NoError(t, a.db.AutoMigrate(&activity.DailyActivity{}))
	stream := make(chan llm.StreamEvent, 1)
	stream <- llm.StreamEvent{Type: llm.EventToolCallEnd, Delta: &llm.ToolCallDelta{
		ToolName: "activity_test", ToolID: "skipped", ArgumentsJSON: json.RawMessage(`{"name":"test"}`),
	}}
	close(stream)
	var outputs []toolOutput
	a.flushInterruptedTools(context.Background(), stream, &RunOptions{}, &outputs)
	require.Len(t, outputs, 1)
	var rows []activity.DailyActivity
	require.NoError(t, a.db.Find(&rows).Error)
	require.Empty(t, rows)
}

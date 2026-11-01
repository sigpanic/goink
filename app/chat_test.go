package app

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/sigpanic/goink/internal/session"
)

// newUserMsg 构造一条 user 消息，Version=1, ToAPI=true, ToFrontend=true, AgentType="main".
func newUserMsg(sessionID string, turnID int, content string) *session.Message {
	return &session.Message{
		SessionID:  sessionID,
		TurnID:     turnID,
		Role:       "user",
		Content:    content,
		Version:    1,
		ToAPI:      true,
		ToFrontend: true,
		AgentType:  "main",
	}
}

// newAssistantMsgWithToolCalls 构造一条 assistant 消息，ExtraMetadata JSON 中包含 tool_calls 数组.
func newAssistantMsgWithToolCalls(sessionID string, turnID int, toolCalls []map[string]any) *session.Message {
	meta := map[string]any{"tool_calls": toolCalls}
	metaJSON, _ := json.Marshal(meta)
	return &session.Message{
		SessionID:     sessionID,
		TurnID:        turnID,
		Role:          "assistant",
		Content:       "",
		ExtraMetadata: string(metaJSON),
		Version:       1,
		ToAPI:         true,
		ToFrontend:    true,
		AgentType:     "main",
	}
}

// newToolMsg 构造一条 tool 消息，ExtraMetadata JSON 包含 tool_call_id 和 tool_name.
func newToolMsg(sessionID string, turnID int, toolCallID, toolName, content string) *session.Message {
	meta := map[string]any{
		"tool_call_id": toolCallID,
		"tool_name":    toolName,
	}
	metaJSON, _ := json.Marshal(meta)
	return &session.Message{
		SessionID:     sessionID,
		TurnID:        turnID,
		Role:          "tool",
		Content:       content,
		ExtraMetadata: string(metaJSON),
		Version:       1,
		ToAPI:         true,
		ToFrontend:    true,
		AgentType:     "main",
	}
}

// toolCall 构造一个 DeepSeek 风格的 tool_call map 元素.
func toolCall(id, name string) map[string]any {
	return map[string]any{
		"id":   id,
		"type": "function",
		"function": map[string]any{
			"name":      name,
			"arguments": "{}",
		},
	}
}

// createMsgsInOrder 按顺序插入多条消息，通过显式设置递增的 CreatedAt 保证
// GetMessagesForAPI 的 ORDER BY created_at ASC 顺序与插入顺序一致.
// GORM 对 autoCreateTime 字段在非零值时不会覆盖，因此显式设置 CreatedAt 是有效的.
func createMsgsInOrder(t *testing.T, app *App, msgs ...*session.Message) {
	t.Helper()
	base := time.Now().Add(-time.Hour)
	for i, m := range msgs {
		m.CreatedAt = base.Add(time.Duration(i) * time.Second)
		require.NoError(t, app.db.Create(m).Error, "create message index=%d", i)
	}
}

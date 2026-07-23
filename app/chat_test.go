package app

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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

// ---------------------------------------------------------------------------
// Test 1: 正常情况 — 无重复，所有消息保留
// ---------------------------------------------------------------------------

func TestLoadAPIMessages_NoDuplicates(t *testing.T) {
	app := setupTestApp(t)
	ctx := context.Background()
	sessionID := "test_session_no_dup"

	msgs := []*session.Message{
		newUserMsg(sessionID, 1, "hello"),
		newAssistantMsgWithToolCalls(sessionID, 1, []map[string]any{
			toolCall("call_a", "foo"),
			toolCall("call_b", "bar"),
		}),
		newToolMsg(sessionID, 1, "call_a", "foo", "result_a"),
		newToolMsg(sessionID, 1, "call_b", "bar", "result_b"),
	}
	createMsgsInOrder(t, app, msgs...)

	result, err := app.loadAPIMessages(ctx, sessionID, 1)
	require.NoError(t, err)
	require.Len(t, result, 4, "无重复时所有消息应保留")

	// 验证角色顺序
	assert.Equal(t, "user", result[0]["role"])
	assert.Equal(t, "assistant", result[1]["role"])
	assert.Equal(t, "tool", result[2]["role"])
	assert.Equal(t, "tool", result[3]["role"])

	// 验证 assistant.tool_calls 顺序和数量不变
	tc, ok := result[1]["tool_calls"].([]any)
	require.True(t, ok, "tool_calls 应为 []any")
	require.Len(t, tc, 2, "tool_calls 数量不变")
	assert.Equal(t, "call_a", tc[0].(map[string]any)["id"])
	assert.Equal(t, "call_b", tc[1].(map[string]any)["id"])

	// 验证两条 tool 消息都保留且对应正确的 tool_call_id
	assert.Equal(t, "call_a", result[2]["tool_call_id"])
	assert.Equal(t, "call_b", result[3]["tool_call_id"])
}

// ---------------------------------------------------------------------------
// Test 2: assistant.tool_calls 内部重复 id 去重
// ---------------------------------------------------------------------------

func TestLoadAPIMessages_AssistantToolCallsDeduped(t *testing.T) {
	app := setupTestApp(t)
	ctx := context.Background()
	sessionID := "test_session_dup_tc"

	msgs := []*session.Message{
		newUserMsg(sessionID, 1, "hello"),
		newAssistantMsgWithToolCalls(sessionID, 1, []map[string]any{
			toolCall("call_a", "foo"),
			toolCall("call_a", "foo"), // 重复 id（模拟 DeepSeek Bug A 持久化脏数据）
			toolCall("call_b", "bar"), // 不同 id
		}),
	}
	createMsgsInOrder(t, app, msgs...)

	result, err := app.loadAPIMessages(ctx, sessionID, 1)
	require.NoError(t, err)
	require.Len(t, result, 2, "user + assistant 两条消息")

	assistant := result[1]
	assert.Equal(t, "assistant", assistant["role"])

	tc, ok := assistant["tool_calls"].([]any)
	require.True(t, ok)
	require.Len(t, tc, 2, "去重后应只剩 2 个 tool_call")

	// 验证保留首次出现的 id（call_a 在前，call_b 在后）
	assert.Equal(t, "call_a", tc[0].(map[string]any)["id"])
	assert.Equal(t, "call_b", tc[1].(map[string]any)["id"])
}

// ---------------------------------------------------------------------------
// Test 3: orphan tool 消息（无对应 tool_call）被跳过
// ---------------------------------------------------------------------------

func TestLoadAPIMessages_OrphanToolMessageSkipped(t *testing.T) {
	app := setupTestApp(t)
	ctx := context.Background()
	sessionID := "test_session_orphan"

	msgs := []*session.Message{
		newUserMsg(sessionID, 1, "hello"),
		newAssistantMsgWithToolCalls(sessionID, 1, []map[string]any{
			toolCall("call_a", "foo"),
		}),
		newToolMsg(sessionID, 1, "call_a", "foo", "result_a"),
		newToolMsg(sessionID, 1, "call_orphan", "bar", "result_orphan"), // orphan：无匹配 tool_call
	}
	createMsgsInOrder(t, app, msgs...)

	result, err := app.loadAPIMessages(ctx, sessionID, 1)
	require.NoError(t, err)
	require.Len(t, result, 3, "orphan tool 消息应被跳过")

	// 验证最后一条是 call_a 的 tool 消息（而非 orphan）
	assert.Equal(t, "tool", result[2]["role"])
	assert.Equal(t, "call_a", result[2]["tool_call_id"])
}

// ---------------------------------------------------------------------------
// Test 4: 重复 tool 消息（同一 tool_call_id）只保留首次出现的
// ---------------------------------------------------------------------------

func TestLoadAPIMessages_DuplicateToolMessageSkipped(t *testing.T) {
	app := setupTestApp(t)
	ctx := context.Background()
	sessionID := "test_session_dup_tool"

	msgs := []*session.Message{
		newUserMsg(sessionID, 1, "hello"),
		newAssistantMsgWithToolCalls(sessionID, 1, []map[string]any{
			toolCall("call_a", "foo"),
		}),
		newToolMsg(sessionID, 1, "call_a", "foo", "first"),
		newToolMsg(sessionID, 1, "call_a", "foo", "second"), // 重复 tool_call_id
	}
	createMsgsInOrder(t, app, msgs...)

	result, err := app.loadAPIMessages(ctx, sessionID, 1)
	require.NoError(t, err)
	require.Len(t, result, 3, "重复 tool 消息应只保留一条")

	// 验证保留的是首次出现的（content="first"）
	assert.Equal(t, "tool", result[2]["role"])
	assert.Equal(t, "call_a", result[2]["tool_call_id"])
	assert.Equal(t, "first", result[2]["content"])
}

// ---------------------------------------------------------------------------
// Test 5: 跨 turn 不误判 — call_a 和 call_b 不互相干扰
// ---------------------------------------------------------------------------

func TestLoadAPIMessages_MultipleTurns(t *testing.T) {
	app := setupTestApp(t)
	ctx := context.Background()
	sessionID := "test_session_multi_turn"

	msgs := []*session.Message{
		// turn 1
		newUserMsg(sessionID, 1, "turn1"),
		newAssistantMsgWithToolCalls(sessionID, 1, []map[string]any{
			toolCall("call_a", "foo"),
		}),
		newToolMsg(sessionID, 1, "call_a", "foo", "result_a"),
		// turn 2
		newUserMsg(sessionID, 2, "turn2"),
		newAssistantMsgWithToolCalls(sessionID, 2, []map[string]any{
			toolCall("call_b", "bar"),
		}),
		newToolMsg(sessionID, 2, "call_b", "bar", "result_b"),
	}
	createMsgsInOrder(t, app, msgs...)

	result, err := app.loadAPIMessages(ctx, sessionID, 1)
	require.NoError(t, err)
	require.Len(t, result, 6, "跨 turn 消息应全部保留")

	// 验证 turn 1 的 assistant.tool_calls
	assert.Equal(t, "assistant", result[1]["role"])
	tc1, ok := result[1]["tool_calls"].([]any)
	require.True(t, ok)
	require.Len(t, tc1, 1)
	assert.Equal(t, "call_a", tc1[0].(map[string]any)["id"])

	// 验证 turn 2 的 assistant.tool_calls
	assert.Equal(t, "assistant", result[4]["role"])
	tc2, ok := result[4]["tool_calls"].([]any)
	require.True(t, ok)
	require.Len(t, tc2, 1)
	assert.Equal(t, "call_b", tc2[0].(map[string]any)["id"])

	// 验证两条 tool 消息都保留且对应正确
	assert.Equal(t, "call_a", result[2]["tool_call_id"])
	assert.Equal(t, "call_b", result[5]["tool_call_id"])
}

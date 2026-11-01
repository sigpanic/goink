package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/llm"
	"github.com/sigpanic/goink/internal/llm/adapter"
	"github.com/sigpanic/goink/internal/session"
)

func setupLLMMessageTestApp(t *testing.T) *App {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "messages.db")), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&session.Message{}))
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &App{db: db, logger: logger, session: session.NewStore(db, logger)}
}

func TestLoadLLMMessagesPreservesNativeHistory(t *testing.T) {
	for _, protocol := range []adapter.Protocol{adapter.ProtocolAnthropicMessages, adapter.ProtocolGeminiInteractions, adapter.ProtocolOpenAIResponses} {
		t.Run(string(protocol), func(t *testing.T) {
			a := setupLLMMessageTestApp(t)
			base := time.Date(2020, 1, 2, 3, 4, 0, 0, time.Local)
			history := []llm.Message{
				{Role: "user", Content: "继续"},
				{Role: "assistant", Content: "读取章节", ThinkingContent: "显示思考", ToolCalls: []llm.ToolCall{
					{ID: "call_1", Name: "read", Arguments: json.RawMessage(`{"chapter":9007199254740993}`)},
					{ID: "call_2", Name: "read", Arguments: json.RawMessage(`{}`)},
				}, State: &llm.ResponseState{
					Origin:   llm.MessageOrigin{AdapterName: protocol, ProviderName: "relay", ProviderType: "test", Model: "model"},
					Complete: true, StopReason: "test", Payload: json.RawMessage(`{"signature":"opaque-test-signature","unknown":[9007199254740993,{}]}`),
				}},
				{Role: "tool", ToolCallID: "call_1", ToolName: "read", Content: ""},
				{Role: "tool", ToolCallID: "call_2", ToolName: "read", Content: "章节原文"},
				{Role: "user", Content: "[2019-01-01 12:00] 已渲染的保留消息"},
			}
			for i, message := range history {
				draft := session.Message{
					SessionID: "target", TurnID: 1, Version: 2, AgentType: "main", ToAPI: true,
					ToFrontend: i == 0, CreatedAt: base.Add(time.Duration(i) * time.Second),
				}
				if message.Role == "assistant" {
					draft.ExtraMetadata = `{"tool_displays":[{"display_text":"读取章节"}],"audit":"keep"}`
				}
				row, err := draft.WithLLMMessage(message)
				require.NoError(t, err)
				require.NoError(t, a.db.Create(&row).Error)
			}
			for _, excluded := range []session.Message{
				{SessionID: "target", Version: 1, ToAPI: true},
				{SessionID: "target", Version: 2, ToAPI: false},
				{SessionID: "other", Version: 2, ToAPI: true},
			} {
				excluded.Role = "assistant"
				excluded.ExtraMetadata = "{broken excluded metadata"
				require.NoError(t, a.db.Create(&excluded).Error)
			}
			var before []session.Message
			require.NoError(t, a.db.Order("id").Find(&before).Error)
			loaded, err := a.loadLLMMessages(context.Background(), "target", 2)
			require.NoError(t, err)
			history[0].Content = "[2020-01-02 03:04] 继续"
			require.Equal(t, history, loaded)
			again, err := a.loadLLMMessages(context.Background(), "target", 2)
			require.NoError(t, err)
			require.Equal(t, loaded, again)
			var after []session.Message
			require.NoError(t, a.db.Order("id").Find(&after).Error)
			require.Equal(t, before, after, "loading must not rewrite stored content or metadata")
		})
	}
}

func TestLoadLLMMessagesLeavesHistoryCleanupToAdapter(t *testing.T) {
	a := setupLLMMessageTestApp(t)
	createMsgsInOrder(t, a,
		newUserMsg("history", 1, "继续"),
		newAssistantMsgWithToolCalls("history", 1, []map[string]any{
			toolCall("call_a", "read"), toolCall("call_a", "read"), toolCall("call_b", "read"),
		}),
		newToolMsg("history", 1, "call_a", "read", "first"),
		newToolMsg("history", 1, "call_a", "read", "second"),
		newToolMsg("history", 1, "orphan", "read", "orphan result"),
		&session.Message{SessionID: "history", Version: 1, ToAPI: true, Role: "system", Content: "中途指令"},
	)
	loaded, err := a.loadLLMMessages(context.Background(), "history", 1)
	require.NoError(t, err)
	require.Len(t, loaded, 6)
	require.Len(t, loaded[1].ToolCalls, 3, "duplicate and unpaired calls must reach the target adapter")
	require.Equal(t, "first", loaded[2].Content)
	require.Equal(t, "second", loaded[3].Content)
	require.Equal(t, "orphan", loaded[4].ToolCallID)
	require.Equal(t, llm.Message{Role: "system", Content: "中途指令"}, loaded[5])
}

func TestLoadLLMMessagesRejectsCorruptMetadata(t *testing.T) {
	a := setupLLMMessageTestApp(t)
	valid := newUserMsg("corrupt", 1, "正常消息")
	corrupt := &session.Message{
		SessionID: "corrupt", Version: 1, ToAPI: true, Role: "assistant",
		ExtraMetadata: `{"tool_calls":[{"id":"bad","type":"function","function":{"name":"read","arguments":"{"}}]}`,
	}
	createMsgsInOrder(t, a, valid, corrupt)
	loaded, err := a.loadLLMMessages(context.Background(), "corrupt", 1)
	require.ErrorContains(t, err, fmt.Sprintf("message %d", corrupt.ID))
	require.ErrorContains(t, err, "invalid tool arguments JSON")
	require.Nil(t, loaded, "do not return a silently truncated history")
	var stored session.Message
	require.NoError(t, a.db.First(&stored, corrupt.ID).Error)
	require.Equal(t, corrupt.ExtraMetadata, stored.ExtraMetadata)
}

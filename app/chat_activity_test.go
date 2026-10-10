package app

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/activity"
	"github.com/sigpanic/goink/internal/llm"
	"github.com/sigpanic/goink/internal/session"
)

func setupChatActivityTest(t *testing.T) (*App, ChatInput) {
	t.Helper()
	a := setupTestApp(t)
	n := createTestNovel(t, a)
	a.llmClient.Reload(map[string]llm.Provider{
		"test": {Models: []llm.ModelInfo{{ID: "test"}}},
	})
	input := ChatInput{SessionID: "activity-chat", NovelID: n.ID, ProviderName: "test", ModelID: "test", Message: "继续创作"}
	require.NoError(t, a.db.Create(&session.Session{SessionID: input.SessionID, NovelID: n.ID, Title: "统计测试"}).Error)
	// 在已接受消息之后模拟读取失败，避免测试依赖 Wails 事件运行时和真实模型。
	require.NoError(t, a.db.Callback().Query().Before("gorm:query").Register("test:reject_api_messages", func(tx *gorm.DB) {
		if tx.Statement.Table == "messages" {
			_ = tx.AddError(errors.New("API messages unavailable"))
		}
	}))
	return a, input
}

func requireConversationTurns(t *testing.T, a *App, want int64) {
	t.Helper()
	var rows []activity.DailyActivity
	require.NoError(t, a.db.Find(&rows).Error)
	var got int64
	for _, row := range rows {
		got += row.ConversationTurns
	}
	require.Equal(t, want, got)
}

func TestChatActivityCountsAcceptedRequestsDespiteLaterFailure(t *testing.T) {
	a, input := setupChatActivityTest(t)
	for i := int64(1); i <= 2; i++ {
		_, err := a.Chat(input)
		require.ErrorContains(t, err, "API messages unavailable")
		requireConversationTurns(t, a, i)
	}
	var messages int64
	require.NoError(t, a.db.Raw("SELECT COUNT(*) FROM messages WHERE role = 'user'").Scan(&messages).Error)
	require.Equal(t, int64(2), messages)
}

func TestChatActivitySkipsRejectedRequests(t *testing.T) {
	a, input := setupChatActivityTest(t)
	invalid := input
	invalid.ModelID = "missing"
	_, err := a.Chat(invalid)
	require.ErrorContains(t, err, "模型未找到")
	requireConversationTurns(t, a, 0)

	require.NoError(t, a.db.Exec(`CREATE TRIGGER reject_user_message BEFORE INSERT ON messages
		WHEN NEW.role = 'user' BEGIN SELECT RAISE(ABORT, 'message rejected'); END`).Error)
	_, err = a.Chat(input)
	require.ErrorContains(t, err, "持久化消息失败")
	requireConversationTurns(t, a, 0)
}

func TestChatActivityFailureDoesNotBlockAcceptedMessage(t *testing.T) {
	a, input := setupChatActivityTest(t)
	require.NoError(t, a.db.Exec(`CREATE TRIGGER reject_activity BEFORE INSERT ON activity_daily
		BEGIN SELECT RAISE(ABORT, 'statistics unavailable'); END`).Error)
	_, err := a.Chat(input)
	require.ErrorContains(t, err, "API messages unavailable")
	var messages int64
	require.NoError(t, a.db.Raw("SELECT COUNT(*) FROM messages WHERE role = 'user'").Scan(&messages).Error)
	require.Equal(t, int64(1), messages)
	requireConversationTurns(t, a, 0)
}

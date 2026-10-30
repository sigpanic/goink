package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/agent"
	"github.com/sigpanic/goink/internal/llm"
	"github.com/sigpanic/goink/internal/session"
)

func TestSaveChatOutcomePreservesClassificationAndHistoryReason(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		kind session.EventType
		text string
	}{
		{"ordinary error", errors.New("加载 API 历史失败"), session.EventError, "对话出错：加载 API 历史失败"},
		{"missing terminal", fmt.Errorf("gemini stream ended without a terminal interaction: %w", io.ErrUnexpectedEOF), session.EventError, "对话出错：gemini stream ended without a terminal interaction: unexpected EOF"},
		{"HTTP error", &llm.APIError{StatusCode: 401, Message: "invalid key"}, session.EventError, "API Key 无效，请在设置中检查（HTTP 401：invalid key）"},
		{"protocol error", &llm.APIError{Kind: "invalid_response", Message: "invalid interaction"}, session.EventError, "对话出错：invalid interaction"},
		{"loop", fmt.Errorf("agent: %w", &agent.LoopInterrupt{Reason: "检测到重复调用循环，已中断对话"}), session.EventSystemInterrupted, "检测到重复调用循环，已中断对话"},
		{"max turns", &agent.MaxTurnsInterrupt{Reason: "已达最大轮数，对话结束"}, session.EventSystemInterrupted, "已达最大轮数，对话结束"},
		{"tool failures", &agent.ToolFailureInterrupt{Reason: "工具 read 连续系统错误 5 次，已中断对话"}, session.EventSystemInterrupted, "工具 read 连续系统错误 5 次，已中断对话"},
		{"cancelled", fmt.Errorf("request failed: %w", context.Canceled), session.EventUserStopped, ""},
		{"cancelled API error", &llm.APIError{Message: "request failed", Cause: context.Canceled}, session.EventUserStopped, ""},
		{"deadline", context.DeadlineExceeded, session.EventError, "对话出错：context deadline exceeded"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := setupLLMMessageTestApp(t)
			sess := &session.Session{SessionID: "outcome", ActiveVersion: 3}
			require.NoError(t, a.db.Create(&session.Message{SessionID: sess.SessionID, TurnID: 2, Version: 3, Role: "assistant", Content: "已有正文", ToAPI: true, ToFrontend: true}).Error)
			outcome, err := a.saveChatOutcome(sess.SessionID, 2, tc.err)
			require.NoError(t, err)
			require.Equal(t, tc.kind, outcome.EventType)
			require.Equal(t, tc.text, outcome.Content)
			require.Equal(t, "system", outcome.Role)
			require.Equal(t, "main", outcome.AgentType)
			require.False(t, outcome.ToAPI)
			require.True(t, outcome.ToFrontend)

			history, err := a.session.GetMessagesForFrontend(context.Background(), sess.SessionID)
			require.NoError(t, err)
			require.Len(t, history, 2)
			require.Equal(t, "已有正文", history[0].Content)
			require.Equal(t, outcome.ID, history[1].ID)
			require.Equal(t, outcome.EventType, history[1].EventType)
			require.Equal(t, outcome.Content, history[1].Content)
			apiMessages, err := a.session.GetMessagesForAPI(context.Background(), sess.SessionID, 3)
			require.NoError(t, err)
			require.Len(t, apiMessages, 1)
			require.Equal(t, "已有正文", apiMessages[0].Content)
		})
	}
}

func TestSaveChatOutcomeReportsWriteFailure(t *testing.T) {
	a := setupLLMMessageTestApp(t)
	failure := errors.New("outcome database unavailable")
	require.NoError(t, a.db.Callback().Create().Before("gorm:create").Register("test:reject_outcome", func(tx *gorm.DB) {
		tx.AddError(failure)
	}))
	outcome, err := a.saveChatOutcome("outcome", 1, errors.New("upstream read failed"))
	require.ErrorIs(t, err, failure)
	require.Equal(t, session.EventError, outcome.EventType)
	require.Equal(t, "对话出错：upstream read failed", outcome.Content)
	var count int64
	require.NoError(t, a.db.Model(&session.Message{}).Count(&count).Error)
	require.Zero(t, count)
}

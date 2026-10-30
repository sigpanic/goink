package app

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/sigpanic/goink/internal/session"
	"github.com/stretchr/testify/require"
)

func TestGetSessionMessagesPageRoutesCursorAndProjectsMetadata(t *testing.T) {
	a := setupLLMMessageTestApp(t)
	a.ctx = context.Background()
	for _, sessionID := range []string{"target", "other"} {
		for turn := 1; turn <= 4; turn++ {
			require.NoError(t, a.db.Create(&session.Message{
				SessionID: sessionID, TurnID: turn, Role: "assistant", ToFrontend: true,
				Content: "visible answer", ExtraMetadata: `{"llm_state":{"payload":"private"},"display_text":"keep"}`,
			}).Error)
		}
	}
	input := GetSessionMessagesPageInput{SessionID: "target", MessagePageOptions: session.MessagePageOptions{BeforeTurnID: 4, Size: 1}}
	page, err := a.GetSessionMessagesPage(input)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.Equal(t, "target", page.Items[0].SessionID)
	require.Equal(t, 3, page.Items[0].TurnID)
	require.JSONEq(t, `{"display_text":"keep"}`, page.Items[0].ExtraMetadata)
	require.True(t, page.HasMore)
	require.Equal(t, 3, page.NextBeforeTurnID)
	encoded, err := json.Marshal(input)
	require.NoError(t, err)
	require.JSONEq(t, `{"session_id":"target","before_turn_id":4,"size":1}`, string(encoded))
	legacy, err := a.GetSessionMessages("target")
	require.NoError(t, err)
	require.Len(t, legacy, 4)
	for _, message := range legacy {
		require.JSONEq(t, `{"display_text":"keep"}`, message.ExtraMetadata)
	}
}

func TestGetSessionMessagesPageHandlesEmptyStoreAndPropagatesErrors(t *testing.T) {
	a := &App{}
	page, err := a.GetSessionMessagesPage(GetSessionMessagesPageInput{SessionID: "missing"})
	require.NoError(t, err)
	require.NotNil(t, page.Items)
	require.Empty(t, page.Items)
	require.False(t, page.HasMore)

	a = setupLLMMessageTestApp(t)
	a.ctx = context.Background()
	page, err = a.GetSessionMessagesPage(GetSessionMessagesPageInput{SessionID: "missing"})
	require.NoError(t, err)
	require.NotNil(t, page.Items)
	require.Empty(t, page.Items)
	page, err = a.GetSessionMessagesPage(GetSessionMessagesPageInput{SessionID: "target", MessagePageOptions: session.MessagePageOptions{Size: -1}})
	require.ErrorContains(t, err, "app: get message page")
	require.Nil(t, page)
}

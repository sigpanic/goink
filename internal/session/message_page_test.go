package session

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newMessagePageStore(t *testing.T) *Store {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "messages.db")), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&Message{}))
	return NewStore(db, discardLogger())
}

func TestFrontendMessagePagesKeepWholeTurnsAndStableCursor(t *testing.T) {
	store := newMessagePageStore(t)
	ctx := context.Background()
	createdAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := []Message{
		{TurnID: 1, Role: "user", Content: "first"},
		{TurnID: 1, Role: "assistant", Content: "first answer"},
		{TurnID: 3, Role: "system", EventType: EventCompression},
		{TurnID: 5, Role: "user", Content: "run tools"},
		{TurnID: 5, Role: "assistant", Content: "tool declaration", ExtraMetadata: `{"tool_displays":[{"tool_id":"call_1","display_text":"读取章节"}]}`},
		{TurnID: 5, Role: "assistant", AgentType: "review", SubTaskID: "call_1", Content: "subagent answer"},
		{TurnID: 5, Role: "system", AgentType: "review", SubTaskID: "call_1", EventType: EventCompression},
		{TurnID: 5, Role: "system", EventType: EventUserStopped},
		{TurnID: 9, Role: "user", Content: "latest"},
		{TurnID: 9, Role: "assistant", Content: "latest answer"},
	}
	for i := range rows {
		rows[i].SessionID = "target"
		rows[i].ToFrontend = true
		rows[i].CreatedAt = createdAt
		rows[i].Version = i%2 + 1
		require.NoError(t, store.DB.Create(&rows[i]).Error)
	}
	for _, excluded := range []Message{
		{SessionID: "target", TurnID: 5},
		{SessionID: "target", TurnID: 100},
		{SessionID: "other", TurnID: 100, ToFrontend: true},
	} {
		excluded.Role = "assistant"
		excluded.ExtraMetadata = "{invalid excluded metadata"
		require.NoError(t, store.DB.Create(&excluded).Error)
	}

	page, err := store.GetMessagesPageForFrontend(ctx, "target", MessagePageOptions{Size: 2})
	require.NoError(t, err)
	require.Equal(t, rows[3:], page.Items, "the page includes complete tool/subagent groups across versions, even when timestamps tie")
	require.True(t, page.HasMore)
	require.Equal(t, 5, page.NextBeforeTurnID)

	newTurn := Message{SessionID: "target", TurnID: 11, Role: "user", Content: "arrived between pages", ToFrontend: true}
	require.NoError(t, store.DB.Create(&newTurn).Error)
	older, err := store.GetMessagesPageForFrontend(ctx, "target", MessagePageOptions{Size: 2, BeforeTurnID: page.NextBeforeTurnID})
	require.NoError(t, err)
	require.Equal(t, rows[:3], older.Items, "new messages cannot shift the cursor or split a compression-only turn")
	require.False(t, older.HasMore)
	require.Zero(t, older.NextBeforeTurnID)

	empty, err := store.GetMessagesPageForFrontend(ctx, "target", MessagePageOptions{Size: 2, BeforeTurnID: 1})
	require.NoError(t, err)
	require.Empty(t, empty.Items)
	require.NotNil(t, empty.Items)
	require.False(t, empty.HasMore)
	require.Zero(t, empty.NextBeforeTurnID)
}

func TestFrontendMessagePagesOrderByTurnTimeAndID(t *testing.T) {
	store := newMessagePageStore(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := []Message{
		{TurnID: 2, CreatedAt: base},
		{TurnID: 1, CreatedAt: base.Add(time.Second)},
		{TurnID: 1, CreatedAt: base},
		{TurnID: 1, CreatedAt: base},
	}
	for i := range rows {
		rows[i].SessionID, rows[i].Role, rows[i].ToFrontend = "ordered", "assistant", true
		require.NoError(t, store.DB.Create(&rows[i]).Error)
	}
	page, err := store.GetMessagesPageForFrontend(context.Background(), "ordered", MessagePageOptions{})
	require.NoError(t, err)
	require.Equal(t, []Message{rows[2], rows[3], rows[1], rows[0]}, page.Items)
}

func TestFrontendMessagePageSizesAndEmptyHistory(t *testing.T) {
	store := newMessagePageStore(t)
	ctx := context.Background()
	rows := make([]Message, 120)
	for i := range rows {
		rows[i] = Message{SessionID: "sized", TurnID: i + 1, Role: "user", ToFrontend: true}
	}
	require.NoError(t, store.DB.Create(&rows).Error)
	for _, tc := range []struct{ size, want int }{{0, 20}, {1, 1}, {1000, 100}} {
		page, err := store.GetMessagesPageForFrontend(ctx, "sized", MessagePageOptions{Size: tc.size})
		require.NoError(t, err)
		require.Len(t, page.Items, tc.want)
		require.Equal(t, 121-tc.want, page.NextBeforeTurnID)
		require.True(t, page.HasMore)
	}
	page, err := store.GetMessagesPageForFrontend(ctx, "missing", MessagePageOptions{})
	require.NoError(t, err)
	encoded, err := json.Marshal(page)
	require.NoError(t, err)
	require.JSONEq(t, `{"items":[],"next_before_turn_id":0,"has_more":false}`, string(encoded))

	legacy := Message{SessionID: "legacy", TurnID: 0, Role: "user", ToFrontend: true, CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	require.NoError(t, store.DB.Create(&legacy).Error)
	page, err = store.GetMessagesPageForFrontend(ctx, "legacy", MessagePageOptions{BeforeTurnID: 1})
	require.NoError(t, err)
	require.Equal(t, []Message{legacy}, page.Items)
	require.False(t, page.HasMore)
}

func TestFrontendMessageQueriesStripOnlyNativeStateWithoutChangingStoredHistory(t *testing.T) {
	store := newMessagePageStore(t)
	ctx := context.Background()
	metadata := []string{
		`{"tool_displays":[{"display_text":"read"}],"llm_state":{"payload":{"signature":"private"}},"unknown":{"id":9007199254740993}}`,
		`{"llm_state":{"complete":true}}`,
		`{ "display_text": "unchanged", "id": 9007199254740993 }`,
		"",
		"null",
	}
	for _, raw := range metadata {
		row := Message{SessionID: "projection", TurnID: 1, Role: "assistant", Content: "answer", ThinkingContent: "visible thinking", HasThinking: true,
			Version: 1, ToAPI: true, ToFrontend: true, ExtraMetadata: raw}
		require.NoError(t, store.DB.Create(&row).Error)
	}
	original, err := store.GetAllMessages(ctx, "projection")
	require.NoError(t, err)
	all, err := store.GetMessagesForFrontend(ctx, "projection")
	require.NoError(t, err)
	page, err := store.GetMessagesPageForFrontend(ctx, "projection", MessagePageOptions{})
	require.NoError(t, err)
	require.Equal(t, all, page.Items)
	require.JSONEq(t, `{"tool_displays":[{"display_text":"read"}],"unknown":{"id":9007199254740993}}`, page.Items[0].ExtraMetadata)
	require.Empty(t, page.Items[1].ExtraMetadata)
	for i := range metadata {
		if i >= 2 {
			require.Equal(t, metadata[i], page.Items[i].ExtraMetadata)
		}
		want := original[i]
		want.ExtraMetadata = page.Items[i].ExtraMetadata
		require.Equal(t, want, page.Items[i], "projection must preserve all other message fields")
	}
	api, err := store.GetMessagesForAPI(ctx, "projection", 1)
	require.NoError(t, err)
	require.Equal(t, original, api)
	audit, err := store.GetAllMessages(ctx, "projection")
	require.NoError(t, err)
	require.Equal(t, original, audit)
}

func TestFrontendMessagePageRejectsInvalidRequestsAndReportsQueryErrors(t *testing.T) {
	store := newMessagePageStore(t)
	ctx := context.Background()
	for _, opts := range []MessagePageOptions{{Size: -1}, {BeforeTurnID: -1}} {
		page, err := store.GetMessagesPageForFrontend(ctx, "target", opts)
		require.Error(t, err)
		require.Nil(t, page)
	}
	row := Message{SessionID: "corrupt", TurnID: 1, Role: "assistant", ToFrontend: true, ExtraMetadata: "{invalid metadata"}
	require.NoError(t, store.DB.Create(&row).Error)
	page, err := store.GetMessagesPageForFrontend(ctx, "corrupt", MessagePageOptions{})
	require.ErrorContains(t, err, "decode frontend metadata")
	require.Nil(t, page)

	require.NoError(t, store.DB.Migrator().DropTable(&Message{}))
	page, err = store.GetMessagesPageForFrontend(ctx, "target", MessagePageOptions{})
	require.ErrorContains(t, err, "select visible turns")
	require.Nil(t, page)
}

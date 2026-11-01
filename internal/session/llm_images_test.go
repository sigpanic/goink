package session

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/attachment"
	"github.com/sigpanic/goink/internal/llm/adapter"
)

func TestImageMessagesPersistAndRestore(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "images.db")), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&Message{}))
	for _, role := range []string{"user", "tool"} {
		t.Run(role, func(t *testing.T) {
			input := adapter.Message{
				Role: role,
				Images: []attachment.Ref{
					{AttachmentID: "first", Variant: attachment.Standard},
					{AttachmentID: "second", Variant: attachment.Original},
				},
			}
			if role == "tool" {
				input.Content = "读取图片成功"
				input.ToolCallID, input.ToolName = "call-read", "read"
			}
			draft := Message{
				SessionID: "images", TurnID: 1, Version: 1, ToAPI: true, ToFrontend: role == "user",
				CreatedAt:     time.Date(2026, 10, 31, 12, 0, 0, 0, time.Local),
				ExtraMetadata: `{"audit":{"keep":true},"tool_displays":[]}`,
			}
			stored, err := draft.WithLLMMessage(input)
			require.NoError(t, err)
			require.NoError(t, db.Create(&stored).Error)
			var loaded Message
			require.NoError(t, db.First(&loaded, stored.ID).Error)
			restored, err := loaded.ToLLMMessage()
			require.NoError(t, err)
			require.Equal(t, input, restored)
			var metadata map[string]json.RawMessage
			require.NoError(t, json.Unmarshal([]byte(loaded.ExtraMetadata), &metadata))
			require.JSONEq(t, `{"keep":true}`, string(metadata["audit"]))
			require.Equal(t, json.RawMessage(`[]`), metadata["tool_displays"])

			request, err := loaded.ToLLMRequestMessage()
			require.NoError(t, err)
			require.Equal(t, input.Images, request.Images)
			copy, err := (Message{Version: 2, ToAPI: true}).WithLLMMessage(request.Clone())
			require.NoError(t, err)
			again, err := copy.ToLLMRequestMessage()
			require.NoError(t, err)
			require.Equal(t, request, again)
			restored.Images[0].AttachmentID = "changed"
			again, err = loaded.ToLLMMessage()
			require.NoError(t, err)
			require.Equal(t, input, again)
		})
	}
}

func TestReplacingMessageClearsStaleImagesAndKeepsOtherMetadata(t *testing.T) {
	draft := Message{ExtraMetadata: `{"images":[{"attachment_id":"old","variant":"original"}],"audit":"keep"}`}
	stored, err := draft.WithLLMMessage(adapter.Message{Role: "user", Content: "纯文本"})
	require.NoError(t, err)
	require.JSONEq(t, `{"audit":"keep"}`, stored.ExtraMetadata)
	for _, metadata := range []string{`{"images":{}}`, `{"images":[{"variant":123}]}`} {
		_, err := (Message{ExtraMetadata: metadata}).ToLLMMessage()
		require.Error(t, err)
	}
}

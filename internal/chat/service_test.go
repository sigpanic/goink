package chat

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"path/filepath"
	"testing"

	"github.com/sigpanic/goink/internal/agentcfg"
	"github.com/sigpanic/goink/internal/attachment"
	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/llm/adapter"
	"github.com/sigpanic/goink/internal/session"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSaveTurnWithImages(t *testing.T) {
	t.Setenv("GOINK_DATA_DIR", t.TempDir())
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "chat.db")), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&session.Message{}))
	var data bytes.Buffer
	require.NoError(t, png.Encode(&data, image.NewGray(image.Rect(0, 0, 32, 32))))
	meta, err := attachment.NewStore(config.AttachmentsDirPath()).Import(7, "reference.png", &data)
	require.NoError(t, err)
	input := TurnInput{
		NovelID: 7, Model: adapter.ModelInfo{ID: "vision", SupportsVision: true},
		User:           session.Message{SessionID: "chat", TurnID: 1, Role: "user", Version: 1, ToAPI: true, ToFrontend: true, AgentType: "main"},
		Images:         []attachment.Ref{{AttachmentID: meta.ID, Variant: attachment.Standard}},
		SystemMessages: &agentcfg.SystemMessages{Identity: "identity", Catalog: "catalog"},
		SlashInject:    "skill instructions",
	}
	service := NewService(db)
	require.NoError(t, service.SaveTurn(context.Background(), input))
	var rows []session.Message
	require.NoError(t, db.Order("id").Find(&rows).Error)
	require.Len(t, rows, 4)
	require.Equal(t, "identity", rows[0].Content)
	require.Equal(t, "catalog", rows[1].Content)
	require.Equal(t, "skill instructions", rows[2].Content)
	require.False(t, rows[2].ToFrontend)
	require.True(t, rows[3].ToFrontend)
	message, err := rows[3].ToLLMMessage()
	require.NoError(t, err)
	require.Empty(t, message.Content)
	require.Equal(t, input.Images, message.Images)
	require.Empty(t, message.ImageInputs)
	require.NotContains(t, rows[3].ExtraMetadata, "base64")

	for _, tc := range []struct {
		name string
		edit func(*TurnInput)
	}{
		{"foreign-novel", func(in *TurnInput) { in.NovelID = 8 }},
		{"text-model", func(in *TurnInput) { in.Model.SupportsVision = false }},
		{"too-many", func(in *TurnInput) {
			in.Images = []attachment.Ref{input.Images[0], input.Images[0], input.Images[0], input.Images[0], input.Images[0]}
		}},
		{"empty", func(in *TurnInput) { in.Images = nil }},
		{"invalid-variant", func(in *TurnInput) { in.Images = []attachment.Ref{{AttachmentID: meta.ID, Variant: "invalid"}} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			invalid := input
			tc.edit(&invalid)
			require.Error(t, service.SaveTurn(context.Background(), invalid))
			var count int64
			require.NoError(t, db.Model(&session.Message{}).Count(&count).Error)
			require.EqualValues(t, 4, count)
		})
	}

	text := input
	text.Images, text.SystemMessages, text.SlashInject = nil, nil, ""
	text.User.Content = "继续"
	text.User.ExtraMetadata = `{"audit":"keep"}`
	require.NoError(t, service.SaveTurn(context.Background(), text))
	var savedText session.Message
	require.NoError(t, db.Last(&savedText).Error)
	require.Equal(t, "继续", savedText.Content)
	require.JSONEq(t, `{"audit":"keep"}`, savedText.ExtraMetadata)

	require.NoError(t, db.Exec(`CREATE TRIGGER reject_user BEFORE INSERT ON messages
WHEN NEW.to_frontend = 1 BEGIN SELECT RAISE(ABORT, 'write failure'); END`).Error)
	require.Error(t, service.SaveTurn(context.Background(), input))
	var count int64
	require.NoError(t, db.Model(&session.Message{}).Count(&count).Error)
	require.EqualValues(t, 5, count)
	_, _, err = attachment.NewStore(config.AttachmentsDirPath()).Read(7, input.Images[0])
	require.NoError(t, err)
}

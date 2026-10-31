package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/png"
	"testing"

	"github.com/sigpanic/goink/internal/session"
	"github.com/stretchr/testify/require"
)

func TestChatImageBindings(t *testing.T) {
	t.Setenv("GOINK_DATA_DIR", t.TempDir())
	a := &App{}
	var data bytes.Buffer
	require.NoError(t, png.Encode(&data, image.NewGray(image.Rect(0, 0, 32, 32))))
	meta, err := a.ImportChatImage(ChatImageInput{NovelID: 7, Name: "clipboard.png", Data: base64.StdEncoding.EncodeToString(data.Bytes())})
	require.NoError(t, err)
	require.Equal(t, "clipboard.png", meta.Name)
	restored, err := a.GetChatImage(7, meta.ID)
	require.NoError(t, err)
	require.Equal(t, meta, restored)
	_, err = a.GetChatImage(8, meta.ID)
	require.Error(t, err)
	_, err = a.ImportChatImage(ChatImageInput{NovelID: 7, Data: "not base64"})
	require.Error(t, err)
}

func TestChatRejectsSessionFromAnotherNovel(t *testing.T) {
	a := setupLLMMessageTestApp(t)
	require.NoError(t, a.session.DB.AutoMigrate(&session.Session{}))
	require.NoError(t, a.session.DB.Create(&session.Session{SessionID: "existing", NovelID: 7}).Error)
	_, _, err := a.loadOrCreateSession(context.Background(), ChatInput{SessionID: "existing", NovelID: 8})
	require.ErrorContains(t, err, "会话不属于当前小说")
}

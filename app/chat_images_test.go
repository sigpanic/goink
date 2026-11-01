package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/sigpanic/goink/internal/apperr"
	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/session"
	"github.com/stretchr/testify/require"
)

func TestChatImageBindings(t *testing.T) {
	t.Setenv("GOINK_DATA_DIR", t.TempDir())
	a := &App{}
	var data bytes.Buffer
	require.NoError(t, png.Encode(&data, image.NewGray(image.Rect(0, 0, 32, 32))))
	result := a.ImportChatImage(ChatImageInput{NovelID: 7, Name: "clipboard.png", Data: base64.StdEncoding.EncodeToString(data.Bytes())})
	require.Equal(t, apperr.CodeOK, result.ErrCode)
	meta := &result.Data
	require.Equal(t, "clipboard.png", meta.Name)
	restored, err := a.GetChatImage(7, meta.ID)
	require.NoError(t, err)
	require.Equal(t, meta, restored)
	_, err = a.GetChatImage(8, meta.ID)
	require.Error(t, err)
	invalid := a.ImportChatImage(ChatImageInput{NovelID: 7, Data: "not base64"})
	require.Equal(t, apperr.CodeAttachmentInvalidImage, invalid.ErrCode)
	require.Zero(t, invalid.Data)
	require.Contains(t, invalid.ErrMsg, "base64")
	require.NoError(t, a.DeleteChatImage(8, meta.ID))
	_, err = a.GetChatImage(7, meta.ID)
	require.NoError(t, err)
	require.Error(t, a.DeleteChatImage(7, "../outside"))
	require.NoError(t, a.DeleteChatImage(7, meta.ID))
	_, err = a.GetChatImage(7, meta.ID)
	require.Error(t, err)
	require.NoError(t, a.DeleteChatImage(7, meta.ID))
}

func TestChatImageImportReturnsBusinessAndUnderlyingErrors(t *testing.T) {
	t.Setenv("GOINK_DATA_DIR", t.TempDir())
	a := &App{}
	var data bytes.Buffer
	require.NoError(t, png.Encode(&data, image.NewGray(image.Rect(0, 0, 2, 2))))
	input := ChatImageInput{NovelID: 7, Name: "picture.png", Data: base64.StdEncoding.EncodeToString(data.Bytes())}
	invalid := input
	invalid.NovelID = 0
	result := a.ImportChatImage(invalid)
	require.Equal(t, apperr.CodeInvalid, result.ErrCode)
	require.Zero(t, result.Data)
	invalid = input
	invalid.Data = base64.StdEncoding.EncodeToString([]byte("corrupt image"))
	result = a.ImportChatImage(invalid)
	require.Equal(t, apperr.CodeAttachmentInvalidImage, result.ErrCode)
	require.Zero(t, result.Data)
	require.Contains(t, result.ErrMsg, "decode image header")

	require.NoError(t, os.MkdirAll(config.AttachmentsDirPath(), 0700))
	blocked := filepath.Join(config.AttachmentsDirPath(), "7")
	require.NoError(t, os.WriteFile(blocked, []byte("not a directory"), 0600))
	result = a.ImportChatImage(input)
	require.Equal(t, apperr.CodeInternal, result.ErrCode)
	require.Zero(t, result.Data)
	require.Contains(t, result.ErrMsg, "create novel directory")
	require.Contains(t, result.ErrMsg, blocked)
}

func TestChatRejectsSessionFromAnotherNovel(t *testing.T) {
	a := setupLLMMessageTestApp(t)
	require.NoError(t, a.session.DB.AutoMigrate(&session.Session{}))
	require.NoError(t, a.session.DB.Create(&session.Session{SessionID: "existing", NovelID: 7}).Error)
	_, _, err := a.loadOrCreateSession(context.Background(), ChatInput{SessionID: "existing", NovelID: 8})
	require.ErrorContains(t, err, "会话不属于当前小说")
}

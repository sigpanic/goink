package app

import (
	"encoding/base64"
	"errors"
	"strings"

	"github.com/sigpanic/goink/internal/apperr"
	"github.com/sigpanic/goink/internal/attachment"
	"github.com/sigpanic/goink/internal/config"
)

type ChatImageInput struct {
	NovelID int64  `json:"novel_id"`
	Name    string `json:"name"`
	Data    string `json:"data"`
}

func (a *App) ImportChatImage(input ChatImageInput) *apperr.Result[attachment.Image] {
	image, err := attachment.NewStore(config.AttachmentsDirPath()).Import(
		input.NovelID, input.Name, base64.NewDecoder(base64.StdEncoding, strings.NewReader(input.Data)),
	)
	if err != nil {
		var corrupt base64.CorruptInputError
		if errors.As(err, &corrupt) {
			err = &apperr.BusinessError{CodeVal: attachment.CodeInvalidImage, Msg: "attachment: 图片编码无效", Cause: err}
		}
		return apperr.Err[attachment.Image](err)
	}
	return apperr.Ok(image)
}

func (a *App) GetChatImage(novelID int64, id string) (*attachment.Image, error) {
	image, err := attachment.NewStore(config.AttachmentsDirPath()).Get(novelID, id)
	if err != nil {
		return nil, err
	}
	return &image, nil
}

func (a *App) DeleteChatImage(novelID int64, id string) error {
	return attachment.NewStore(config.AttachmentsDirPath()).Delete(novelID, id)
}

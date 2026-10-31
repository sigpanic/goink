package app

import (
	"encoding/base64"
	"strings"

	"github.com/sigpanic/goink/internal/attachment"
	"github.com/sigpanic/goink/internal/config"
)

type ChatImageInput struct {
	NovelID int64  `json:"novel_id"`
	Name    string `json:"name"`
	Data    string `json:"data"`
}

func (a *App) ImportChatImage(input ChatImageInput) (*attachment.Image, error) {
	image, err := attachment.NewStore(config.AttachmentsDirPath()).Import(
		input.NovelID, input.Name, base64.NewDecoder(base64.StdEncoding, strings.NewReader(input.Data)),
	)
	if err != nil {
		return nil, err
	}
	return &image, nil
}

func (a *App) GetChatImage(novelID int64, id string) (*attachment.Image, error) {
	image, err := attachment.NewStore(config.AttachmentsDirPath()).Get(novelID, id)
	if err != nil {
		return nil, err
	}
	return &image, nil
}

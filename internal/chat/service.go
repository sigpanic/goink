package chat

import (
	"context"
	"fmt"
	"strings"

	"github.com/sigpanic/goink/internal/agentcfg"
	"github.com/sigpanic/goink/internal/attachment"
	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/llm/adapter"
	"github.com/sigpanic/goink/internal/session"
	"gorm.io/gorm"
)

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service { return &Service{db: db} }

type TurnInput struct {
	NovelID        int64
	Model          adapter.ModelInfo
	User           session.Message
	Images         []attachment.Ref
	SystemMessages *agentcfg.SystemMessages
	SlashInject    string
}

func (s *Service) SaveTurn(ctx context.Context, input TurnInput) error {
	if strings.TrimSpace(input.User.Content) == "" && len(input.Images) == 0 {
		return fmt.Errorf("消息不能为空")
	}
	if len(input.Images) > 4 {
		return fmt.Errorf("每条消息最多添加 4 张图片")
	}
	if len(input.Images) > 0 && !input.Model.SupportsVision {
		return fmt.Errorf("模型 %s 不支持图片输入，请切换支持视觉的模型", input.Model.ID)
	}
	store := attachment.NewStore(config.AttachmentsDirPath())
	for _, ref := range input.Images {
		if _, _, err := store.Read(input.NovelID, ref); err != nil {
			return fmt.Errorf("读取图片 %s: %w", ref.AttachmentID, err)
		}
	}
	user, err := input.User.WithLLMMessage(adapter.Message{
		Role: "user", Content: input.User.Content, Images: input.Images,
	})
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if input.SystemMessages != nil {
			if err := writeSystemMessages(tx, user.SessionID, user.TurnID, input.SystemMessages); err != nil {
				return err
			}
		}
		if input.SlashInject != "" {
			if err := tx.Create(&session.Message{
				SessionID: user.SessionID, TurnID: user.TurnID, Role: "user",
				Content: input.SlashInject, Version: user.Version, ToAPI: true, ToFrontend: false, AgentType: "main",
			}).Error; err != nil {
				return err
			}
		}
		return tx.Create(&user).Error
	})
}

// writeSystemMessages 在新 session 的事务内写入预构建的 system 消息到 messages 表。
// sysMsgs 应在事务外通过 BuildSystemMessages 构建完成（compress 模式，避免死锁）。
func writeSystemMessages(tx *gorm.DB, sessionID string, turnID int, msgs *agentcfg.SystemMessages) error {
	sysMsg := func(content string) *session.Message {
		return &session.Message{
			SessionID: sessionID, TurnID: turnID, Role: "system", Content: content,
			Version: 1, ToAPI: true, ToFrontend: false, AgentType: "main",
		}
	}

	for _, c := range []string{msgs.Identity, msgs.Always, msgs.Catalog, msgs.Profile, msgs.State} {
		if c != "" {
			if err := tx.Create(sysMsg(c)).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

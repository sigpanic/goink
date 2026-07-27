package setting

import (
	"context"
	"fmt"
	"log/slog"

	"gorm.io/gorm"
)

// Store 管理 SettingItem 持久化。
type Store struct {
	DB     *gorm.DB
	logger *slog.Logger
}

// NewStore 创建 setting 存储。
func NewStore(db *gorm.DB, logger *slog.Logger) *Store {
	return &Store{DB: db, logger: logger}
}

// ListSettings 返回该小说的全部设定，前端展示用。
// 注：注入 LLM 走 agentcfg.NovelProfile 直接查 db（updated_at DESC，截断保留最近活跃的），
// 不经此方法；此方法只服务前端，排序 created_at ASC（最早在前，跟 preference store 一致）。
func (s *Store) ListSettings(ctx context.Context, novelID int64) ([]SettingItem, error) {
	var items []SettingItem
	if err := s.DB.WithContext(ctx).
		Where("novel_id = ?", novelID).
		Order("created_at ASC").
		Find(&items).Error; err != nil {
		return nil, fmt.Errorf("setting store: list settings: %w", err)
	}
	return items, nil
}

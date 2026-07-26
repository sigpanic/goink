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

// ListSettings 返回该小说的全部设定。
// 唯一查询入口：注入 NovelState 和前端展示都用此方法（v2 砍 is_global 后两者 SQL 等价，
// 不再需要 ListNovelSettings 这种"前端专用"副本，减少冗余 API surface）。
// 排序 created_at DESC：最新在前。
// 注：注入用和前端展示用的排序口径一致——下一轮若改 updated_at DESC 也是一起改。
func (s *Store) ListSettings(ctx context.Context, novelID int64) ([]SettingItem, error) {
	var items []SettingItem
	if err := s.DB.WithContext(ctx).
		Where("novel_id = ?", novelID).
		Order("created_at DESC").
		Find(&items).Error; err != nil {
		return nil, fmt.Errorf("setting store: list settings: %w", err)
	}
	return items, nil
}

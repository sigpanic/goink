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

// ListSettings 返回该小说的专属设定 + 全部全局设定。
// 排序 is_global DESC, created_at DESC：全局在前、组内最新在前，用于 NovelProfile 注入（截断保留最新）。
func (s *Store) ListSettings(ctx context.Context, novelID int64) ([]SettingItem, error) {
	var items []SettingItem
	if err := s.DB.WithContext(ctx).
		Where("is_global = ? OR novel_id = ?", true, novelID).
		Order("is_global DESC, created_at DESC").
		Find(&items).Error; err != nil {
		return nil, fmt.Errorf("setting store: list settings: %w", err)
	}
	return items, nil
}

// ListNovelSettings 只返回某小说的专属设定（不含全局），前端编辑用。
func (s *Store) ListNovelSettings(ctx context.Context, novelID int64) ([]SettingItem, error) {
	var items []SettingItem
	if err := s.DB.WithContext(ctx).
		Where("is_global = ? AND novel_id = ?", false, novelID).
		Order("created_at ASC").
		Find(&items).Error; err != nil {
		return nil, fmt.Errorf("setting store: list novel settings: %w", err)
	}
	return items, nil
}

// ListGlobalSettings 只返回全局设定（所有小说共享）。
func (s *Store) ListGlobalSettings(ctx context.Context) ([]SettingItem, error) {
	var items []SettingItem
	if err := s.DB.WithContext(ctx).
		Where("is_global = ?", true).
		Order("created_at ASC").
		Find(&items).Error; err != nil {
		return nil, fmt.Errorf("setting store: list global settings: %w", err)
	}
	return items, nil
}

package preference

import (
	"context"
	"fmt"
	"log/slog"

	"gorm.io/gorm"
)

// Store 管理 Novel 持久化。DB 导出供调用方做简单 CRUD。
type Store struct {
	DB     *gorm.DB
	logger *slog.Logger
}

// NewStore 创建 novel 存储。
func NewStore(db *gorm.DB, logger *slog.Logger) *Store {
	return &Store{DB: db, logger: logger}
}

// ── PreferenceItem ────────────────────────────────────

// ListPreferences 返回该小说的专属偏好 + 全部全局偏好。
func (s *Store) ListPreferences(ctx context.Context, novelID int64) ([]PreferenceItem, error) {
	var items []PreferenceItem
	if err := s.DB.WithContext(ctx).
		Where("is_global = ? OR novel_id = ?", true, novelID).
		Order("is_global DESC, created_at ASC").
		Find(&items).Error; err != nil {
		return nil, fmt.Errorf("novel store: list preferences: %w", err)
	}
	return items, nil
}

// ListNovelPreferences 只返回某小说的专属偏好（不含全局），前端编辑用。
func (s *Store) ListNovelPreferences(ctx context.Context, novelID int64) ([]PreferenceItem, error) {
	var items []PreferenceItem
	if err := s.DB.WithContext(ctx).
		Where("is_global = ? AND novel_id = ?", false, novelID).
		Order("created_at ASC").
		Find(&items).Error; err != nil {
		return nil, fmt.Errorf("novel store: list novel preferences: %w", err)
	}
	return items, nil
}

// ListGlobalPreferences 只返回全局偏好（所有小说共享）。
func (s *Store) ListGlobalPreferences(ctx context.Context) ([]PreferenceItem, error) {
	var items []PreferenceItem
	if err := s.DB.WithContext(ctx).
		Where("is_global = ?", true).
		Order("created_at ASC").
		Find(&items).Error; err != nil {
		return nil, fmt.Errorf("novel store: list global preferences: %w", err)
	}
	return items, nil
}

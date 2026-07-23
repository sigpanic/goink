package novel

import (
	"context"
	"fmt"
	"log/slog"

	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/storage"
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

// ListNovelsOptions 是 List 方法的可选过滤条件。零值表示不过滤。
type ListNovelsOptions struct {
	PageParams storage.PageParams
	Genre      string // 空字符串=不过滤
	Search     string // 空字符串=不过滤，按 title LIKE 匹配
}

// List 分页列出小说，支持 genre 过滤和 title 搜索。
func (s *Store) List(ctx context.Context, opts ListNovelsOptions) (*storage.PageResult[Novel], error) {
	pp := opts.PageParams
	pp.Normalize()

	q := s.DB.WithContext(ctx).Model(&Novel{})

	if opts.Genre != "" {
		q = q.Where("genre = ?", opts.Genre)
	}
	if opts.Search != "" {
		q = q.Where("title LIKE ?", "%"+opts.Search+"%")
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("novel store: count: %w", err)
	}

	var novels []Novel
	offset := (pp.Page - 1) * pp.Size
	if err := q.Order("updated_at DESC").Offset(offset).Limit(pp.Size).Find(&novels).Error; err != nil {
		return nil, fmt.Errorf("novel store: list: %w", err)
	}

	s.logger.Debug("novel store: listed", "total", total, "page", pp.Page)
	return storage.NewPageResult(novels, total, pp.Page, pp.Size), nil
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

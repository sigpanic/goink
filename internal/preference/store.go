package preference

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

// ── PreferenceItem ────────────────────────────────────

// ListOptions 是 ListGlobalPreferences / ListNovelPreferences 的可选参数。
type ListOptions struct {
	PageParams storage.PageParams
	Search     string // 空字符串=不过滤，按 content LIKE OR category LIKE 模糊匹配
	Order      string // 空字符串=默认 created_at ASC（重构前硬编码值，Order 保留约束）
}

// ListNovelPreferences 只返回某小说的专属偏好（不含全局），前端编辑用。
// 支持搜索（content/category LIKE）和分页，前端全量拉取（Size=-1）和全局搜索复用。
func (s *Store) ListNovelPreferences(ctx context.Context, novelID int64, opts ListOptions) (*storage.PageResult[PreferenceItem], error) {
	pp := opts.PageParams
	pp.Normalize()

	q := s.DB.WithContext(ctx).
		Model(&PreferenceItem{}).
		Where("is_global = ? AND novel_id = ?", false, novelID)

	if opts.Search != "" {
		q = q.Where("content LIKE ? OR category LIKE ?", "%"+opts.Search+"%", "%"+opts.Search+"%")
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("novel store: count novel preferences: %w", err)
	}

	order := opts.Order
	if order == "" {
		order = "created_at ASC"
	}
	var items []PreferenceItem
	if err := q.Order(order).Offset(pp.Offset()).Limit(pp.Size).Find(&items).Error; err != nil {
		return nil, fmt.Errorf("novel store: list novel preferences: %w", err)
	}

	s.logger.Debug("preference store: listed novel", "novel_id", novelID, "total", total, "page", pp.Page)
	return storage.NewPageResult(items, total, pp.Page, pp.Size), nil
}

// ListGlobalPreferences 只返回全局偏好（所有小说共享）。
// 支持搜索（content/category LIKE）和分页，前端全量拉取（Size=-1）和全局搜索复用。
func (s *Store) ListGlobalPreferences(ctx context.Context, opts ListOptions) (*storage.PageResult[PreferenceItem], error) {
	pp := opts.PageParams
	pp.Normalize()

	q := s.DB.WithContext(ctx).
		Model(&PreferenceItem{}).
		Where("is_global = ?", true)

	if opts.Search != "" {
		q = q.Where("content LIKE ? OR category LIKE ?", "%"+opts.Search+"%", "%"+opts.Search+"%")
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("novel store: count global preferences: %w", err)
	}

	order := opts.Order
	if order == "" {
		order = "created_at ASC"
	}
	var items []PreferenceItem
	if err := q.Order(order).Offset(pp.Offset()).Limit(pp.Size).Find(&items).Error; err != nil {
		return nil, fmt.Errorf("novel store: list global preferences: %w", err)
	}

	s.logger.Debug("preference store: listed global", "total", total, "page", pp.Page)
	return storage.NewPageResult(items, total, pp.Page, pp.Size), nil
}

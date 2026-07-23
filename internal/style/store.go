package style

import (
	"context"
	"fmt"
	"log/slog"

	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/storage"
)

// Store 管理 Sample 持久化。
type Store struct {
	DB     *gorm.DB
	logger *slog.Logger
}

// NewStore 创建 style 存储。
func NewStore(db *gorm.DB, logger *slog.Logger) *Store {
	return &Store{DB: db, logger: logger}
}

// ListOptions 是 List 方法的可选过滤条件。零值表示不过滤。
type ListOptions struct {
	PageParams storage.PageParams
	NovelID    int64  // 0=仅全局，N=全局+该小说专属
	Search     string // 空字符串=不过滤，按 name LIKE 匹配
}

// List 分页列出风格素材。
// 当 NovelID > 0 时，返回全局素材 + 该小说专属素材（is_global = true OR novel_id = ?）；
// 当 NovelID == 0 时，仅返回全局素材（is_global = true）。
// List 只查摘要字段，不读大 content。
func (s *Store) List(ctx context.Context, opts ListOptions) (*storage.PageResult[Sample], error) {
	pp := opts.PageParams
	pp.Normalize()

	q := s.DB.WithContext(ctx).Model(&Sample{}).
		Select("id, novel_id, is_global, name, preview, tags, word_count, created_at, updated_at")

	if opts.NovelID > 0 {
		q = q.Where("is_global = ? OR novel_id = ?", true, opts.NovelID)
	} else {
		q = q.Where("is_global = ?", true)
	}

	if opts.Search != "" {
		q = q.Where("name LIKE ?", "%"+opts.Search+"%")
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("style store: count: %w", err)
	}

	var samples []Sample
	offset := (pp.Page - 1) * pp.Size
	if err := q.Order("created_at ASC").Offset(offset).Limit(pp.Size).Find(&samples).Error; err != nil {
		return nil, fmt.Errorf("style store: list: %w", err)
	}

	return storage.NewPageResult(samples, total, pp.Page, pp.Size), nil
}

// GetByIDs 批量按 ID 获取素材，用于 ComputeStats 和 ExtractStyle。
func (s *Store) GetByIDs(ctx context.Context, ids []int64) ([]Sample, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var samples []Sample
	if err := s.DB.WithContext(ctx).Where("id IN ?", ids).Find(&samples).Error; err != nil {
		return nil, fmt.Errorf("style store: get by ids: %w", err)
	}
	return samples, nil
}

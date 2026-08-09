package reader

import (
	"context"
	"fmt"
	"log/slog"

	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/storage"
)

// Store 管理 ReaderPerspective 持久化。DB 导出供调用方做简单 CRUD。
type Store struct {
	DB     *gorm.DB
	logger *slog.Logger
}

// NewStore 创建 reader 存储。
func NewStore(db *gorm.DB, logger *slog.Logger) *Store {
	return &Store{DB: db, logger: logger}
}

// ListByNovelOptions 是 ListByNovel 的可选参数。
type ListByNovelOptions struct {
	PageParams storage.PageParams
	Search     string // 空字符串=不过滤，按 content LIKE OR related_truth LIKE 模糊匹配
	Type       string // 空字符串=不过滤，"known"/"suspense"/"misconception"
	Order      string // 空字符串=默认 type, planted_chapter ASC
}

// ListByNovel 分页列出某小说的读者认知条目，支持搜索和按类型过滤。
// 前端全量拉取（Size=-1）和全局搜索复用。
func (s *Store) ListByNovel(ctx context.Context, novelID int64, opts ListByNovelOptions) (*storage.PageResult[ReaderPerspective], error) {
	pp := opts.PageParams
	pp.Normalize()

	q := s.DB.WithContext(ctx).Model(&ReaderPerspective{}).Where("novel_id = ?", novelID)

	if opts.Search != "" {
		q = q.Where("content LIKE ? OR related_truth LIKE ?", "%"+opts.Search+"%", "%"+opts.Search+"%")
	}
	if opts.Type != "" {
		q = q.Where("type = ?", opts.Type)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("reader store: count: %w", err)
	}

	order := opts.Order
	if order == "" {
		order = "type, planted_chapter ASC"
	}
	var items []ReaderPerspective
	if err := q.Order(order).Offset(pp.Offset()).Limit(pp.Size).Find(&items).Error; err != nil {
		return nil, fmt.Errorf("reader store: list: %w", err)
	}

	s.logger.Debug("reader store: listed", "novel_id", novelID, "total", total, "page", pp.Page)
	return storage.NewPageResult(items, total, pp.Page, pp.Size), nil
}

// ListActive 返回全部未回收（revealed_chapter=0）的读者认知条目。
// 全量返回——活跃集合天然小，context builder 和 MCP 工具需要完整快照。
func (s *Store) ListActive(ctx context.Context, novelID int64) ([]ReaderPerspective, error) {
	var items []ReaderPerspective
	if err := s.DB.WithContext(ctx).
		Where("novel_id = ? AND revealed_chapter = 0", novelID).
		Order("type, planted_chapter ASC").
		Find(&items).Error; err != nil {
		return nil, fmt.Errorf("reader store: list active: %w", err)
	}
	return items, nil
}

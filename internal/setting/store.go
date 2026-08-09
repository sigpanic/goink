package setting

import (
	"context"
	"fmt"
	"log/slog"

	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/storage"
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

// ── SettingItem ────────────────────────────────────

// ListOptions 是 ListByNovel 的可选参数。
type ListOptions struct {
	PageParams storage.PageParams
	Search     string // 空字符串=不过滤，按 content LIKE OR category LIKE 模糊匹配
	Order      string // 空字符串=默认 created_at ASC（重构前硬编码值，Order 保留约束）
}

// ListByNovel 返回该小说的全部设定，前端展示用。
// 支持搜索（content/category LIKE）和分页，前端全量拉取（Size=-1）和全局搜索复用。
//
// 注：注入 LLM 走 agentcfg.NovelProfile 直接查 db（updated_at DESC，截断保留最近活跃的），
// 不经此方法；此方法只服务前端 + 全局搜索，排序 created_at ASC（最早在前，跟 preference store 一致）。
func (s *Store) ListByNovel(ctx context.Context, novelID int64, opts ListOptions) (*storage.PageResult[SettingItem], error) {
	pp := opts.PageParams
	pp.Normalize()

	q := s.DB.WithContext(ctx).
		Model(&SettingItem{}).
		Where("novel_id = ?", novelID)

	if opts.Search != "" {
		q = q.Where("content LIKE ? OR category LIKE ?", "%"+opts.Search+"%", "%"+opts.Search+"%")
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("setting store: count settings: %w", err)
	}

	order := opts.Order
	if order == "" {
		order = "created_at ASC"
	}
	var items []SettingItem
	if err := q.Order(order).Offset(pp.Offset()).Limit(pp.Size).Find(&items).Error; err != nil {
		return nil, fmt.Errorf("setting store: list settings: %w", err)
	}

	s.logger.Debug("setting store: listed", "novel_id", novelID, "total", total, "page", pp.Page)
	return storage.NewPageResult(items, total, pp.Page, pp.Size), nil
}

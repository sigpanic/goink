package timeline

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/git"
	"github.com/sigpanic/goink/internal/storage"
)

// Store 管理 ChapterPlan 和 TimelineEntry 持久化。DB 导出供调用方做简单 CRUD。
type Store struct {
	DB     *gorm.DB
	logger *slog.Logger
}

// NewStore 创建 timeline 存储。
func NewStore(db *gorm.DB, logger *slog.Logger) *Store {
	return &Store{DB: db, logger: logger}
}

// ── ChapterPlan ──────────────────────────────────────

// GetPlans 返回某小说的全部章节计划（next/near/far 三个槽位）。
// 从 plans/{scope}.md 文件读取，文件不存在时 content 为空字符串。
func (s *Store) GetPlans(ctx context.Context, novelID int64) ([]ChapterPlan, error) {
	scopes := []string{"next", "near", "far"}
	plans := make([]ChapterPlan, 0, 3)
	for _, scope := range scopes {
		content, err := git.ReadFile(novelID, git.PlanPath(scope))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("timeline store: read plan %s: %w", scope, err)
		}
		plans = append(plans, ChapterPlan{
			NovelID: novelID,
			Scope:   scope,
			Content: content,
		})
	}
	return plans, nil
}

// SavePlan 将章节计划写入 plans/{scope}.md 文件，全量替换。
func (s *Store) SavePlan(ctx context.Context, plan *ChapterPlan) error {
	return git.WriteFile(plan.NovelID, git.PlanPath(plan.Scope), plan.Content)
}

// ── TimelineEntry ────────────────────────────────────

// ListByNovelOptions 是 ListByNovel 的可选参数。
type ListByNovelOptions struct {
	PageParams storage.PageParams
	Search     string // 空字符串=不过滤，按 title LIKE OR content LIKE 模糊匹配
	Category   string // 空字符串=不过滤，"foreshadowing"/"user_directive"
	Status     string // 空字符串=不过滤，"pending"/"resolved"/"abandoned"
	Order      string // 空字符串=默认 target_chapter ASC, importance DESC
}

// ListByNovel 分页列出某小说的伏笔/用户指令，支持搜索、分类和状态过滤。
// 前端全量拉取（Size=-1）和全局搜索复用。废弃 ListByChapterRange（前端传 0,0 全量等价）
// 和 SearchByNovel（搜索改走 ListByNovel(Search)）。
func (s *Store) ListByNovel(ctx context.Context, novelID int64, opts ListByNovelOptions) (*storage.PageResult[TimelineEntry], error) {
	pp := opts.PageParams
	pp.Normalize()

	q := s.DB.WithContext(ctx).Model(&TimelineEntry{}).Where("novel_id = ?", novelID)

	if opts.Search != "" {
		q = q.Where("title LIKE ? OR content LIKE ?", "%"+opts.Search+"%", "%"+opts.Search+"%")
	}
	if opts.Category != "" {
		q = q.Where("category = ?", opts.Category)
	}
	if opts.Status != "" {
		q = q.Where("status = ?", opts.Status)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("timeline store: count: %w", err)
	}

	order := opts.Order
	if order == "" {
		order = "target_chapter ASC, importance DESC"
	}
	var entries []TimelineEntry
	if err := q.Order(order).Offset(pp.Offset()).Limit(pp.Size).Find(&entries).Error; err != nil {
		return nil, fmt.Errorf("timeline store: list: %w", err)
	}

	s.logger.Debug("timeline store: listed", "novel_id", novelID, "total", total, "page", pp.Page)
	return storage.NewPageResult(entries, total, pp.Page, pp.Size), nil
}

// ListBefore 取 target_chapter < beforeChapter 的最近 limit 条，不论状态。
func (s *Store) ListBefore(ctx context.Context, novelID int64, chapterNum int, limit int) ([]TimelineEntry, error) {
	var entries []TimelineEntry
	if err := s.DB.WithContext(ctx).
		Where("novel_id = ? AND target_chapter < ?", novelID, chapterNum).
		Order("target_chapter DESC").
		Limit(limit).
		Find(&entries).Error; err != nil {
		return nil, fmt.Errorf("timeline store: list before: %w", err)
	}
	return entries, nil
}

// ListPendingBefore 取 target_chapter < chapterNum 且 pending 的全部条目，兜底截断 100。
// 按 target_chapter DESC——最近的在前，远古的在后，先展示最相关的。
func (s *Store) ListPendingBefore(ctx context.Context, novelID int64, chapterNum int) ([]TimelineEntry, error) {
	var entries []TimelineEntry
	if err := s.DB.WithContext(ctx).
		Where("novel_id = ? AND target_chapter < ? AND status = ?", novelID, chapterNum, "pending").
		Order("target_chapter DESC").
		Limit(100).
		Find(&entries).Error; err != nil {
		return nil, fmt.Errorf("timeline store: list pending before: %w", err)
	}
	return entries, nil
}

// ListAfter 取 target_chapter >= fromChapter 的全部条目，不论状态，兜底截断 100。
func (s *Store) ListAfter(ctx context.Context, novelID int64, chapterNum int) ([]TimelineEntry, error) {
	var entries []TimelineEntry
	if err := s.DB.WithContext(ctx).
		Where("novel_id = ? AND target_chapter >= ?", novelID, chapterNum).
		Order("target_chapter ASC").
		Limit(100).
		Find(&entries).Error; err != nil {
		return nil, fmt.Errorf("timeline store: list after: %w", err)
	}
	return entries, nil
}

//具体来说 构造上下文的时候拿到前10条历史+未来100条，以及前边的所有pending的（状态异常了，也可以不给，等review的时候再传递），未来如果有显示已经完成的，也算作状态异常，状态异常的
//需要提醒llm进行修正，确保之前的全部结束，之后的全部pending，targetchapter是用来作为一个大概的锚点的，llm根据章节进度，实时维护，后续提供reviewagent一个专属
//工具。专门用来查询各种异常状态的，用以提醒

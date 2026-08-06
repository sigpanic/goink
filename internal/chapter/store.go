package chapter

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/git"
	"github.com/sigpanic/goink/internal/storage"
)

// Store 管理 Chapter 持久化。DB 导出供调用方做简单 CRUD。
type Store struct {
	DB     *gorm.DB
	logger *slog.Logger
}

// NewStore 创建 chapter 存储。
func NewStore(db *gorm.DB, logger *slog.Logger) *Store {
	return &Store{DB: db, logger: logger}
}

// ListByNovelOptions 是 ListByNovel 的可选参数。零值即可直接使用（默认升序）。
type ListByNovelOptions struct {
	PageParams storage.PageParams
	Order      string // "asc"(默认) 或 "desc"，按 chapter_number 排序
}

// ListByNovel 分页列出某小说的章节。
func (s *Store) ListByNovel(ctx context.Context, novelID int64, opts ListByNovelOptions) (*storage.PageResult[Chapter], error) {
	pp := opts.PageParams
	pp.Normalize()

	order := "chapter_number ASC"
	if strings.ToLower(opts.Order) == "desc" {
		order = "chapter_number DESC"
	}

	q := s.DB.WithContext(ctx).Model(&Chapter{}).Where("novel_id = ?", novelID)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("chapter store: count: %w", err)
	}

	var chapters []Chapter
	if err := q.Order(order).Offset(pp.Offset()).Limit(pp.Size).Find(&chapters).Error; err != nil {
		return nil, fmt.Errorf("chapter store: list: %w", err)
	}

	for i := range chapters {
		chapters[i].FilePath = git.ChapterPath(chapters[i].ChapterNumber)
	}

	s.logger.Debug("chapter store: listed", "novel_id", novelID, "total", total, "page", pp.Page)
	return storage.NewPageResult(chapters, total, pp.Page, pp.Size), nil
}

// ListAllByNovel 返回某小说的全部章节（不分页），按 chapter_number 升序。
func (s *Store) ListAllByNovel(ctx context.Context, novelID int64) ([]Chapter, error) {
	var chapters []Chapter
	if err := s.DB.WithContext(ctx).
		Where("novel_id = ?", novelID).
		Order("chapter_number ASC").
		Find(&chapters).Error; err != nil {
		return nil, fmt.Errorf("chapter store: list all: %w", err)
	}
	for i := range chapters {
		chapters[i].FilePath = git.ChapterPath(chapters[i].ChapterNumber)
	}
	return chapters, nil
}

// GetByNovelAndNumber 按 novel_id + chapter_number 取单章。
func (s *Store) GetByNovelAndNumber(ctx context.Context, novelID int64, chapterNumber int) (*Chapter, error) {
	var ch Chapter
	if err := s.DB.WithContext(ctx).
		Where("novel_id = ? AND chapter_number = ?", novelID, chapterNumber).
		First(&ch).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("chapter store: get by novel+number: %w", err)
		}
		return nil, fmt.Errorf("chapter store: get by novel+number: %w", err)
	}
	ch.FilePath = git.ChapterPath(ch.ChapterNumber)
	return &ch, nil
}

// GetLatestNumber 返回该小说当前最大的章节编号，无章节时返回 0。
func (s *Store) GetLatestNumber(ctx context.Context, novelID int64) (int, error) {
	var maxNum int
	if err := s.DB.WithContext(ctx).
		Model(&Chapter{}).
		Where("novel_id = ?", novelID).
		Select("COALESCE(MAX(chapter_number), 0)").
		Scan(&maxNum).Error; err != nil {
		return 0, fmt.Errorf("chapter store: latest number: %w", err)
	}
	return maxNum, nil
}

// SearchByNovel 按关键词搜索某小说的章节，匹配标题和摘要。
func (s *Store) SearchByNovel(ctx context.Context, novelID int64, query string, limit int) ([]Chapter, error) {
	var chapters []Chapter
	if err := s.DB.WithContext(ctx).
		Where("novel_id = ? AND (title LIKE ? OR summary LIKE ?)", novelID, "%"+query+"%", "%"+query+"%").
		Order("chapter_number ASC").
		Limit(limit).
		Find(&chapters).Error; err != nil {
		return nil, fmt.Errorf("chapter store: search: %w", err)
	}
	for i := range chapters {
		chapters[i].FilePath = git.ChapterPath(chapters[i].ChapterNumber)
	}
	return chapters, nil
}

// GetRecent 取最近 N 章，按 chapter_number 降序。
func (s *Store) GetRecent(ctx context.Context, novelID int64, limit int) ([]Chapter, error) {
	var chapters []Chapter
	if err := s.DB.WithContext(ctx).
		Where("novel_id = ?", novelID).
		Order("chapter_number DESC").
		Limit(limit).
		Find(&chapters).Error; err != nil {
		return nil, fmt.Errorf("chapter store: recent: %w", err)
	}
	for i := range chapters {
		chapters[i].FilePath = git.ChapterPath(chapters[i].ChapterNumber)
	}
	return chapters, nil
}

// UpdateTitle 更新章节标题。
func (s *Store) UpdateTitle(ctx context.Context, novelID int64, chapterNumber int, title string) error {
	return s.DB.WithContext(ctx).
		Model(&Chapter{}).
		Where("novel_id = ? AND chapter_number = ?", novelID, chapterNumber).
		Update("title", title).Error
}

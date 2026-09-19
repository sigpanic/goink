package chapter

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/git"
	"github.com/sigpanic/goink/internal/storage"
	"github.com/sigpanic/goink/internal/volume"
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
	Order      string // "asc"(默认) 或 "desc"，按阅读顺序排序
}

const (
	// chapterOrderAsc 按卷的可重排顺序和卷内顺序排序，未分卷章节置后。
	chapterOrderAsc = "chapters.volume_id IS NULL ASC, volumes.sort_order ASC, chapters.sort_order ASC, chapters.id ASC"
	// chapterOrderDesc 是阅读顺序的倒序，用于 "desc" 与 GetRecent。
	chapterOrderDesc = "chapters.volume_id IS NULL DESC, volumes.sort_order DESC, chapters.sort_order DESC, chapters.id DESC"
)

// orderedByNovel 返回带卷排序信息的章节查询。必须按 volumes.sort_order 排序，
// 不能只按 chapters.volume_id，否则 ReorderVolumes 不会改变章节的阅读顺序。
func (s *Store) orderedByNovel(ctx context.Context, novelID int64) *gorm.DB {
	return s.DB.WithContext(ctx).
		Model(&Chapter{}).
		Joins("LEFT JOIN volumes ON volumes.id = chapters.volume_id").
		Where("chapters.novel_id = ?", novelID)
}

// ListByNovel 分页列出某小说的章节。
func (s *Store) ListByNovel(ctx context.Context, novelID int64, opts ListByNovelOptions) (*storage.PageResult[Chapter], error) {
	pp := opts.PageParams
	pp.Normalize()

	order := chapterOrderAsc
	if strings.ToLower(opts.Order) == "desc" {
		order = chapterOrderDesc
	}

	q := s.orderedByNovel(ctx, novelID)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("chapter store: count: %w", err)
	}

	var chapters []Chapter
	if err := q.Order(order).Offset(pp.Offset()).Limit(pp.Size).Find(&chapters).Error; err != nil {
		return nil, fmt.Errorf("chapter store: list: %w", err)
	}

	for i := range chapters {
		chapters[i].FilePath = git.ChapterPath(chapters[i].ID)
	}

	s.logger.Debug("chapter store: listed", "novel_id", novelID, "total", total, "page", pp.Page)
	return storage.NewPageResult(chapters, total, pp.Page, pp.Size), nil
}

// ListAllByNovel 返回某小说的全部章节（不分页），按阅读顺序升序。
func (s *Store) ListAllByNovel(ctx context.Context, novelID int64) ([]Chapter, error) {
	var chapters []Chapter
	if err := s.orderedByNovel(ctx, novelID).
		Order(chapterOrderAsc).
		Find(&chapters).Error; err != nil {
		return nil, fmt.Errorf("chapter store: list all: %w", err)
	}
	for i := range chapters {
		chapters[i].FilePath = git.ChapterPath(chapters[i].ID)
	}
	return chapters, nil
}

// CountByNovel 返回小说当前的章节总数。
// 阅读顺序中的章节号从 1 连续编号，因此该值也等于当前最大的展示章节号。
func (s *Store) CountByNovel(ctx context.Context, novelID int64) (int, error) {
	var count int64
	if err := s.DB.WithContext(ctx).
		Model(&Chapter{}).
		Where("novel_id = ?", novelID).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("chapter store: count by novel: %w", err)
	}
	return int(count), nil
}

// GetReadingNumberByID 返回章节在当前阅读顺序中的 1-based 位次。
// 章节号由卷顺序、卷内 sort_order 和未分卷末尾规则实时计算，不依赖 chapter_number 列。
func (s *Store) GetReadingNumberByID(ctx context.Context, novelID, chapterID int64) (int, error) {
	var ids []int64
	if err := s.orderedByNovel(ctx, novelID).
		Order(chapterOrderAsc).
		Pluck("chapters.id", &ids).Error; err != nil {
		return 0, fmt.Errorf("chapter store: list reading order: %w", err)
	}
	for i, id := range ids {
		if id == chapterID {
			return i + 1, nil
		}
	}
	return 0, fmt.Errorf("chapter store: get reading number: %w", gorm.ErrRecordNotFound)
}

// GetByID 按 novel_id + id 取单章。
func (s *Store) GetByID(ctx context.Context, novelID, chapterID int64) (*Chapter, error) {
	var ch Chapter
	if err := s.DB.WithContext(ctx).
		Where("novel_id = ? AND id = ?", novelID, chapterID).
		First(&ch).Error; err != nil {
		return nil, fmt.Errorf("chapter store: get by id: %w", err)
	}
	ch.FilePath = git.ChapterPath(ch.ID)
	return &ch, nil
}

// Create 新建章节记录，并追加到指定卷或未分卷组的末尾。
// volumeID 为 nil 时创建未分卷章节；非 nil 时必须属于该小说。
// tx 可为 nil；传入时创建与 sort_order 分配使用同一事务。
func (s *Store) Create(ctx context.Context, tx *gorm.DB, novelID int64, volumeID *int64, title string) (*Chapter, error) {
	db := s.DB
	if tx != nil {
		db = tx
	}

	var created *Chapter
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		pos, err := volume.NewStore(s.DB, s.logger).AllocateChapterSortOrder(ctx, tx, novelID, volumeID)
		if err != nil {
			return err
		}

		created = &Chapter{
			NovelID:   novelID,
			VolumeID:  volumeID,
			SortOrder: pos,
			Title:     title,
		}
		if err := tx.WithContext(ctx).Create(created).Error; err != nil {
			return fmt.Errorf("insert chapter: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("chapter store: create: %w", err)
	}
	return created, nil
}

// SearchByNovel 按关键词搜索某小说的章节，匹配标题和摘要。
func (s *Store) SearchByNovel(ctx context.Context, novelID int64, query string, limit int) ([]Chapter, error) {
	var chapters []Chapter
	if err := s.orderedByNovel(ctx, novelID).
		Where("chapters.title LIKE ? OR chapters.summary LIKE ?", "%"+query+"%", "%"+query+"%").
		Order(chapterOrderAsc).
		Limit(limit).
		Find(&chapters).Error; err != nil {
		return nil, fmt.Errorf("chapter store: search: %w", err)
	}
	for i := range chapters {
		chapters[i].FilePath = git.ChapterPath(chapters[i].ID)
	}
	return chapters, nil
}

// GetRecent 取阅读顺序最后 N 章，按阅读顺序倒序返回。
func (s *Store) GetRecent(ctx context.Context, novelID int64, limit int) ([]Chapter, error) {
	var chapters []Chapter
	if err := s.orderedByNovel(ctx, novelID).
		Order(chapterOrderDesc).
		Limit(limit).
		Find(&chapters).Error; err != nil {
		return nil, fmt.Errorf("chapter store: recent: %w", err)
	}
	for i := range chapters {
		chapters[i].FilePath = git.ChapterPath(chapters[i].ID)
	}
	return chapters, nil
}

// UpdateTitle 按 novel_id + id 更新章节标题。
func (s *Store) UpdateTitle(ctx context.Context, novelID, chapterID int64, title string) error {
	result := s.DB.WithContext(ctx).
		Model(&Chapter{}).
		Where("novel_id = ? AND id = ?", novelID, chapterID).
		Update("title", title)
	if result.Error != nil {
		return fmt.Errorf("chapter store: update title: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("chapter store: update title: %w", gorm.ErrRecordNotFound)
	}
	return nil
}

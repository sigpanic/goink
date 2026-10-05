package chapter

import (
	"context"
	"fmt"
	"log/slog"

	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/deletion"
	"github.com/sigpanic/goink/internal/git"
	"github.com/sigpanic/goink/internal/volume"
)

// ChapterChunkCleaner 清理某章节派生的向量数据。
type ChapterChunkCleaner interface {
	DeleteChapterChunks(ctx context.Context, novelID, chapterID int64) error
}

// ChapterCacheInvalidator 清理某章节的搜索缓存。
type ChapterCacheInvalidator interface {
	DeleteCachedChapter(novelID, chapterID int64)
}

// DeleteResult 描述章节删除的结果。References 非空时 Deleted 为 false。
type DeleteResult struct {
	Deleted    bool               `json:"deleted"`
	References []deletion.Blocker `json:"references"`
}

// Service 编排章节用例涉及的数据库、Git 文件与派生数据生命周期。
type Service struct {
	store         *Store
	volumes       *volume.Store
	deletionGuard *deletion.Guard
	logger        *slog.Logger

	chunkCleaner     func() ChapterChunkCleaner
	cacheInvalidator func() ChapterCacheInvalidator
}

// NewService 创建章节领域服务。provider 允许可选基础设施在运行时延迟初始化。
func NewService(store *Store, volumes *volume.Store, deletionGuard *deletion.Guard, logger *slog.Logger,
	chunkCleaner func() ChapterChunkCleaner, cacheInvalidator func() ChapterCacheInvalidator) *Service {
	return &Service{
		store:            store,
		volumes:          volumes,
		deletionGuard:    deletionGuard,
		logger:           logger,
		chunkCleaner:     chunkCleaner,
		cacheInvalidator: cacheInvalidator,
	}
}

// CreateDefault 按既有规则在最后一卷末尾创建章节；没有卷时创建到未分卷组。
func (s *Service) CreateDefault(ctx context.Context, novelID int64, title string) (*Chapter, error) {
	lastVolume, err := s.volumes.LastByNovel(ctx, nil, novelID)
	if err != nil {
		return nil, fmt.Errorf("get last volume: %w", err)
	}
	var volumeID *int64
	if lastVolume != nil {
		volumeID = &lastVolume.ID
	}
	return s.Place(ctx, PlaceInput{NovelID: novelID, Title: &title, TargetVolumeID: volumeID})
}

// Place 创建或移动章节，并将其放入目标章节组的指定位置。
func (s *Service) Place(ctx context.Context, input PlaceInput) (*Chapter, error) {
	if input.SourceChapterID == nil && input.Title == nil {
		return nil, fmt.Errorf("创建章节时必须提供标题")
	}
	if input.SourceChapterID != nil && input.Title != nil {
		return nil, fmt.Errorf("移动章节时不能提供标题")
	}

	var placed *Chapter
	if input.SourceChapterID == nil {
		err := s.store.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var err error
			placed, err = s.store.Place(ctx, tx, input)
			if err != nil {
				return err
			}
			placed.FilePath = git.ChapterPath(placed.ID)
			placed.OutlineFilePath = git.OutlinePath(placed.ID)
			if err := git.WriteFile(input.NovelID, placed.FilePath, ""); err != nil {
				return fmt.Errorf("create chapter file: %w", err)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	} else {
		var err error
		placed, err = s.store.Place(ctx, nil, input)
		if err != nil {
			return nil, err
		}
		placed.FilePath = git.ChapterPath(placed.ID)
		placed.OutlineFilePath = git.OutlinePath(placed.ID)
	}
	readingNumber, err := s.store.GetReadingNumberByID(ctx, nil, input.NovelID, placed.ID)
	if err != nil {
		return nil, fmt.Errorf("get chapter reading number: %w", err)
	}
	placed.ReadingNumber = readingNumber
	return placed, nil
}

// Delete 删除没有交叉引用的章节及其正文、大纲与派生索引。
func (s *Service) Delete(ctx context.Context, novelID, chapterID int64) (*DeleteResult, error) {
	references, err := s.deletionGuard.Blockers(ctx, deletion.Target{NovelID: novelID, Kind: deletion.EntityChapter, ID: chapterID})
	if err != nil {
		return nil, err
	}
	if len(references) > 0 {
		return &DeleteResult{References: references}, nil
	}
	// TODO: 如需跨文件系统与数据库的崩溃一致性，可先将文件原子移动到暂存目录，
	// 待事务提交后再异步删除，并在启动时按 DB 状态恢复或清理暂存文件。
	err = s.store.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.store.Delete(ctx, tx, novelID, chapterID); err != nil {
			return err
		}
		for _, path := range []string{git.ChapterPath(chapterID), git.OutlinePath(chapterID)} {
			if err := git.RemoveFile(novelID, path); err != nil {
				return fmt.Errorf("remove chapter file: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if s.chunkCleaner != nil {
		if cleaner := s.chunkCleaner(); cleaner != nil {
			if err := cleaner.DeleteChapterChunks(ctx, novelID, chapterID); err != nil {
				s.logger.Warn("删除章节向量失败", "novel_id", novelID, "chapter_id", chapterID, "err", err)
			}
		}
	}
	if s.cacheInvalidator != nil {
		if invalidator := s.cacheInvalidator(); invalidator != nil {
			invalidator.DeleteCachedChapter(novelID, chapterID)
		}
	}
	return &DeleteResult{Deleted: true}, nil
}

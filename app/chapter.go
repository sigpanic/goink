package app

import (
	"fmt"

	"github.com/sigpanic/goink/internal/chapter"
	"github.com/sigpanic/goink/internal/git"
)

// CreateChapterInput 是创建章节的入参。
type CreateChapterInput struct {
	NovelID int64  `json:"novel_id"`
	Title   string `json:"title"`
}

// ── 章节 ──────────────────────────────────────────────────

// GetChapters 返回指定小说的章节列表，含文件路径。
func (a *App) GetChapters(novelID int64) ([]chapter.Chapter, error) {
	chapters, err := a.chapter.ListAllByNovel(a.ctx, novelID)
	if err != nil {
		return nil, err
	}
	return chapters, nil
}

// GetMaxChapterNumber 返回该小说当前最大章节号，无章节时返回 0。前端确定写作进度用。
func (a *App) GetMaxChapterNumber(novelID int64) (int, error) {
	return a.chapter.GetLatestNumber(a.ctx, novelID)
}

// UpdateChapterTitle 更新章节标题。
func (a *App) UpdateChapterTitle(novelID int64, chapterNumber int, title string) error {
	return a.chapter.UpdateTitle(a.ctx, novelID, chapterNumber, title)
}

// CreateChapter 创建新章节，章节号自动递增。同时创建空正文文件。
func (a *App) CreateChapter(input CreateChapterInput) (*chapter.Chapter, error) {
	latest, err := a.chapter.GetLatestNumber(a.ctx, input.NovelID)
	if err != nil {
		return nil, fmt.Errorf("failed to create chapter: %w", err)
	}

	ch := chapter.Chapter{
		NovelID:       input.NovelID,
		ChapterNumber: latest + 1,
		Title:         input.Title,
	}

	if err := a.chapter.DB.WithContext(a.ctx).Create(&ch).Error; err != nil {
		return nil, fmt.Errorf("failed to create chapter: %w", err)
	}

	if err := git.WriteFile(input.NovelID, git.ChapterPath(ch.ChapterNumber), ""); err != nil {
		return nil, fmt.Errorf("failed to create chapter: %w", err)
	}

	ch.FilePath = git.ChapterPath(ch.ChapterNumber)

	return &ch, nil
}

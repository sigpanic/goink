package app

import (
	"fmt"
	"strings"

	"github.com/sigpanic/goink/internal/chapter"
	"github.com/sigpanic/goink/internal/git"
)

// ensureChapterIDsInNovel 确认章节 ID 均属于当前小说。
// chapter_id 是逻辑外键，App 写入交叉引用前必须显式校验其归属。
func (a *App) ensureChapterIDsInNovel(novelID int64, ids []int64) error {
	missing, err := a.chapter.MissingIDsByNovel(a.ctx, nil, novelID, ids)
	if err != nil {
		return fmt.Errorf("check chapter ownership: %w", err)
	}
	if len(missing) == 0 {
		return nil
	}

	values := make([]string, len(missing))
	for i, id := range missing {
		values[i] = fmt.Sprint(id)
	}
	return fmt.Errorf("章节 %s 不存在或不属于当前小说", strings.Join(values, "、"))
}

// CreateChapterInput 是创建章节的入参。
type CreateChapterInput struct {
	NovelID int64  `json:"novel_id"`
	Title   string `json:"title"`
}

// ── 章节 ──────────────────────────────────────────────────

// GetChapters 返回指定小说的章节列表，含文件路径。
func (a *App) GetChapters(novelID int64) ([]chapter.Chapter, error) {
	chapters, err := a.chapter.ListAllByNovel(a.ctx, nil, novelID)
	if err != nil {
		return nil, err
	}
	return chapters, nil
}

// GetMaxChapterNumber 返回该小说当前章节总数，无章节时返回 0。前端确定写作进度用。
// 展示章节号由阅读顺序实时计算，最大展示章节号等于章节总数。
func (a *App) GetMaxChapterNumber(novelID int64) (int, error) {
	return a.chapter.CountByNovel(a.ctx, nil, novelID)
}

// UpdateChapterTitle 更新章节标题。
func (a *App) UpdateChapterTitle(novelID, chapterID int64, title string) error {
	return a.chapter.UpdateTitle(a.ctx, nil, novelID, chapterID, title)
}

// CreateChapter 创建新章节。同时创建空正文文件。
// 新章节默认追加到最后一卷；尚未建卷时追加到未分卷组。
func (a *App) CreateChapter(input CreateChapterInput) (*chapter.Chapter, error) {
	lastVolume, err := a.volume.LastByNovel(a.ctx, nil, input.NovelID)
	if err != nil {
		return nil, fmt.Errorf("failed to create chapter: %w", err)
	}

	var volumeID *int64
	if lastVolume != nil {
		volumeID = &lastVolume.ID
	}

	ch, err := a.chapter.Create(a.ctx, nil, input.NovelID, volumeID, input.Title)
	if err != nil {
		return nil, fmt.Errorf("failed to create chapter: %w", err)
	}

	ch.FilePath = git.ChapterPath(ch.ID)
	if err := git.WriteFile(input.NovelID, ch.FilePath, ""); err != nil {
		return nil, fmt.Errorf("failed to create chapter: %w", err)
	}

	readingNumber, err := a.chapter.GetReadingNumberByID(a.ctx, nil, input.NovelID, ch.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to create chapter: %w", err)
	}
	ch.ReadingNumber = readingNumber

	return ch, nil
}

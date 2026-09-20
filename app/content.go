package app

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/sigpanic/goink/internal/chapter"
	"github.com/sigpanic/goink/internal/git"
	"github.com/sigpanic/goink/internal/rag"
	"github.com/sigpanic/goink/internal/skill"
	"github.com/sigpanic/goink/internal/text"
)

// SaveContentInput 是保存文件内容的入参。
type SaveContentInput struct {
	NovelID int64  `json:"novel_id"`
	Path    string `json:"path"`
	Content string `json:"content"`
}

// GetContent 返回小说仓库中指定路径的文件内容。文件不存在时返回空字符串。
// 内置 skill 路径（/builtin/skills/）从内存读取。
func (a *App) GetContent(novelID int64, path string) (string, error) {
	if strings.HasPrefix(path, "/builtin/skills/") {
		name := strings.TrimSuffix(strings.TrimPrefix(path, "/builtin/skills/"), ".md")
		if a.skill == nil {
			return "", os.ErrNotExist
		}
		sk, ok := a.skill.Get(novelID, name)
		if !ok {
			return "", os.ErrNotExist
		}
		return sk.RawContent, nil
	}

	content, err := git.ReadFile(novelID, path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	return content, nil
}

// SaveContent 保存小说仓库中指定路径的文件内容。
func (a *App) SaveContent(input SaveContentInput) error {
	if isSkillPath(input.Path) {
		if _, err := skill.ParseBytes([]byte(input.Content), ""); err != nil {
			return fmt.Errorf("skill 格式错误: %w", err)
		}
	}

	if err := git.WriteFile(input.NovelID, input.Path, input.Content); err != nil {
		return err
	}

	if ref, ok := git.ParseChapterLikePath(input.Path); ok && !ref.IsOutline && !ref.IsNew && ref.VolumeID == 0 {
		chapterID := ref.ID
		rag.SubmitRefresh(input.NovelID, chapterID, input.Content)
		if svc := a.searchService.Load(); svc != nil {
			svc.UpdateCachedChapter(input.NovelID, chapterID, input.Content)
		}
		stats := text.ComputeStats(input.Content)

		// 记录字数变化
		var oldWC int
		a.chapter.DB.WithContext(a.ctx).
			Model(&chapter.Chapter{}).
			Select("COALESCE(word_count, 0)").
			Where("novel_id = ? AND id = ?", input.NovelID, chapterID).
			Scan(&oldWC)
		if delta := stats.WordCount - oldWC; delta != 0 && a.writing != nil {
			a.writing.LogDelta(a.ctx, input.NovelID, chapterID, delta)
		}

		if err := a.chapter.DB.WithContext(a.ctx).
			Model(&chapter.Chapter{}).
			Where("novel_id = ? AND id = ?", input.NovelID, chapterID).
			Update("word_count", stats.WordCount).Error; err != nil {
			a.logger.Warn("更新字数失败", "novel_id", input.NovelID, "chapter_id", chapterID, "err", err)
		}
	}

	return nil
}

func isSkillPath(p string) bool {
	return strings.HasPrefix(p, "skills/") || strings.HasPrefix(p, "~/.goink/skills/")
}

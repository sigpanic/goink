package app

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"novel/internal/chapter"
	"novel/internal/git"
	"novel/internal/rag"
	"novel/internal/skill"
	"novel/internal/text"
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

var chPathRe = regexp.MustCompile(`^chapters/(\d{1,6})\.md$`)

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

	if m := chPathRe.FindStringSubmatch(input.Path); m != nil {
		chapNum, _ := strconv.Atoi(m[1])
		rag.SubmitRefresh(input.NovelID, chapNum, input.Content)
		if svc := a.searchService.Load(); svc != nil {
			svc.UpdateCachedChapter(input.NovelID, chapNum, input.Content)
		}
		stats := text.ComputeStats(input.Content)

		// 记录字数变化
		var oldWC int
		a.chapter.DB.WithContext(a.ctx).
			Model(&chapter.Chapter{}).
			Select("COALESCE(word_count, 0)").
			Where("novel_id = ? AND chapter_number = ?", input.NovelID, chapNum).
			Scan(&oldWC)
		if delta := stats.WordCount - oldWC; delta != 0 && a.writing != nil {
			a.writing.LogDelta(a.ctx, input.NovelID, chapNum, delta)
		}

		if err := a.chapter.DB.WithContext(a.ctx).
			Model(&chapter.Chapter{}).
			Where("novel_id = ? AND chapter_number = ?", input.NovelID, chapNum).
			Update("word_count", stats.WordCount).Error; err != nil {
			a.logger.Warn("更新字数失败", "novel_id", input.NovelID, "chapter", chapNum, "err", err)
		}
	}

	return nil
}

func isSkillPath(p string) bool {
	return strings.HasPrefix(p, "skills/") || strings.HasPrefix(p, "~/.goink/skills/")
}

package app

import (
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/sigpanic/goink/internal/chapter"
	"github.com/sigpanic/goink/internal/git"
	"github.com/sigpanic/goink/internal/rag"
	"github.com/sigpanic/goink/internal/skill"
	"github.com/sigpanic/goink/internal/text"
)

// SaveContentInput 是保存文件内容的入参。
type SaveContentInput struct {
	NovelID         int64   `json:"novel_id"`
	Path            string  `json:"path"`
	Content         string  `json:"content"`
	ExpectedContent *string `json:"expected_content,omitempty"`
}

// GetContent 返回小说仓库中指定路径的文件内容。文件不存在时返回空字符串。
// 内置 skill 路径（/builtin/skills/）从内存读取。
func (a *App) GetContent(novelID int64, path string) (string, error) {
	if err := a.validateChapterContentPath(novelID, path); err != nil {
		return "", err
	}
	if err := a.validateVolumeContentPath(novelID, path); err != nil {
		return "", err
	}
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
	if err := a.validateChapterContentPath(input.NovelID, input.Path); err != nil {
		return err
	}
	if err := a.validateVolumeContentPath(input.NovelID, input.Path); err != nil {
		return err
	}
	if isSkillPath(input.Path) {
		if _, err := skill.ParseBytes([]byte(input.Content), ""); err != nil {
			return fmt.Errorf("skill 格式错误: %w", err)
		}
	}

	var err error
	if input.ExpectedContent != nil {
		err = git.WriteFileIfUnchanged(input.NovelID, input.Path, *input.ExpectedContent, input.Content)
	} else {
		err = git.WriteFile(input.NovelID, input.Path, input.Content)
	}
	if errors.Is(err, git.ErrFileChanged) {
		return fmt.Errorf("CONTENT_CONFLICT: 磁盘内容已变化，请处理冲突后再保存: %w", err)
	}
	if err != nil {
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
		if a.writing != nil && input.ExpectedContent != nil {
			changes := text.ComputeWordChanges(*input.ExpectedContent, input.Content)
			a.writing.LogChanges(a.ctx, input.NovelID, chapterID, changes.Added, changes.Deleted)
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

func (a *App) validateChapterContentPath(novelID int64, filePath string) error {
	// 剥离前导 /：ResolvePath 的 filepath.Join 会吞掉它，若不剥离则 /chapters/... 会绕过校验
	clean := strings.TrimPrefix(strings.ToLower(path.Clean(strings.ReplaceAll(filePath, "\\", "/"))), "/")
	if !strings.HasPrefix(clean, "chapters/") && !strings.HasPrefix(clean, "outlines/") {
		return nil
	}

	ref, ok := git.ParseChapterLikePath(filePath)
	if !ok || ref.IsNew || ref.ID <= 0 {
		return fmt.Errorf("章节路径无效: %q", filePath)
	}
	canonical := git.ChapterPath(ref.ID)
	if ref.IsOutline {
		canonical = git.OutlinePath(ref.ID)
	}
	if filePath != canonical {
		return fmt.Errorf("章节路径非规范格式: %q，应使用 %q", filePath, canonical)
	}
	return a.ensureChapterIDsInNovel(novelID, []int64{ref.ID})
}

func (a *App) validateVolumeContentPath(novelID int64, filePath string) error {
	normalized := strings.ToLower(strings.ReplaceAll(filePath, "\\", "/"))
	// 同 validateChapterContentPath：剥离前导 /，避免 /volumes/... 绕过校验
	clean := strings.TrimPrefix(path.Clean(normalized), "/")
	if normalized != "volumes" && !strings.HasPrefix(normalized, "volumes/") &&
		clean != "volumes" && !strings.HasPrefix(clean, "volumes/") {
		return nil
	}

	volumeID, ok := git.ParseVolumePath(filePath)
	if !ok || filePath != git.VolumePath(volumeID) {
		return fmt.Errorf("卷纲路径无效: %q", filePath)
	}
	_, err := a.volume.GetByID(a.ctx, nil, novelID, volumeID)
	return err
}

func isSkillPath(p string) bool {
	return strings.HasPrefix(p, "skills/") || strings.HasPrefix(p, "~/.goink/skills/")
}

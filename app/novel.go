package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/chapter"
	"github.com/sigpanic/goink/internal/character"
	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/export"
	"github.com/sigpanic/goink/internal/git"
	"github.com/sigpanic/goink/internal/location"
	"github.com/sigpanic/goink/internal/novel"
	"github.com/sigpanic/goink/internal/preference"
	"github.com/sigpanic/goink/internal/reader"
	"github.com/sigpanic/goink/internal/session"
	"github.com/sigpanic/goink/internal/setting"
	"github.com/sigpanic/goink/internal/storyarc"
	"github.com/sigpanic/goink/internal/timeline"
)

// ── 小说 ──────────────────────────────────────────────────

// SetActiveNovelInput 是切换当前小说的入参。
type SetActiveNovelInput struct {
	NovelID int64 `json:"novel_id"`
}

// ── 小说操作 ──────────────────────────────────────────────

// GetNovels 返回小说列表。
func (a *App) GetNovels() ([]novel.Novel, error) {
	result, err := a.novel.List(a.ctx, novel.ListNovelsOptions{})
	if err != nil {
		return nil, err
	}
	return result.Items, nil
}

// CreateNovelInput 是创建小说的入参。
type CreateNovelInput struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Genre       string `json:"genre,omitempty"`
}

// CreateNovel 创建一部新小说。
func (a *App) CreateNovel(input CreateNovelInput) (*novel.Novel, error) {
	n := novel.Novel{
		Title:       input.Title,
		Description: input.Description,
		Genre:       input.Genre,
	}
	if err := a.novel.DB.WithContext(a.ctx).Create(&n).Error; err != nil {
		return nil, fmt.Errorf("failed to create novel: %w", err)
	}

	if _, err := git.New(n.ID, a.settings.GitName, a.settings.GitEmail, a.logger); err != nil {
		a.logger.Error("创建小说失败: git 初始化失败", "novelID", n.ID, "title", n.Title, "error", err)
		a.novel.DB.WithContext(a.ctx).Delete(&n) // 回滚孤儿 DB 记录
		return nil, fmt.Errorf("failed to init novel repo: %w", err)
	}

	// 为新小说创建 goink.md 空文件
	if err := git.WriteFile(n.ID, git.GoinkPath(), ""); err != nil {
		a.logger.Error("创建小说失败: goink.md 写入失败", "novelID", n.ID, "error", err)
		return nil, fmt.Errorf("failed to create goink.md: %w", err)
	}

	a.logger.Info("小说创建成功", "novelID", n.ID, "title", n.Title)
	return &n, nil
}

// SetActiveNovel 记录当前活跃的小说 ID，下次启动自动恢复。
func (a *App) SetActiveNovel(input SetActiveNovelInput) error {
	a.settings.LastNovelID = input.NovelID
	return config.SaveSettings(a.db, a.settings)
}

// UpdateNovelInput 是更新小说的入参，空字段不更新（PATCH 语义）。
type UpdateNovelInput struct {
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Genre       string `json:"genre,omitempty"`
}

// UpdateNovel 更新小说信息。
func (a *App) UpdateNovel(novelID int64, input UpdateNovelInput) (*novel.Novel, error) {
	var n novel.Novel
	if err := a.novel.DB.WithContext(a.ctx).First(&n, novelID).Error; err != nil {
		return nil, fmt.Errorf("update novel: %w", err)
	}
	if input.Title != "" {
		n.Title = input.Title
	}
	if input.Description != "" {
		n.Description = input.Description
	}
	if input.Genre != "" {
		n.Genre = input.Genre
	}
	if err := a.novel.DB.WithContext(a.ctx).Save(&n).Error; err != nil {
		return nil, fmt.Errorf("update novel: %w", err)
	}
	return &n, nil
}

// DeleteNovel 删除小说，级联清理所有关联数据、Git 仓库目录。
func (a *App) DeleteNovel(novelID int64) error {
	var n novel.Novel
	if err := a.novel.DB.WithContext(a.ctx).First(&n, novelID).Error; err != nil {
		return fmt.Errorf("delete novel: %w", err)
	}

	// 用 SafePath 从 novelID 重新计算待删目录
	novelsParent := filepath.Join(config.DataDirPath(), "novels")
	safeDir, err := git.SafePath(novelsParent, fmt.Sprintf("%d", novelID))
	if err != nil {
		return fmt.Errorf("delete novel: unsafe path: %w", err)
	}

	// 在事务中先删子表再删主表
	err = a.novel.DB.WithContext(a.ctx).Transaction(func(tx *gorm.DB) error {
		// 仅删小说专属偏好，保留全局偏好
			if err := tx.Where("is_global = ? AND novel_id = ?", false, novelID).Delete(&preference.PreferenceItem{}).Error; err != nil {
				return fmt.Errorf("preferences: %w", err)
			}
			// v2 取消全局设定概念，设定全部归属当前小说，直接按 novel_id 删
			if err := tx.Where("novel_id = ?", novelID).Delete(&setting.SettingItem{}).Error; err != nil {
				return fmt.Errorf("settings: %w", err)
			}
		for _, op := range []struct {
			label string
			fn    func(*gorm.DB) error
		}{
			{"characters", func(tx *gorm.DB) error { return tx.Where("novel_id = ?", novelID).Delete(&character.Character{}).Error }},
			{"character_relations", func(tx *gorm.DB) error {
				return tx.Where("novel_id = ?", novelID).Delete(&character.CharacterRelation{}).Error
			}},
			{"chapters", func(tx *gorm.DB) error { return tx.Where("novel_id = ?", novelID).Delete(&chapter.Chapter{}).Error }},
			{"locations", func(tx *gorm.DB) error { return tx.Where("novel_id = ?", novelID).Delete(&location.Location{}).Error }},
			{"location_relations", func(tx *gorm.DB) error {
				return tx.Where("novel_id = ?", novelID).Delete(&location.LocationRelation{}).Error
			}},
			{"time_entries", func(tx *gorm.DB) error {
				return tx.Where("novel_id = ?", novelID).Delete(&timeline.TimelineEntry{}).Error
			}},
			{"story_arcs", func(tx *gorm.DB) error { return tx.Where("novel_id = ?", novelID).Delete(&storyarc.StoryArc{}).Error }},
			{"arc_nodes", func(tx *gorm.DB) error { return tx.Where("novel_id = ?", novelID).Delete(&storyarc.ArcNode{}).Error }},
			{"reader_perspectives", func(tx *gorm.DB) error {
				return tx.Where("novel_id = ?", novelID).Delete(&reader.ReaderPerspective{}).Error
			}},
			{"sessions", func(tx *gorm.DB) error { return tx.Where("novel_id = ?", novelID).Delete(&session.Session{}).Error }},
		} {
			if err := op.fn(tx); err != nil {
				return fmt.Errorf("%s: %w", op.label, err)
			}
		}
		return tx.Delete(&n).Error
	})
	if err != nil {
		return fmt.Errorf("delete novel: %w", err)
	}

	if err := os.RemoveAll(safeDir); err != nil {
		return fmt.Errorf("delete novel dir: %w", err)
	}

	// 如果删除的是当前活跃书籍，清除记录
	if a.settings.LastNovelID == novelID {
		a.settings.LastNovelID = 0
		if err := config.SaveSettings(a.db, a.settings); err != nil {
			return fmt.Errorf("delete novel: clear last novel: %w", err)
		}
	}
	return nil
}

// ── 封面 ──────────────────────────────────────────────────

// SaveCover 保存小说封面并提交到 Git 仓库。
func (a *App) SaveCover(novelID int64, data []byte) error {
	repo, err := git.New(novelID, a.settings.GitName, a.settings.GitEmail, a.logger)
	if err != nil {
		return fmt.Errorf("save cover: %w", err)
	}
	dir := config.NovelDirPath(novelID)
	coverPath, err := git.SafePath(dir, git.CoverPath())
	if err != nil {
		return fmt.Errorf("save cover: %w", err)
	}
	if err := os.WriteFile(coverPath, data, 0644); err != nil {
		return fmt.Errorf("save cover: %w", err)
	}
	if err := repo.StageAll(); err != nil {
		return fmt.Errorf("save cover: %w", err)
	}
	if _, err := repo.Commit("update cover"); err != nil {
		return fmt.Errorf("save cover: %w", err)
	}
	return nil
}

// ── 导出 ──────────────────────────────────────────────────

// ExportNovel 将小说导出为指定格式，弹出保存对话框让用户选择保存位置。
func (a *App) ExportNovel(novelID int64, format string) error {
	var n novel.Novel
	if err := a.novel.DB.WithContext(a.ctx).First(&n, novelID).Error; err != nil {
		return fmt.Errorf("export novel: %w", err)
	}

	chapters, err := a.chapter.ListAllByNovel(a.ctx, novelID)
	if err != nil {
		return fmt.Errorf("export novel: %w", err)
	}
	if len(chapters) == 0 {
		return fmt.Errorf("export novel: 没有可导出的章节")
	}

	var cc []export.ChapterWithContent
	for _, ch := range chapters {
		content, err := git.ReadFile(novelID, git.ChapterPath(ch.ChapterNumber))
		if err != nil {
			return fmt.Errorf("export novel: 读取第%d章失败: %w", ch.ChapterNumber, err)
		}
		cc = append(cc, export.ChapterWithContent{Chapter: ch, Content: content})
	}

	data, filename, err := export.ExportNovel(&n, cc, format, a.settings.UserName)
	if err != nil {
		return fmt.Errorf("export novel: %w", err)
	}

	filters := map[string][]runtime.FileFilter{
		"epub":     {{DisplayName: "EPUB 电子书 (*.epub)", Pattern: "*.epub"}},
		"markdown": {{DisplayName: "Markdown 文件 (*.md)", Pattern: "*.md"}},
		"txt":      {{DisplayName: "文本文件 (*.txt)", Pattern: "*.txt"}},
	}

	savePath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename:      filename,
		Title:                "导出小说",
		Filters:              filters[format],
		CanCreateDirectories: true,
	})
	if err != nil {
		return fmt.Errorf("export novel: %w", err)
	}
	if savePath == "" {
		return nil // 用户取消
	}

	if err := os.WriteFile(savePath, data, 0644); err != nil {
		return fmt.Errorf("export novel: 写入文件失败: %w", err)
	}
	return nil
}

// DeleteCover 删除小说封面并提交到 Git 仓库。
func (a *App) DeleteCover(novelID int64) error {
	repo, err := git.New(novelID, a.settings.GitName, a.settings.GitEmail, a.logger)
	if err != nil {
		return fmt.Errorf("delete cover: %w", err)
	}
	dir := config.NovelDirPath(novelID)
	coverPath, err := git.SafePath(dir, git.CoverPath())
	if err != nil {
		return fmt.Errorf("delete cover: %w", err)
	}
	if err := os.Remove(coverPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("delete cover: %w", err)
	}
	if err := repo.StageAll(); err != nil {
		return fmt.Errorf("delete cover: %w", err)
	}
	if _, err := repo.Commit("remove cover"); err != nil {
		return fmt.Errorf("delete cover: %w", err)
	}
	return nil
}

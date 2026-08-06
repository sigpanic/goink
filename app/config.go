package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/git"
	"github.com/sigpanic/goink/internal/novel"
	"github.com/sigpanic/goink/internal/rag"
	"github.com/sigpanic/goink/internal/storage"
)

// SaveSettingsInput 是保存设置的入参。
type SaveSettingsInput struct {
	// 后续加 LLM 配置字段（provider、模型选择、APIKey 等）
}

// ── 设置 ──────────────────────────────────────────────────

// GetSettings 返回运行时配置。
func (a *App) GetSettings() (*config.AppSettings, error) {
	return a.settings, nil
}

// SaveSettings 保存运行时配置。
func (a *App) SaveSettings(input SaveSettingsInput) error {
	return config.SaveSettings(a.db, a.settings)
}

// SetSelectedModel 保存选中的模型 key 和推理程度，持久化到 DB。
func (a *App) SetSelectedModel(key, effort string) error {
	a.settings.SelectedModelKey = key
	a.settings.ReasoningEffort = effort
	return config.SaveSettings(a.db, a.settings)
}

// SetReasoningEffort 单独保存推理程度。
func (a *App) SetReasoningEffort(effort string) error {
	a.settings.ReasoningEffort = effort
	return config.SaveSettings(a.db, a.settings)
}

// SetLastSession 保存上次活跃的会话 ID。
func (a *App) SetLastSession(sessionID string) error {
	a.settings.LastSessionID = sessionID
	return config.SaveSettings(a.db, a.settings)
}

// SaveUserName 保存用户名称。
func (a *App) SaveUserName(name string) error {
	a.settings.UserName = name
	return config.SaveSettings(a.db, a.settings)
}

// SaveGitConfig 保存 Git user.name 和 user.email，并同步到所有已有仓库。
func (a *App) SaveGitConfig(name, email string) error {
	a.settings.GitName = name
	a.settings.GitEmail = email
	if err := config.SaveSettings(a.db, a.settings); err != nil {
		return err
	}
	result, err := a.novel.List(a.ctx, novel.ListNovelsOptions{PageParams: storage.PageParams{Size: -1}})
	if err != nil {
		return fmt.Errorf("save git config: list novels: %w", err)
	}
	var errs []string
	for _, n := range result.Items {
		repo, repoErr := git.New(n.ID, name, email, a.logger)
		if repoErr != nil {
			errs = append(errs, fmt.Sprintf("小说 %d: %v", n.ID, repoErr))
			continue
		}
		if err := repo.SetGitConfig(name, email); err != nil {
			errs = append(errs, fmt.Sprintf("小说 %d: %v", n.ID, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("部分小说配置同步失败:\n%s", strings.Join(errs, "\n"))
	}
	return nil
}

// SaveAvatar 保存用户头像到数据目录。
func (a *App) SaveAvatar(data []byte) error {
	userDir := filepath.Join(config.DataDirPath(), "user")
	if err := os.MkdirAll(userDir, 0700); err != nil {
		return fmt.Errorf("save avatar: %w", err)
	}
	avatarPath := filepath.Join(userDir, "avatar.jpg")
	return os.WriteFile(avatarPath, data, 0644)
}

// RebuildNovelIndex 无条件全量重建指定小说的向量索引，用于数据异常时的手动兜底。
func (a *App) RebuildNovelIndex(novelID int64) error {
	rq := rag.GetRefreshQueue()
	if rq == nil {
		return fmt.Errorf("向量索引服务未初始化")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	return rq.RebuildNovel(ctx, novelID)
}

package app

import (
	"time"

	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/update"
	"github.com/sigpanic/goink/internal/version"
)

// updateCheckInterval 自动检查的最小间隔。距上次检查不足此值时，自动检查跳过（不发包）；
// 手动检查（skipDismiss=true）不受此限。
const updateCheckInterval = 12 * time.Hour

// CheckUpdate 检查 GitHub Release 是否有新版本。
// skipDismiss 为 true 时跳过已忽略版本的过滤 + 12h 节流（手动检查场景）。
// 返回 CheckResult（包含是否有更新的信息），网络/解析错误时返回 error。
// 自动检查场景下，若距上次检查不足 12h，或用户已忽略过该版本且没有更新的版本，返回 nil。
func (a *App) CheckUpdate(skipDismiss bool) (*update.CheckResult, error) {
	// 自动检查：12h 节流，避免每次启动/重启都打 GitHub。零值表示从未检查过，放行。
	if !skipDismiss {
		lastCheck := a.settingsSnapshot().LastUpdateCheckAt
		if !lastCheck.IsZero() && time.Since(lastCheck) < updateCheckInterval {
			return nil, nil
		}
	}

	result, err := update.CheckLatest(a.logger)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	// 请求成功（拿到结果）：更新上次检查时间戳。
	// 手动检查也更新，避免手动查完紧接着启动又查一次（节流目的是减少 GitHub 请求，手动查也算一次）。
	if saveErr := a.updateSettings(func(s *config.AppSettings) {
		s.LastUpdateCheckAt = time.Now()
	}); saveErr != nil {
		a.logger.Warn("update: 保存上次检查时间失败", "err", saveErr)
	}

	// 自动检查场景下，检查用户是否已忽略过该版本
	if !skipDismiss && result.HasUpdate && a.settingsSnapshot().DismissedVersion == result.Latest.TagName {
		return nil, nil
	}

	return result, nil
}

// DismissUpdate 记录用户已忽略的更新版本号，同一版本不再提示。
func (a *App) DismissUpdate(tagName string) error {
	return a.updateSettings(func(s *config.AppSettings) {
		s.DismissedVersion = tagName
	})
}

// GetVersion 返回当前应用版本号。
func (a *App) GetVersion() string {
	return version.Version
}

// GetBuildHash 返回构建时的短 Git 提交号。
func (a *App) GetBuildHash() string {
	return version.BuildHash()
}

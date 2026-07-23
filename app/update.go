package app

import (
	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/update"
	"github.com/sigpanic/goink/internal/version"
)

// CheckUpdate 检查 GitHub Release 是否有新版本。
// skipDismiss 为 true 时跳过已忽略版本的过滤（手动检查场景）。
// 返回 CheckResult（包含是否有更新的信息），网络/解析错误时返回 error。
// 自动检查场景下，如果用户已忽略过该版本且没有更新的版本，返回 nil。
func (a *App) CheckUpdate(skipDismiss bool) (*update.CheckResult, error) {
	result, err := update.CheckLatest(a.logger)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	// 自动检查场景下，检查用户是否已忽略过该版本
	if !skipDismiss && result.HasUpdate && a.settings.DismissedVersion == result.Latest.TagName {
		return nil, nil
	}

	return result, nil
}

// DismissUpdate 记录用户已忽略的更新版本号，同一版本不再提示。
func (a *App) DismissUpdate(tagName string) error {
	a.settings.DismissedVersion = tagName
	return config.SaveSettings(a.db, a.settings)
}

// GetVersion 返回当前应用版本号。
func (a *App) GetVersion() string {
	return version.Version
}

package agentcfg

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/preference"
	"github.com/sigpanic/goink/internal/setting"
)

// NovelProfile 构建小说稳定设定快照（设定+偏好），独立 system 消息。
// 与 NovelState 区分：Profile 装长期稳定内容（设定/偏好），State 装动态内容（goink.md）。
// KV cache 友好：Profile 跨轮稳定，State 每轮可能变。
//
// 调用方负责事务外构建（compress 模式）：传入的 db 应为带 ctx 的非事务句柄，
// 避免在事务内调用导致 SQLite 单连接池死锁。
//
// 设定软上限 12k token（setting.SettingsTokenBudget）、偏好软上限 8k token（preference.PreferencesTokenBudget）：
// 超预算时按 updated_at DESC 保留最近活跃的，丢弃最久未活动的。这里用 slog.Default() 记 warn，best-effort 不阻塞流程。
// 拼接顺序：设定在前、偏好在后（与 system 消息注入顺序一致）。
//
// v2：设定取消 is_global，全部归属当前小说（只查 novel_id = ?）；
// 偏好保留 is_global（用户级容器，含全局+小说级，仍查 is_global OR novel_id）。
// 排序改 updated_at DESC：用户最近更新过的旧条目也能被保留（不被截断），比 created_at 更符合"活跃=保留"语义。
func NovelProfile(ctx context.Context, db *gorm.DB, novelID int64) (string, error) {
	// 加载设定（v2 取消 is_global，只查 novel_id，updated_at DESC 最近活跃在前）
	var setItems []setting.SettingItem
	if err := db.WithContext(ctx).
		Where("novel_id = ?", novelID).
		Order("updated_at DESC").
		Find(&setItems).Error; err != nil {
		return "", fmt.Errorf("agentcfg: load settings: %w", err)
	}

	// 加载偏好（同排序策略：is_global DESC 全局在前，组内 updated_at DESC 最近活跃在前）
	var prefItems []preference.PreferenceItem
	if err := db.WithContext(ctx).
		Where("is_global = ? OR novel_id = ?", true, novelID).
		Order("is_global DESC, updated_at DESC").
		Find(&prefItems).Error; err != nil {
		return "", fmt.Errorf("agentcfg: load preferences: %w", err)
	}

	setText, setTruncated := setting.FormatSettings(setItems)
	prefText, prefTruncated := preference.FormatPreferences(prefItems)

	if setTruncated > 0 {
		slog.Warn("novel profile: settings truncated due to budget exceeded",
			"novel_id", novelID,
			"total", len(setItems),
			"truncated", setTruncated,
			"budget", setting.SettingsTokenBudget,
		)
	}
	if prefTruncated > 0 {
		slog.Warn("novel profile: preferences truncated due to budget exceeded",
			"novel_id", novelID,
			"total", len(prefItems),
			"truncated", prefTruncated,
			"budget", preference.PreferencesTokenBudget,
		)
	}

	var b strings.Builder
	b.WriteString(setText)
	b.WriteString(prefText)
	return b.String(), nil
}

package agentcfg

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/novel"
)

// NovelProfile 构建小说稳定设定快照（设定+偏好），独立 system 消息。
// 与 NovelState 区分：Profile 装长期稳定内容（设定/偏好），State 装动态内容（goink.md）。
// KV cache 友好：Profile 跨轮稳定，State 每轮可能变。
// 本轮只注入【偏好】，下一轮扩展【设定】。
//
// 调用方负责事务外构建（compress 模式）：传入的 db 应为带 ctx 的非事务句柄，
// 避免在事务内调用导致 SQLite 单连接池死锁。
func NovelProfile(ctx context.Context, db *gorm.DB, novelID int64) (string, error) {
	var items []novel.PreferenceItem
	if err := db.WithContext(ctx).
		Where("is_global = ? OR novel_id = ?", true, novelID).
		Order("is_global DESC, created_at ASC").
		Find(&items).Error; err != nil {
		return "", fmt.Errorf("agentcfg: load preferences: %w", err)
	}

	return novel.FormatPreferences(items), nil
}

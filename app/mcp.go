package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/sigpanic/goink/internal/mcpserver"
	"github.com/sigpanic/goink/internal/novel"
	"gorm.io/gorm"
)

// currentNovel 读取界面确认过的当前小说，供每次 MCP 调用取得独立快照。
func (a *App) currentNovel(ctx context.Context) (mcpserver.CurrentNovel, error) {
	id := a.activeNovelID.Load()
	if id <= 0 {
		return mcpserver.CurrentNovel{}, mcpserver.ErrNoCurrentNovel
	}
	var item novel.Novel
	if err := a.novel.DB.WithContext(ctx).First(&item, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return mcpserver.CurrentNovel{}, mcpserver.ErrNoCurrentNovel
		}
		return mcpserver.CurrentNovel{}, fmt.Errorf("读取当前小说: %w", err)
	}
	return mcpserver.CurrentNovel{ID: item.ID, Title: item.Title}, nil
}

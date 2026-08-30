package app

import (
	"fmt"
	"runtime"

	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/storage"
)

// GetAppConfig 返回当前运行时配置信息（供前端诊断）。
func (a *App) GetAppConfig() map[string]any {
	if a.cfg == nil {
		return map[string]any{"initialized": false}
	}
	return map[string]any{
		"initialized": true,
		"data_dir":    config.DataDirPath(),
	}
}

// UpdateDataDir 更改数据目录并重新初始化所有运行时模块。
//
// TODO: 实现数据迁移——更改目录时自动将旧目录中的数据文件移动到新目录。
// 同盘用 os.Rename（原子），跨盘用递归拷贝+进度回调，目标非空时弹确认框。
func (a *App) UpdateDataDir(newPath string) error {
	if newPath == "" {
		return fmt.Errorf("数据目录路径不能为空")
	}

	// 先保存新配置，失败时旧 DB 仍可用
	if err := config.Save(newPath); err != nil {
		return fmt.Errorf("保存配置失败: %w", err)
	}

	// 关闭旧数据库
	if a.db != nil {
		if err := storage.Close(a.db); err != nil {
			return fmt.Errorf("关闭旧数据库失败: %w", err)
		}
		a.db = nil
	}

	// 重新加载并初始化
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("加载新配置失败: %w", err)
	}

	if err := a.initWithConfig(cfg); err != nil {
		return fmt.Errorf("重新初始化失败: %w", err)
	}
	a.logger.Info("数据目录已更改", "data_dir", config.DataDirPath())
	return nil
}

// GetPlatform 返回平台信息，供前端决定默认路径等行为。
func (a *App) GetPlatform() map[string]any {
	return map[string]any{
		"os":          runtime.GOOS,
		"defaultPath": config.DataDirPath(),
	}
}

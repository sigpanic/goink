package engine

import (
	"fmt"
	"log/slog"

	"gorm.io/gorm"
)

// AutoMigrateStep 使用冻结模型对齐本组 schema；进度由 RunSteps 统一记录。
// auto 完成状态必须落盘后才能进入 post，避免重启时重新补回 post 已删除的列。
type AutoMigrateStep struct {
	Models func() []any
}

func (AutoMigrateStep) Key() string { return "auto" }

func (s AutoMigrateStep) Run(db *gorm.DB, _ *slog.Logger) error {
	for _, model := range s.Models() {
		if err := db.AutoMigrate(model); err != nil {
			return fmt.Errorf("auto migrate %T: %w", model, err)
		}
	}
	return nil
}

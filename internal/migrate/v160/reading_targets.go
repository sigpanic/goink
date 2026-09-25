package v160

import (
	"fmt"
	"log/slog"

	"gorm.io/gorm"
)

// migrateReadingTargets 保留未来计划的阅读位置。target_chapter 不是指向既有章节的
// 交叉引用：未来章节没有 ID，因此不能参与 num→id 重写。
func migrateReadingTargets(db *gorm.DB, _ *slog.Logger) error {
	targets := []struct {
		table string
	}{
		{table: "time_entries"},
		{table: "arc_nodes"},
	}
	for _, target := range targets {
		if !db.Migrator().HasTable(target.table) ||
			!db.Migrator().HasColumn(target.table, "target_chapter") ||
			!db.Migrator().HasColumn(target.table, "target_reading_number") {
			continue
		}
		query := fmt.Sprintf("UPDATE %s SET target_reading_number = target_chapter WHERE target_reading_number = 0 AND target_chapter > 0", target.table)
		if err := db.Exec(query).Error; err != nil {
			return fmt.Errorf("migrate v160: 保留 %s 的目标阅读序号: %w", target.table, err)
		}
	}
	return nil
}

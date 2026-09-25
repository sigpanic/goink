package v120

import (
	"fmt"
	"log/slog"

	"gorm.io/gorm"
)

type columnRename struct {
	table     string
	oldColumn string
	newColumn string
}

// chapterNumberColumnRenames 是 Go 版曾将章节号保存在 *_id 误名列中的完整清单。
var chapterNumberColumnRenames = []columnRename{
	{"time_entries", "source_chapter_id", "source_chapter"},
	{"time_entries", "resolved_chapter_id", "resolved_chapter"},
	{"writing_log", "chapter_id", "chapter_number"},
	{"character_relations", "chapter_id", "chapter_number"},
}

// migrateRenameChapterNumberColumns 必须在当前 model AutoMigrate 前运行；否则
// GORM 会先添加新列，使 SQLite RENAME COLUMN 因重复列名失败。
func migrateRenameChapterNumberColumns(db *gorm.DB, _ *slog.Logger) error {
	if !db.Migrator().HasTable("chapters") || !db.Migrator().HasColumn("chapters", "chapter_number") {
		return nil
	}
	for _, rename := range chapterNumberColumnRenames {
		if !db.Migrator().HasTable(rename.table) || !db.Migrator().HasColumn(rename.table, rename.oldColumn) {
			continue
		}
		if db.Migrator().HasColumn(rename.table, rename.newColumn) {
			return fmt.Errorf("检测到无法安全迁移的混合 schema：%s 同时存在 %s 和 %s；已停止迁移，请先手工备份数据库后处理", rename.table, rename.oldColumn, rename.newColumn)
		}
		query := fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO %s", rename.table, rename.oldColumn, rename.newColumn)
		if err := db.Exec(query).Error; err != nil {
			return fmt.Errorf("rename %s.%s to %s: %w", rename.table, rename.oldColumn, rename.newColumn, err)
		}
	}
	return nil
}

// migrateDropNovelDirPath 清理从未被读取的旧 novels.dir_path 列。它不依赖
// 历史章节号列，故可在当前 model AutoMigrate 后安全运行。
func migrateDropNovelDirPath(db *gorm.DB, _ *slog.Logger) error {
	if !db.Migrator().HasTable("novels") || !db.Migrator().HasColumn("novels", "dir_path") {
		return nil
	}
	if err := db.Exec("ALTER TABLE novels DROP COLUMN dir_path").Error; err != nil {
		return fmt.Errorf("drop novels.dir_path: %w", err)
	}
	return nil
}

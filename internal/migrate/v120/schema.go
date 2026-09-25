package v120

import (
	"fmt"

	"gorm.io/gorm"
)

// needsMigration 只识别没有 migrate_state 的既有数据库。章节表仍保留
// chapter_number 是历史章节号 schema 的必要边界：最终 id 化 schema 中同名的
// *_id 列已经恢复为真正的稳定 ID，绝不能再次改名。
func needsMigration(db *gorm.DB) (bool, error) {
	hasLegacyChapterSchema := db.Migrator().HasTable("chapters") &&
		db.Migrator().HasColumn("chapters", "chapter_number")
	hasDirPath := db.Migrator().HasTable("novels") && db.Migrator().HasColumn("novels", "dir_path")
	if !hasLegacyChapterSchema {
		return hasDirPath, nil
	}

	hasLegacyRenames := false
	for _, rename := range chapterNumberColumnRenames {
		if !db.Migrator().HasTable(rename.table) || !db.Migrator().HasColumn(rename.table, rename.oldColumn) {
			continue
		}
		if db.Migrator().HasColumn(rename.table, rename.newColumn) {
			return false, fmt.Errorf("检测到无法安全迁移的混合 schema：%s 同时存在 %s 和 %s；已停止迁移，请先手工备份数据库后处理", rename.table, rename.oldColumn, rename.newColumn)
		}
		hasLegacyRenames = true
	}
	return hasLegacyRenames || hasDirPath, nil
}

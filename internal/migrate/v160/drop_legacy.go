package v160

import (
	"fmt"
	"log/slog"

	"gorm.io/gorm"
)

// legacyColumns 是 v1.6.0 前以章节号表达的持久化列。
// target_chapter 虽然不是既有章节的引用，但已在 reading-targets 步骤复制到
// target_reading_number，因而也必须在本步骤清理，避免两个字段继续表达同一语义。
var legacyColumns = []struct {
	table  string
	column string
}{
	{"chapters", "chapter_number"},
	{"time_entries", "target_chapter"},
	{"time_entries", "source_chapter"},
	{"time_entries", "resolved_chapter"},
	{"arc_nodes", "target_chapter"},
	{"arc_nodes", "actual_chapter"},
	{"reader_perspectives", "planted_chapter"},
	{"reader_perspectives", "revealed_chapter"},
	{"writing_log", "chapter_number"},
	{"character_relations", "chapter_number"},
}

// migrateDropLegacy 删除已完成 id 化的旧章节号列。
//
// 本步骤必须位于 crossref-data、rename-files、reading-targets 之后：前三步仍需读取
// 旧列来反查稳定 ID、初始化排序和保留未来阅读位置。每列独立检查，迁移状态丢失或
// 中断后重跑时已删除的列会直接跳过。
func migrateDropLegacy(db *gorm.DB, log *slog.Logger) error {
	for _, legacy := range legacyColumns {
		if !db.Migrator().HasTable(legacy.table) || !db.Migrator().HasColumn(legacy.table, legacy.column) {
			continue
		}
		if err := dropIndexesForColumn(db, legacy.table, legacy.column); err != nil {
			return fmt.Errorf("migrate v160: 删除 %s.%s 的索引: %w", legacy.table, legacy.column, err)
		}
		// SQLite 的 GORM Migrator 在以字符串表名 DropColumn 时会因缺少 model schema
		// 而 panic。Goink 的持久化数据库固定为 SQLite，直接使用其原生 DDL；table 和
		// column 均来自上方固定清单，不含外部输入。
		query := fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", legacy.table, legacy.column)
		if err := db.Exec(query).Error; err != nil {
			return fmt.Errorf("migrate v160: 删除旧列 %s.%s: %w", legacy.table, legacy.column, err)
		}
		log.Info("migrate v160: 已删除旧章节号列", "table", legacy.table, "column", legacy.column)
	}
	return nil
}

// dropIndexesForColumn 先删除所有包含该列的索引。不能只按固定名称删除：SQLite
// RENAME COLUMN 后会保留原索引名，例如 writing_log 的 chapter_id 索引改名后仍可能
// 索引 chapter_number。
func dropIndexesForColumn(db *gorm.DB, table, column string) error {
	indexes, err := db.Migrator().GetIndexes(table)
	if err != nil {
		return fmt.Errorf("列举索引: %w", err)
	}
	for _, index := range indexes {
		for _, indexedColumn := range index.Columns() {
			if indexedColumn != column {
				continue
			}
			if err := db.Migrator().DropIndex(table, index.Name()); err != nil {
				return fmt.Errorf("删除索引 %s: %w", index.Name(), err)
			}
			break
		}
	}
	return nil
}

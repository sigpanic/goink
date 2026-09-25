package v160

import (
	"fmt"
	"log/slog"

	"gorm.io/gorm"
)

// colPair 描述一张表里"旧章节号列 → 新 chapter_id 列"的一对映射。
type colPair struct {
	numCol string // 旧章节号列，如 target_chapter
	idCol  string // 新 chapters.id 外键列，如 target_chapter_id
}

// tableCols 描述一张交叉引用表要重写的列对。
type tableCols struct {
	table string
	pairs []colPair
}

// crossRefTables 是 commit 1.5 需要 num→id 重写的 5 张交叉引用表。
// vec_novel_{id} 不在此列：commit 1.3 已用 DROP 重建 + RebuildAll 覆盖度检查处理。
var crossRefTables = []tableCols{
	{table: "time_entries", pairs: []colPair{
		{"source_chapter", "source_chapter_id"},
		{"resolved_chapter", "resolved_chapter_id"},
	}},
	{table: "arc_nodes", pairs: []colPair{
		{"actual_chapter", "actual_chapter_id"},
	}},
	{table: "reader_perspectives", pairs: []colPair{
		{"planted_chapter", "planted_chapter_id"},
		{"revealed_chapter", "revealed_chapter_id"},
	}},
	{table: "writing_log", pairs: []colPair{
		{"chapter_number", "chapter_id"},
	}},
	{table: "character_relations", pairs: []colPair{
		{"chapter_number", "chapter_id"},
	}},
}

// migrateCrossRefData 重写 5 张交叉引用表的章节引用：按 (novel_id, 旧 num 列) 反查
// chapters.id，写入 commit 1.2 新增的 chapter_id 列。幂等可重入：
//
//   - 只处理 id 列仍为 NULL 且 num > 0 的行（已填充的跳过；num <= 0 语义为"未定义/未回收"，
//     保持 NULL 不再处理）
//   - 反查失败（该章节号不存在，如 LLM 估算的未来章 / 已删除章节）→ id 保持 NULL + 汇总告警
//   - 列不存在（新库 / 尚未加列）→ 跳过该列
func migrateCrossRefData(db *gorm.DB, log *slog.Logger) error {
	if !db.Migrator().HasTable("chapters") || !db.Migrator().HasColumn("chapters", "chapter_number") {
		return nil
	}
	// chapters 反查映射：(novel_id, chapter_number) → id。（novel_id, chapter_number）有唯一索引，无歧义。
	type chKey struct {
		novelID int64
		num     int
	}
	chapterIDByKey := make(map[chKey]int64)
	{
		type chRow struct {
			ID            int64
			NovelID       int64
			ChapterNumber int
		}
		var rows []chRow
		if err := db.Table("chapters").Select("id, novel_id, chapter_number").Scan(&rows).Error; err != nil {
			return fmt.Errorf("migrate v160: 读取 chapters: %w", err)
		}
		for _, r := range rows {
			chapterIDByKey[chKey{novelID: r.NovelID, num: r.ChapterNumber}] = r.ID
		}
	}

	filled := 0 // 所有表已填充的总行数（warn 按表/列汇总）
	for _, tc := range crossRefTables {
		if !db.Migrator().HasTable(tc.table) {
			continue // 该表不存在（新库）→ 跳过
		}
		for _, p := range tc.pairs {
			// 新 id 列或旧 num 列不存在时跳过该列：新库不会有旧列；迁移状态
			// 丢失后重跑也不能查询已删除的旧列。
			if !db.Migrator().HasColumn(tc.table, p.idCol) || !db.Migrator().HasColumn(tc.table, p.numCol) {
				continue
			}
			// 待迁移行：id 列仍为 NULL 且 num > 0
			type row struct {
				ID      int64
				NovelID int64
				Num     int
			}
			var rows []row
			q := fmt.Sprintf("SELECT id, novel_id, %s AS num FROM %s WHERE %s IS NULL AND %s > 0",
				p.numCol, tc.table, p.idCol, p.numCol)
			if err := db.Raw(q).Scan(&rows).Error; err != nil {
				return fmt.Errorf("migrate v160: 读取 %s.%s: %w", tc.table, p.numCol, err)
			}
			if len(rows) == 0 {
				continue
			}
			missing := 0
			upd := fmt.Sprintf("UPDATE %s SET %s = ? WHERE id = ?", tc.table, p.idCol)
			for _, r := range rows {
				id, ok := chapterIDByKey[chKey{novelID: r.NovelID, num: r.Num}]
				if !ok {
					missing++ // 反查失败：保持 NULL（孤儿引用）
					continue
				}
				if err := db.Exec(upd, id, r.ID).Error; err != nil {
					return fmt.Errorf("migrate v160: 更新 %s.%s: %w", tc.table, p.idCol, err)
				}
				filled++
			}
			if missing > 0 {
				log.Warn("migrate v160: 章节号反查失败，id 保持 NULL（孤儿引用）",
					"table", tc.table, "num_col", p.numCol, "rows", missing)
			}
		}
	}
	if filled > 0 {
		log.Info("migrate v160: 交叉引用 num→id 重写完成", "filled", filled)
	}
	return nil
}

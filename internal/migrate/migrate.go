package migrate

import (
	"fmt"
	"log/slog"

	"gorm.io/gorm"

	"novel/internal/chapter"
	"novel/internal/character"
	"novel/internal/config"
	"novel/internal/location"
	"novel/internal/novel"
	"novel/internal/reader"
	"novel/internal/rollback"
	"novel/internal/session"
	"novel/internal/storage"
	"novel/internal/storyarc"
	"novel/internal/style"
	"novel/internal/timeline"
	"novel/internal/writing"
)

// Run 自动创建/更新全部数据表，幂等安全。
func Run(db *gorm.DB, log *slog.Logger) error {
	// 移除旧 novels 表的 dir_path 列（该字段从未被读取过）。幂等：列不存在时报错忽略。
	if err := db.Exec("ALTER TABLE novels DROP COLUMN dir_path").Error; err != nil {
		log.Warn("迁移：删除 novels.dir_path 列失败（如列已不存在则无害）", "err", err)
	}

	models := []any{
		&config.AppSettings{},
		&novel.Novel{},
		&novel.PreferenceItem{},
		&chapter.Chapter{},
		&character.Character{},
		&character.CharacterRelation{},
		&timeline.TimelineEntry{},
		&storyarc.StoryArc{},
		&storyarc.ArcNode{},
		&location.Location{},
		&location.LocationRelation{},
		&reader.ReaderPerspective{},
		&session.Session{},
		&session.Message{},
		&storage.OperationLogRecord{},
		&rollback.TurnCommit{},
		&style.Sample{},
		&writing.WritingLog{},
	}

	for _, m := range models {
		if err := db.AutoMigrate(m); err != nil {
			return fmt.Errorf("migrate: %T: %w", m, err)
		}
	}

	// ── 字段改名迁移 ──
	// 背景：Python→Go 迁移时，timeline/writing/character 的章节引用字段
	// 语义从 chapters.id 改成章节号，但字段名沿用了 Python 版的 _id 后缀。
	// 此处统一改名纠正。幂等：列已改名或新库直接建新列名时，RENAME 报错忽略。
	renameColumns := []struct {
		table, oldCol, newCol string
	}{
		{"time_entries", "source_chapter_id", "source_chapter"},
		{"time_entries", "resolved_chapter_id", "resolved_chapter"},
		{"writing_log", "chapter_id", "chapter_number"},
		{"character_relations", "chapter_id", "chapter_number"},
	}
	for _, r := range renameColumns {
		sql := fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO %s", r.table, r.oldCol, r.newCol)
		if err := db.Exec(sql).Error; err != nil {
			log.Warn("迁移：字段改名失败（如列已改名或不存在则无害）", "table", r.table, "old", r.oldCol, "new", r.newCol, "err", err)
		}
	}

	log.Info("数据库迁移完成", "tables", len(models))
	return nil
}

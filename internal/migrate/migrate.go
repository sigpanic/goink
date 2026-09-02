package migrate

import (
	"fmt"
	"log/slog"

	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/chapter"
	"github.com/sigpanic/goink/internal/character"
	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/location"
	"github.com/sigpanic/goink/internal/novel"
	"github.com/sigpanic/goink/internal/preference"
	"github.com/sigpanic/goink/internal/reader"
	"github.com/sigpanic/goink/internal/rollback"
	"github.com/sigpanic/goink/internal/session"
	"github.com/sigpanic/goink/internal/setting"
	"github.com/sigpanic/goink/internal/storage"
	"github.com/sigpanic/goink/internal/storyarc"
	"github.com/sigpanic/goink/internal/style"
	"github.com/sigpanic/goink/internal/timeline"
	"github.com/sigpanic/goink/internal/volume"
	"github.com/sigpanic/goink/internal/writing"
)

// Run 自动创建/更新全部数据表，幂等安全。
func Run(db *gorm.DB, log *slog.Logger) error {
	// 移除旧 novels 表的 dir_path 列（该字段从未被读取过）。幂等：列不存在时报错忽略。
	if err := db.Exec("ALTER TABLE novels DROP COLUMN dir_path").Error; err != nil {
		log.Warn("迁移：删除 novels.dir_path 列失败（如列已不存在则无害）", "err", err)
	}

	// ── 字段改名迁移 ──
	// 背景：Python→Go 迁移时，timeline/writing/character 的章节引用字段
	// 语义从 chapters.id 改成章节号，但字段名沿用了 Python 版的 _id 后缀。
	// 此处统一改名纠正。
	//
	// 顺序：必须在 AutoMigrate 之前执行。否则 AutoMigrate 会为 model 缺失的新列名
	// 自动加空列，导致 RENAME 因 duplicate column name 失败。
	//
	// 幂等：旧列不存在（已迁移/新库）→ 跳过；表不存在（新库首次启动）→ 跳过交给 AutoMigrate 建表。
	renameColumns := []struct {
		table, oldCol, newCol string
	}{
		{"time_entries", "source_chapter_id", "source_chapter"},
		{"time_entries", "resolved_chapter_id", "resolved_chapter"},
		{"writing_log", "chapter_id", "chapter_number"},
		{"character_relations", "chapter_id", "chapter_number"},
	}
	for _, r := range renameColumns {
		// 表不存在（新库首次启动）→ 跳过，交给 AutoMigrate 建表
		if !db.Migrator().HasTable(r.table) {
			continue
		}
		colTypes, err := db.Migrator().ColumnTypes(r.table)
		if err != nil {
			return fmt.Errorf("migrate inspect %s columns: %w", r.table, err)
		}
		hasOld, hasNew := false, false
		for _, ct := range colTypes {
			switch ct.Name() {
			case r.oldCol:
				hasOld = true
			case r.newCol:
				hasNew = true
			}
		}
		// 旧列不存在（已迁移或新库）→ 跳过
		if !hasOld {
			continue
		}
		// 新旧列同时存在 → 异常状态，跳过避免冲突
		if hasNew {
			log.Warn("迁移：字段改名跳过（新旧列同时存在，异常状态）", "table", r.table, "old", r.oldCol, "new", r.newCol)
			continue
		}
		// 正常路径：旧列存在 + 新列不存在 → RENAME
		sql := fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO %s", r.table, r.oldCol, r.newCol)
		if err := db.Exec(sql).Error; err != nil {
			return fmt.Errorf("migrate rename %s.%s→%s: %w", r.table, r.oldCol, r.newCol, err)
		}
	}

	models := []any{
		&config.AppSettings{},
		&novel.Novel{},
		&preference.PreferenceItem{},
		&setting.SettingItem{},
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
		&volume.Volume{},
		&MigrateState{},
	}

	for _, m := range models {
		if err := db.AutoMigrate(m); err != nil {
			return fmt.Errorf("migrate: %T: %w", m, err)
		}
	}

	log.Info("数据库迁移完成", "tables", len(models))
	return nil
}

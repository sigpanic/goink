package migrate

import (
	"fmt"
	"log/slog"
	"strings"

	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/chapter"
	"github.com/sigpanic/goink/internal/character"
	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/location"
	"github.com/sigpanic/goink/internal/migrate/engine"
	_ "github.com/sigpanic/goink/internal/migrate/v120" // blank：触发 v120.init 注册，Run 无需感知具体迁移
	_ "github.com/sigpanic/goink/internal/migrate/v160" // blank：触发 v160.init 注册，Run 无需感知具体迁移
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

// Run 自动创建/更新全部数据表，幂等安全。作为外部入口，遍历 engine.Registry
// 执行所有已注册迁移（备份 + 步骤），不感知具体迁移。
func Run(db *gorm.DB, log *slog.Logger) error {
	// 只能在建任何业务表前识别新库。任何应用表残留都说明这不是新库，不能直接
	// 跳过历史迁移；migrate_state 和 SQLite 内部表不计入业务表。
	isNewDatabase, err := isNewDatabase(db)
	if err != nil {
		return fmt.Errorf("migrate: 检查数据库是否为新库: %w", err)
	}

	// 0. 先建 migrate_state 表：backup 的"本迁移组 done 判断"依赖它存在；
	//    只建这一张表，不触发业务表 schema 变化，保证后续计划仍面对原始 schema。
	if err := db.AutoMigrate(&engine.MigrateState{}); err != nil {
		return fmt.Errorf("migrate: migrate_state 表: %w", err)
	}

	plans, err := engine.BuildPlans(db, log, isNewDatabase, engine.Registry)
	if err != nil {
		return fmt.Errorf("migrate: 生成迁移计划: %w", err)
	}
	// 对无需执行的迁移，计划一经原始 schema 判定便立即登记 done。后续的
	// AutoMigrate 可能暂时制造新旧列共存；必须在此之前持久化判定结果，才能让
	// 中断后的重启依赖 migrate_state 而非面对中间 schema 重新探测。
	if err := markPlannedDone(db, log, plans); err != nil {
		return err
	}

	// 备份和前置步骤均只能针对计划执行的迁移。由 plan 决定是否执行，避免
	// 新库、最终 schema 或状态丢失的库误触历史 DDL。
	for _, plan := range plans {
		if plan.Action != engine.PlanRun || !plan.Migration.Destructive {
			continue
		}
		if err := backupBeforeMigrate(db, log, plan.Migration.Name); err != nil {
			return fmt.Errorf("migrate: 破坏性迁移前备份失败，迁移未执行 (%s): %w", plan.Migration.Name, err)
		}
	}
	for _, plan := range plans {
		if plan.Action != engine.PlanRun {
			continue
		}
		if err := engine.RunSteps(db, log, plan.Migration.Name, plan.Migration.PreSchemaSteps); err != nil {
			return err
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
		&engine.MigrateState{},
	}

	for _, m := range models {
		if err := db.AutoMigrate(m); err != nil {
			return fmt.Errorf("migrate: %T: %w", m, err)
		}
	}

	// 后置步骤依赖当前 model 已补齐的列。无需执行的 migration 已在任何 schema
	// 变化前登记完成，避免中断后把当前 migration 的中间 schema 误判为历史 schema。
	for _, plan := range plans {
		if plan.Action == engine.PlanRun {
			log.Info("执行迁移", "migration", plan.Migration.Name, "description", plan.Migration.Description)
			if err := engine.RunSteps(db, log, plan.Migration.Name, plan.Migration.PostSchemaSteps); err != nil {
				return err
			}
		}
	}

	// SQLite 的 RENAME COLUMN 会保留旧索引名。例如历史 writing_log.chapter_id
	// 改为 chapter_number 后，旧索引仍叫 idx_writing_log_chapter_id；首次 AutoMigrate
	// 因同名不补建当前 chapter_id 的索引，drop-legacy 删除旧列时便会一并删掉它。
	// 所有迁移步骤完成后再次按当前 model 对齐，补回这类被历史 DDL 遮蔽或删除的索引。
	if err := db.AutoMigrate(models...); err != nil {
		return fmt.Errorf("migrate: 迁移后 schema 对齐: %w", err)
	}

	log.Info("数据库迁移完成", "tables", len(models))
	return nil
}

// markPlannedDone 将原始 schema 判定为无需执行的迁移立即落盘。每个 migration
// 由 MarkStepsDone 在单独事务中写入全部 step，避免留下半组状态。
func markPlannedDone(db *gorm.DB, log *slog.Logger, plans []engine.Plan) error {
	for _, plan := range plans {
		if plan.Action != engine.PlanMarkDone {
			continue
		}
		if err := engine.MarkStepsDone(db, log, plan.Migration.Name, plan.Migration.AllSteps()); err != nil {
			return fmt.Errorf("migrate: 登记无需执行的迁移 %s: %w", plan.Migration.Name, err)
		}
	}
	return nil
}

// isNewDatabase 仅在数据库没有任何应用业务表时返回 true。它必须在创建
// migrate_state 前调用，保证新库可作为迁移计划的唯一短路入口。
func isNewDatabase(db *gorm.DB) (bool, error) {
	tables, err := db.Migrator().GetTables()
	if err != nil {
		return false, err
	}
	for _, table := range tables {
		if table == "migrate_state" || strings.HasPrefix(table, "sqlite_") {
			continue
		}
		return false, nil
	}
	return true, nil
}

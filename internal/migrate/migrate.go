package migrate

import (
	"fmt"
	"log/slog"
	"strings"

	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/migrate/engine"
	"github.com/sigpanic/goink/internal/migrate/v120"
	"github.com/sigpanic/goink/internal/migrate/v160"
	"github.com/sigpanic/goink/internal/migrate/v161"
)

// 必须显式固定版本顺序；包的 init 顺序会受依赖关系影响，不能用来决定迁移顺序。
func init() {
	v120.Register()
	v160.Register()
	v161.Register()
}

// Run 自动创建/更新全部数据表，幂等安全。作为外部入口，遍历 engine.Registry
// 执行所有已注册迁移（备份 + 步骤），不感知具体迁移。
func Run(db *gorm.DB, log *slog.Logger) error {
	// 只能在建任何业务表前识别新库。任何应用表残留都说明这不是新库，不能直接
	// 跳过历史迁移；migrate_state 和 SQLite 内部表不计入业务表。
	isNewDatabase, err := isNewDatabase(db)
	if err != nil {
		return fmt.Errorf("migrate: 检查数据库是否为新库: %w", err)
	}

	// 0. 先建 migrate_state 表：迁移完成判断和备份预检依赖它存在；
	//    只建这一张表，不触发业务表 schema 变化，保证后续计划仍面对原始 schema。
	if err := db.AutoMigrate(&engine.MigrateState{}); err != nil {
		return fmt.Errorf("migrate: migrate_state 表: %w", err)
	}
	// 在任何业务 schema 或文件变化前检查全部旧记录；不接管旧全局流程的中断。
	if err := engine.PrepareSequential(db, log, engine.Registry); err != nil {
		return fmt.Errorf("migrate: 初始化逐组迁移: %w", err)
	}

	// 本轮若可能执行破坏性迁移，在任何业务变化前准备唯一的起始快照。
	if err := prepareUpgradeBackup(db, log, isNewDatabase, engine.Registry); err != nil {
		return fmt.Errorf("migrate: 升级前备份准备失败，迁移未执行: %w", err)
	}
	for _, migration := range engine.Registry {
		// 前一组可能创建后续迁移的旧表；计划必须在本组执行前重新判断。
		plans, err := engine.BuildPlans(db, log, isNewDatabase, []engine.Migration{migration})
		if err != nil {
			return fmt.Errorf("migrate: 生成迁移计划: %w", err)
		}
		switch plans[0].Action {
		case engine.PlanSkip:
			continue
		case engine.PlanMarkDone:
			// 先持久化无需迁移的判定，后续组的 AutoMigrate 才不会让重启误判。
			if err := engine.MarkStepsDone(db, log, migration.Name, migration.AllSteps()); err != nil {
				return err
			}
		case engine.PlanRun:
			log.Info("执行迁移", "migration", migration.Name, "description", migration.Description)
			if err := engine.RunSteps(db, log, migration.Name, migration.AllSteps()); err != nil {
				return err
			}
		}
	}
	models := Models()

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

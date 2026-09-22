package engine

import (
	"fmt"
	"log/slog"

	"gorm.io/gorm"
)

// Migration 描述一次迁移。PreSchemaSteps 必须在当前 model 的 AutoMigrate 前运行；
// PostSchemaSteps 则在其后运行。两类步骤均按 Registry 注册顺序执行。
type Migration struct {
	Name        string // 迁移标识，如 "v1.6.0-chapter-id-refactor"，写入 migrate_state.migration
	Description string // 迁移目的简述（本次迁移为了什么、主要做什么），打印进日志便于排查
	Destructive bool   // 是否破坏性迁移（删列/rename/数据重写等）：true 时 migrate.Run 会在执行前自动备份全量数据

	// NeedsMigration 仅在既有数据库缺少本 migration 的 migrate_state 时调用，识别
	// 是否仍处于该 migration 的旧 schema。应返回 error 拒绝无法安全识别的混合 schema。
	NeedsMigration  func(db *gorm.DB) (bool, error)
	PreSchemaSteps  []Step // 当前 model AutoMigrate 前执行，例如历史字段改名
	PostSchemaSteps []Step // 当前 model AutoMigrate 后执行，例如数据重写、文件迁移、删旧列
}

// AllSteps 按迁移的完整执行顺序返回所有步骤。
func (m Migration) AllSteps() []Step {
	steps := make([]Step, 0, len(m.PreSchemaSteps)+len(m.PostSchemaSteps))
	steps = append(steps, m.PreSchemaSteps...)
	steps = append(steps, m.PostSchemaSteps...)
	return steps
}

// PlanAction 是一次启动中对一个 migration 的执行计划。
type PlanAction uint8

const (
	PlanSkip     PlanAction = iota // 所有已知步骤均 done
	PlanRun                        // 运行步骤，或从未完成状态恢复
	PlanMarkDone                   // 当前 schema 无需迁移，仅登记步骤完成
)

// Plan 是根据启动前 schema 和 migrate_state 得出的单次迁移计划。
type Plan struct {
	Migration Migration
	Action    PlanAction
}

// BuildPlans 在任何业务 model AutoMigrate 前为全部注册迁移生成计划。
// 对于已有状态的迁移，migrate_state 是唯一真相；只有完全没有状态记录的既有库才
// 使用 Migration.NeedsMigration 识别历史 schema。
func BuildPlans(db *gorm.DB, log *slog.Logger, isNewDatabase bool, registry []Migration) ([]Plan, error) {
	plans := make([]Plan, 0, len(registry))
	for _, migration := range registry {
		steps := migration.AllSteps()
		if err := validateSteps(migration.Name, steps); err != nil {
			return nil, err
		}

		var states []MigrateState
		if err := db.Where("migration = ?", migration.Name).Find(&states).Error; err != nil {
			return nil, fmt.Errorf("read migration state %s: %w", migration.Name, err)
		}
		if len(states) > 0 {
			allDone, unknownSteps := allStepsDone(states, steps)
			if len(unknownSteps) > 0 {
				log.Warn("迁移状态含已废弃或未知步骤，已忽略",
					"migration", migration.Name, "steps", unknownSteps)
			}
			if allDone {
				plans = append(plans, Plan{Migration: migration, Action: PlanSkip})
			} else {
				plans = append(plans, Plan{Migration: migration, Action: PlanRun})
			}
			continue
		}

		if isNewDatabase {
			plans = append(plans, Plan{Migration: migration, Action: PlanMarkDone})
			continue
		}
		if migration.NeedsMigration == nil {
			return nil, fmt.Errorf("migration %s has no schema detector", migration.Name)
		}
		needsMigration, err := migration.NeedsMigration(db)
		if err != nil {
			return nil, fmt.Errorf("detect migration %s: %w", migration.Name, err)
		}
		if needsMigration {
			plans = append(plans, Plan{Migration: migration, Action: PlanRun})
		} else {
			plans = append(plans, Plan{Migration: migration, Action: PlanMarkDone})
		}
	}
	return plans, nil
}

func validateSteps(migration string, steps []Step) error {
	keys := make(map[string]struct{}, len(steps))
	for _, step := range steps {
		if step.Key() == "" {
			return fmt.Errorf("migration %s has empty step key", migration)
		}
		if _, exists := keys[step.Key()]; exists {
			return fmt.Errorf("migration %s has duplicate step key %q", migration, step.Key())
		}
		keys[step.Key()] = struct{}{}
	}
	return nil
}

func allStepsDone(states []MigrateState, steps []Step) (bool, []string) {
	expected := make(map[string]struct{}, len(steps))
	for _, step := range steps {
		expected[step.Key()] = struct{}{}
	}
	byKey := make(map[string]string, len(states))
	unknownSteps := make([]string, 0)
	for _, state := range states {
		if _, ok := expected[state.Step]; !ok {
			unknownSteps = append(unknownSteps, state.Step)
			continue
		}
		byKey[state.Step] = state.Status
	}
	for _, step := range steps {
		if byKey[step.Key()] != "done" {
			return false, unknownSteps
		}
	}
	return true, unknownSteps
}

// Registry 聚合所有已注册迁移，注册顺序 = 执行顺序。
// 具体迁移包（如 internal/migrate/v160）在 init() 里调用 Register 自注册；
// migrate.Run 遍历本注册表执行，对具体迁移无感知。
var Registry []Migration

// Register 注册一次迁移，供具体迁移包的 init() 调用。
func Register(m Migration) {
	Registry = append(Registry, m)
}

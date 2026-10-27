package engine

import (
	"errors"
	"fmt"
	"log/slog"

	"gorm.io/gorm"
)

// Migration 描述一次迁移；数据库阶段按 pre → 本组冻结 schema → post 执行，
// 全部数据库迁移及当前 schema 对齐成功后，再按注册顺序执行 PostDBMigrationSteps。
type Migration struct {
	Name        string // 迁移标识，如 "v1.6.0-chapter-id-refactor"，写入 migrate_state.migration
	Description string // 迁移目的简述（本次迁移为了什么、主要做什么），打印进日志便于排查
	Destructive bool   // 是否破坏性数据库迁移（删列/rename/数据重写等）：true 时 migrate.Run 会在执行前自动备份全量数据

	// NeedsMigration 仅在既有数据库缺少本 migration 的数据库步骤状态时调用，识别
	// 是否仍处于该 migration 的旧 schema。应返回 error 拒绝无法安全识别的混合 schema。
	NeedsMigration  func(db *gorm.DB) (bool, error)
	PreSchemaSteps  []Step // 本组 schema 对齐前执行，例如历史字段改名
	SchemaStep      Step   // 使用冻结模型对齐 schema，与其他步骤共用状态记录
	PostSchemaSteps []Step // 本组 schema 对齐后执行，例如数据重写、文件迁移、删旧列

	// PostDBMigrationSteps 用于配置文件等不依赖历史 schema 的迁移，新库也会执行。
	// 与数据库步骤共用 migrate_state，Key 不可重复；外部资源的备份由步骤负责。
	PostDBMigrationSteps []Step
}

// DatabaseSteps 返回本组数据库阶段的步骤，不包含全部数据库迁移后的步骤。
func (m Migration) DatabaseSteps() []Step {
	steps := make([]Step, 0, len(m.PreSchemaSteps)+1+len(m.PostSchemaSteps))
	steps = append(steps, m.PreSchemaSteps...)
	if m.SchemaStep != nil {
		steps = append(steps, m.SchemaStep)
	}
	steps = append(steps, m.PostSchemaSteps...)
	return steps
}

// AllSteps 返回本组两个阶段的所有步骤，供校验和完整状态检查使用。
func (m Migration) AllSteps() []Step {
	return append(m.DatabaseSteps(), m.PostDBMigrationSteps...)
}

// PlanAction 是一次启动中对一个 migration 数据库阶段的执行计划。
type PlanAction uint8

const (
	PlanSkip     PlanAction = iota // 所有已知步骤均 done
	PlanRun                        // 运行步骤，或从未完成状态恢复
	PlanMarkDone                   // 当前 schema 无需迁移，仅登记步骤完成
)

// Plan 是根据执行前 schema 和 migrate_state 得出的单次迁移计划。
type Plan struct {
	Migration Migration
	Action    PlanAction
}

// BuildPlans 根据当前 schema 生成数据库阶段计划。逐组执行时必须在前一组完成后重新判断后一组，
// 因为前一组的冻结模型可能创建后一组需要处理的旧表或旧列。
// 对于已有数据库步骤状态的迁移，migrate_state 是唯一真相；只有没有该阶段状态的既有库才
// 使用 Migration.NeedsMigration 识别历史 schema。
func BuildPlans(db *gorm.DB, log *slog.Logger, isNewDatabase bool, registry []Migration) ([]Plan, error) {
	plans := make([]Plan, 0, len(registry))
	for _, migration := range registry {
		if err := validateSteps(migration.Name, migration.AllSteps()); err != nil {
			return nil, err
		}
		steps := migration.DatabaseSteps()
		if len(steps) == 0 {
			plans = append(plans, Plan{Migration: migration, Action: PlanSkip})
			continue
		}

		var states []MigrateState
		if err := db.Where("migration = ?", migration.Name).Find(&states).Error; err != nil {
			return nil, fmt.Errorf("read migration state %s: %w", migration.Name, err)
		}
		states = migration.databaseStates(states)
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

const (
	schedulerMigration = "_engine"
	sequentialStep     = "sequential-schema"
)

// ErrLegacyMigrationIncomplete 表示旧全局流程尚未完成，须由旧版本先行恢复。
var ErrLegacyMigrationIncomplete = errors.New("检测到旧版未完成的迁移，请先使用原版本启动并完成迁移，再升级")

// PrepareSequential 在业务 schema 变化前原子接纳已完成的旧记录，并登记调度方式。
// 旧组只补记 schema 步骤，不重放冻结模型；任一旧组未完成时整笔登记回滚。
// 标记必须先于任何 pre 落盘，否则只有 pre 的新组中断后会与旧完成记录无法区分。
// 本入口不接管旧流程中断；未来若增加支持，应在这里处理，普通步骤不承担兼容逻辑。
func PrepareSequential(db *gorm.DB, log *slog.Logger, registry []Migration) error {
	return db.Transaction(func(tx *gorm.DB) error {
		status, err := stepStatus(tx, schedulerMigration, sequentialStep)
		if err != nil {
			return err
		}
		if status == "done" {
			return nil
		}
		if status != "" {
			return fmt.Errorf("invalid migration scheduler state: %q", status)
		}
		for _, migration := range registry {
			if err := validateSteps(migration.Name, migration.AllSteps()); err != nil {
				return err
			}
			var states []MigrateState
			if err := tx.Where("migration = ?", migration.Name).Find(&states).Error; err != nil {
				return fmt.Errorf("read migration state %s: %w", migration.Name, err)
			}
			states = migration.databaseStates(states)
			if len(states) == 0 {
				continue
			}
			legacySteps := append(append([]Step{}, migration.PreSchemaSteps...), migration.PostSchemaSteps...)
			if done, _ := allStepsDone(states, legacySteps); !done {
				return fmt.Errorf("%w (%s)", ErrLegacyMigrationIncomplete, migration.Name)
			}
			if migration.SchemaStep != nil {
				// 若已有 auto 记录却没有调度标记，不能把未完成的 auto 当作旧版成功记录。
				for _, state := range states {
					if state.Step == migration.SchemaStep.Key() && state.Status != "done" {
						return fmt.Errorf("incomplete schema state without scheduler marker: %s", migration.Name)
					}
				}
				if err := markStep(tx, log, migration.Name, migration.SchemaStep.Key(), "done", nil); err != nil {
					return err
				}
			}
		}
		return markStep(tx, log, schedulerMigration, sequentialStep, "done", nil)
	})
}

// databaseStates 排除已知的第二阶段记录，保留未知旧步骤供既有接纳和告警逻辑处理。
func (m Migration) databaseStates(states []MigrateState) []MigrateState {
	postDBKeys := make(map[string]struct{}, len(m.PostDBMigrationSteps))
	for _, step := range m.PostDBMigrationSteps {
		postDBKeys[step.Key()] = struct{}{}
	}
	result := make([]MigrateState, 0, len(states))
	for _, state := range states {
		if _, postDB := postDBKeys[state.Step]; !postDB {
			result = append(result, state)
		}
	}
	return result
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
// migrate 包按版本顺序显式调用各包的 Register，避免 Go 包依赖改变 init 顺序。
var Registry []Migration

// Register 注册一次迁移，供具体迁移包的 Register 调用。
func Register(m Migration) {
	Registry = append(Registry, m)
}

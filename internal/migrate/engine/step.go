package engine

import (
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Step 描述一个迁移步骤：由 RunSteps 统一驱动。
//
// 契约：Run 内部必须幂等可重入——自身完成条件（HasColumn / IS NULL / 目标文件存在等）
// 已满足时直接返回 nil。中断后框架从"未 done"的 step 重跑，靠此契约保证安全。
//
// 设计：Step 是接口而非结构体。具体迁移包（如 internal/migrate/v160）用自己的私有
// step 类型实现它并注册进 Registry，migrate 包无需感知具体迁移。
type Step interface {
	Key() string                             // 步骤标识（迁移内序数 N-描述，如 "1-crossref-data"）
	Run(db *gorm.DB, log *slog.Logger) error // 业务逻辑，幂等可重入
}

// RunSteps 按注册表顺序驱动一次迁移的步骤（框架统一写状态）。
//
// 幂等 / 中断恢复：
//   - 已 done 的 step 跳过；未 done 的置 running → 执行 → done / failed
//   - 某 step 写 done 前中断 → 该 step 未 done → 下次启动从它重跑（step 内部幂等）
//
// 状态记录失败只 warn 不阻塞（下次启动重跑该 step，幂等无害）。
func RunSteps(db *gorm.DB, log *slog.Logger, migration string, steps []Step) error {
	if len(steps) == 0 {
		return nil
	}
	for _, s := range steps {
		if stepStatus(db, migration, s.Key()) == "done" {
			continue
		}
		log.Info("迁移步骤开始", "migration", migration, "step", s.Key())
		markStep(db, log, migration, s.Key(), "running", nil)
		if err := s.Run(db, log); err != nil {
			markStep(db, log, migration, s.Key(), "failed", err)
			return fmt.Errorf("migrate step %s: %w", s.Key(), err)
		}
		markStep(db, log, migration, s.Key(), "done", nil)
		log.Info("迁移步骤完成", "migration", migration, "step", s.Key())
	}
	return nil
}

// MarkStepsDone 原子登记无需执行的数据迁移。它与 markStep 的“状态写入失败仅告警”
// 语义不同：新库/最终 schema 必须一次性得到全部 done 或完全不写入，避免进程中断
// 后留下半组状态并在下次被误当作待恢复的旧库迁移。
func MarkStepsDone(db *gorm.DB, log *slog.Logger, migration string, steps []Step) error {
	if len(steps) == 0 {
		return nil
	}
	now := time.Now()
	if err := db.Transaction(func(tx *gorm.DB) error {
		for _, step := range steps {
			state := MigrateState{
				Migration:  migration,
				Step:       step.Key(),
				Status:     "done",
				FinishedAt: &now,
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "migration"}, {Name: "step"}},
				DoUpdates: clause.Assignments(map[string]any{
					"status":      "done",
					"finished_at": now,
					"error":       "",
					"updated_at":  now,
				}),
			}).Create(&state).Error; err != nil {
				return fmt.Errorf("mark step %s done: %w", step.Key(), err)
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("mark migration %s done: %w", migration, err)
	}
	log.Info("无需迁移，步骤已登记完成", "migration", migration)
	return nil
}

// stepStatus 读取某 step 的状态；行不存在返回 ""（视为未开始）。
func stepStatus(db *gorm.DB, migration, key string) string {
	var st MigrateState
	if err := db.Where("migration = ? AND step = ?", migration, key).First(&st).Error; err != nil {
		return ""
	}
	return st.Status
}

// markStep upsert 某 step 的状态（running / done / failed），并维护时间戳与错误信息。
func markStep(db *gorm.DB, log *slog.Logger, migration, key, status string, runErr error) {
	now := time.Now()
	var st MigrateState
	err := db.Where("migration = ? AND step = ?", migration, key).First(&st).Error
	if err != nil {
		st = MigrateState{Migration: migration, Step: key}
	}
	st.Status = status
	switch status {
	case "running":
		st.StartedAt = &now
		st.FinishedAt = nil
		st.Error = ""
	case "done":
		st.FinishedAt = &now
		st.Error = ""
	case "failed":
		st.FinishedAt = &now
	}
	if status == "failed" && runErr != nil {
		st.Error = runErr.Error()
	}
	if err := db.Save(&st).Error; err != nil {
		log.Warn("migrate_state 状态写入失败（下次启动将重跑该 step）",
			"migration", migration, "step", key, "status", status, "err", err)
	}
}

package engine

import (
	"errors"
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
// 状态读取或写入失败立即停止；running 未落盘不执行步骤，done 未落盘不进入下一步。
func RunSteps(db *gorm.DB, log *slog.Logger, migration string, steps []Step) error {
	if len(steps) == 0 {
		return nil
	}
	for _, s := range steps {
		status, err := stepStatus(db, migration, s.Key())
		if err != nil {
			return err
		}
		if status == "done" {
			continue
		}
		log.Info("迁移步骤开始", "migration", migration, "step", s.Key())
		if err := markStep(db, log, migration, s.Key(), "running", nil); err != nil {
			return err
		}
		if err := s.Run(db, log); err != nil {
			stateErr := markStep(db, log, migration, s.Key(), "failed", err)
			return fmt.Errorf("migrate step %s: %w", s.Key(), errors.Join(err, stateErr))
		}
		if err := markStep(db, log, migration, s.Key(), "done", nil); err != nil {
			return err
		}
		log.Info("迁移步骤完成", "migration", migration, "step", s.Key())
	}
	return nil
}

// MarkStepsDone 原子登记无需执行的数据迁移；全部 done 或完全不写入，避免中断
// 后留下半组状态。调用方必须在本组 schema 变化前持久化无需迁移的判定。
func MarkStepsDone(db *gorm.DB, log *slog.Logger, migration string, steps []Step) error {
	if len(steps) == 0 {
		return nil
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		for _, step := range steps {
			if err := markStep(tx, log, migration, step.Key(), "done", nil); err != nil {
				return err
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
func stepStatus(db *gorm.DB, migration, key string) (string, error) {
	var st MigrateState
	if err := db.Where("migration = ? AND step = ?", migration, key).First(&st).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		return "", fmt.Errorf("read migration state %s/%s: %w", migration, key, err)
	}
	return st.Status, nil
}

// markStep upsert 某 step 的状态（running / done / failed），并维护时间戳与错误信息。
func markStep(db *gorm.DB, log *slog.Logger, migration, key, status string, runErr error) error {
	now := time.Now()
	st := MigrateState{
		Migration: migration,
		Step:      key,
		Status:    status,
		UpdatedAt: now,
	}
	updates := map[string]any{
		"status":     status,
		"updated_at": now,
		"error":      "",
	}
	switch status {
	case "running":
		st.StartedAt = &now
		updates["started_at"] = now
		updates["finished_at"] = nil
	case "done", "failed":
		st.FinishedAt = &now
		updates["finished_at"] = now
	}
	if status == "failed" && runErr != nil {
		st.Error = runErr.Error()
		updates["error"] = st.Error
	}
	// 冲突时只更新本次状态对应的字段，保留记录创建时间和既有开始时间。
	if err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "migration"}, {Name: "step"}},
		DoUpdates: clause.Assignments(updates),
	}).Create(&st).Error; err != nil {
		log.Warn("migrate_state 状态写入失败（下次启动将重跑该 step）",
			"migration", migration, "step", key, "status", status, "err", err)
		return fmt.Errorf("record migration state %s/%s %s: %w", migration, key, status, err)
	}
	return nil
}

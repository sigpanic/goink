package engine

import (
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"
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
//   - 新库短路：chapters 表不存在（全新用户，无历史数据）→ 所有 step 直接 done
//   - 已 done 的 step 跳过；未 done 的置 running → 执行 → done / failed
//   - 某 step 写 done 前中断 → 该 step 未 done → 下次启动从它重跑（step 内部幂等）
//
// 状态记录失败只 warn 不阻塞（下次启动重跑该 step，幂等无害）。
func RunSteps(db *gorm.DB, log *slog.Logger, migration string, steps []Step) error {
	if len(steps) == 0 {
		return nil
	}
	// 新库短路：无 chapters 表 → 无历史数据，跳过全部迁移步骤
	if !db.Migrator().HasTable("chapters") {
		for _, s := range steps {
			markStep(db, log, migration, s.Key(), "done", nil)
		}
		log.Info("新库，跳过迁移步骤", "migration", migration)
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
	case "done", "failed":
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

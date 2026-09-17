package migrate

import "time"

// migrationV160 是 v1.6.0 分卷 + 章节 id 化改造的迁移标识。
// 备份/完成判断只看本组，未来新增迁移各自一行一组，互不干扰。
const migrationV160 = "v1.6.0-chapter-id-refactor"

// MigrateState 是迁移进度跟踪表，按"迁移 + step"两级记录。
//
// 设计原则：
//   - 一行 = (migration, step)。migration 标识一次大迁移（如 "v1.6.0-chapter-id-refactor"），
//     step 标识该迁移内的执行步骤（语义化 key，如 "1.5-crossref-data"，见 step.go 注册表）
//   - status 取值：running | done | failed（未开始 = 无行，stepStatus 返回空串）
//   - 幂等：迁移启动时查 migration 组内非 done 的 step，逐个执行；每步"内部执行完再写 done"
//   - 中断续跑：某 step 写 done 前中断 → 该 step 未 done → 下次从它重跑（step 内部幂等）
//   - 备份/完成判断：查 migration 组的行是否全部 done，只看本组，不看其他迁移
//   - 新用户 DB 初始化后所有 step INSERT 为 done（无需迁移）
//
// 时间戳语义：
//   - started_at：本 step 开始执行时间（nullable，未执行时为 NULL）
//   - finished_at：本 step 执行完成时间（nullable，running 时为 NULL）
//   - error：failed 时填写错误信息，便于诊断中断原因
type MigrateState struct {
	ID         int64      `gorm:"column:id;primaryKey;autoIncrement"                      json:"id"`
	Migration  string     `gorm:"column:migration;not null;uniqueIndex:uk_migration_step" json:"migration"`   // 迁移标识，如 "v1.6.0-chapter-id-refactor"
	Step       string     `gorm:"column:step;not null;uniqueIndex:uk_migration_step"      json:"step"`        // 迁移内 step 标识（语义化 key，如 "1.5-crossref-data"）
	Status     string     `gorm:"column:status;not null;index"                            json:"status"`      // "running" | "done" | "failed"（未开始无行）
	StartedAt  *time.Time `gorm:"column:started_at"                                       json:"started_at"`  // 本 step 开始时间，nullable
	FinishedAt *time.Time `gorm:"column:finished_at"                                      json:"finished_at"` // 本 step 完成时间，nullable
	Error      string     `gorm:"column:error"                                            json:"error"`       // failed 时的错误信息
	CreatedAt  time.Time  `gorm:"column:created_at;autoCreateTime"                        json:"created_at"`
	UpdatedAt  time.Time  `gorm:"column:updated_at;autoUpdateTime"                        json:"updated_at"`
}

// TableName 指定 GORM 表名。
func (MigrateState) TableName() string { return "migrate_state" }

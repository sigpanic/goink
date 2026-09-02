package migrate

import "time"

// MigrateState 是 v1.5.0 分卷+章节 id 化改造的迁移进度跟踪表。
//
// 设计原则：
//   - 每个 step 一行，step 标识形如 "1.1".."1.7"，对应 commit-roadmap.md 中的 7 个迁移 commit
//   - status 取值：pending | running | done | failed
//   - 应用启动时 Run() 检查所有 step 是否 done，全部 done 跳过迁移；否则继续未完成 step
//   - 新用户 DB 初始化后所有 step INSERT 为 done（无需迁移）
//   - 幂等：迁移代码 WHERE status != 'done' 才执行；失败保留 failed 记录便于诊断
//
// 时间戳语义：
//   - started_at：本 step 开始执行时间（nullable，未执行时为 NULL）
//   - finished_at：本 step 执行完成时间（nullable，pending/running 时为 NULL）
//   - error：failed 时填写错误信息，便于诊断中断原因
type MigrateState struct {
	ID         int64      `gorm:"column:id;primaryKey;autoIncrement"          json:"id"`
	Step       string     `gorm:"column:step;not null;uniqueIndex:uk_step"   json:"step"`        // "1.1".."1.7" 等 step 标识
	Status     string     `gorm:"column:status;not null;index"               json:"status"`      // "pending" | "running" | "done" | "failed"
	StartedAt  *time.Time `gorm:"column:started_at"                          json:"started_at"`  // 本 step 开始时间，nullable
	FinishedAt *time.Time `gorm:"column:finished_at"                         json:"finished_at"` // 本 step 完成时间，nullable
	Error      string     `gorm:"column:error"                               json:"error"`       // failed 时的错误信息
	CreatedAt  time.Time  `gorm:"column:created_at;autoCreateTime"          json:"created_at"`
	UpdatedAt  time.Time  `gorm:"column:updated_at;autoUpdateTime"          json:"updated_at"`
}

// TableName 指定 GORM 表名。
func (MigrateState) TableName() string { return "migrate_state" }

package engine

// Migration 描述一次迁移：名称 + 按依赖顺序排列的 step 列表。
type Migration struct {
	Name        string // 迁移标识，如 "v1.6.0-chapter-id-refactor"，写入 migrate_state.migration
	Description string // 迁移目的简述（本次迁移为了什么、主要做什么），打印进日志便于排查
	Destructive bool   // 是否破坏性迁移（删列/rename/数据重写等）：true 时 migrate.Run 会在执行前自动备份全量数据
	Steps       []Step // 该迁移内的执行步骤，按序执行
}

// Registry 聚合所有已注册迁移，注册顺序 = 执行顺序。
// 具体迁移包（如 internal/migrate/v160）在 init() 里调用 Register 自注册；
// migrate.Run 遍历本注册表执行，对具体迁移无感知。
var Registry []Migration

// Register 注册一次迁移，供具体迁移包的 init() 调用。
func Register(m Migration) {
	Registry = append(Registry, m)
}

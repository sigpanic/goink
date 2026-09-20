package v160

import (
	"log/slog"

	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/migrate/engine"
)

// migration 是本次迁移的标识，写入 migrate_state.migration，与 engine 注册名一致。
const migration = "v1.6.0-chapter-id-refactor"

// step 实现 engine.Step 接口：Go 接口结构化满足，v160 只需 import engine，无需 import migrate。
type step struct {
	key string
	run func(db *gorm.DB, log *slog.Logger) error
}

func (s step) Key() string                             { return s.key }
func (s step) Run(db *gorm.DB, log *slog.Logger) error { return s.run(db, log) }

// registry 是本次迁移的步骤列表。
// key 在本 migration 内独立计数（N-描述），与 commit 编号解耦；按依赖顺序排列。
var registry = []engine.Step{
	// commit 1.5：交叉引用数据 num→id 重写
	step{key: "1-crossref-data", run: migrateCrossRefData},
	// commit 1.6：sort_order 初始化 + 建 volumes/ + 文件 rename
	step{key: "2-rename-files", run: migrateRenameFiles},
	// 保留未来计划的阅读位置；它不是可改写为 chapter_id 的既有章节引用。
	step{key: "3-reading-targets", run: migrateReadingTargets},
	// commit 1.7：删旧列 + 收尾（框架写 done 即收尾）
	step{key: "4-drop-legacy", run: migrateDropLegacy},
}

// init 自注册进 engine：migrate.Run 遍历 engine.Registry 统一执行，无需感知本包。
// Destructive：本次迁移含数据重写 + 文件 rename + 删旧列，属破坏性迁移，需自动备份。
func init() {
	engine.Register(engine.Migration{
		Name:        migration,
		Description: "v1.6.0 章节 id 化：交叉引用数据 chapter_number→chapter_id 重写、章节文件改名 chapters/id_{id}.md、删除旧 num 列",
		Destructive: true,
		Steps:       registry,
	})
}

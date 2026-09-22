// Package v120 normalizes the chapter-number column names introduced before
// v1.2.0. It is kept separate from v160 because v160 consumes the normalized
// chapter-number schema as its historical input.
package v120

import (
	"log/slog"

	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/migrate/engine"
)

const migration = "v1.2.0-chapter-number-column-names"

type step struct {
	key string
	run func(db *gorm.DB, log *slog.Logger) error
}

func (s step) Key() string                             { return s.key }
func (s step) Run(db *gorm.DB, log *slog.Logger) error { return s.run(db, log) }

func init() {
	engine.Register(engine.Migration{
		Name:           migration,
		Description:    "v1.2.0 章节号字段语义化命名：修正历史 *_id 误名并清理 novels.dir_path",
		Destructive:    true,
		NeedsMigration: needsMigration,
		PreSchemaSteps: []engine.Step{
			step{key: "1-rename-chapter-number-columns", run: migrateRenameChapterNumberColumns},
		},
		PostSchemaSteps: []engine.Step{
			step{key: "2-drop-novel-dir-path", run: migrateDropNovelDirPath},
		},
	})
}

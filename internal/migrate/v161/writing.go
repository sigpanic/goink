package v161

import (
	"fmt"
	"log/slog"

	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/migrate/engine"
)

const writingIndex = "uk_writing_date_novel"

type writingDailyStep struct{}

func (writingDailyStep) Key() string { return "1-aggregate-writing-by-date-and-novel" }

func needsWritingMigration(db *gorm.DB) (bool, error) {
	return db.Migrator().HasTable("writing_log") && !db.Migrator().HasIndex("writing_log", writingIndex), nil
}

// 必须先合并旧明细，再由 AutoMigrate 对齐唯一索引；事务内换表也移除章节关联。
func (writingDailyStep) Run(db *gorm.DB, _ *slog.Logger) error {
	return db.Transaction(func(tx *gorm.DB) error {
		needed, err := needsWritingMigration(tx)
		if err != nil || !needed {
			return err
		}
		added := "CASE WHEN word_delta > 0 THEN word_delta ELSE 0 END"
		deleted := "CASE WHEN word_delta < 0 THEN -word_delta ELSE 0 END"
		if tx.Migrator().HasColumn("writing_log", "words_added") {
			added = "COALESCE(words_added, " + added + ")"
		}
		if tx.Migrator().HasColumn("writing_log", "words_deleted") {
			deleted = "COALESCE(words_deleted, " + deleted + ")"
		}
		createdAt := "NULL"
		if tx.Migrator().HasColumn("writing_log", "created_at") {
			createdAt = "MIN(created_at)"
		}
		for _, query := range []string{
			`CREATE TABLE writing_log_daily_migration (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				date TEXT NOT NULL, novel_id INTEGER NOT NULL DEFAULT 0,
				word_delta INTEGER NOT NULL,
				words_added INTEGER NOT NULL DEFAULT 0,
				words_deleted INTEGER NOT NULL DEFAULT 0,
				created_at DATETIME
			)`,
			`INSERT INTO writing_log_daily_migration
				(id, date, novel_id, word_delta, words_added, words_deleted, created_at)
			 SELECT MIN(id), date, novel_id, SUM(word_delta), SUM(` + added + `), SUM(` + deleted + `), ` + createdAt + `
			 FROM writing_log GROUP BY date, novel_id`,
			`DROP TABLE writing_log`,
			`ALTER TABLE writing_log_daily_migration RENAME TO writing_log`,
			`CREATE UNIQUE INDEX ` + writingIndex + ` ON writing_log(date, novel_id)`,
		} {
			if err := tx.Exec(query).Error; err != nil {
				return fmt.Errorf("aggregate writing log: %w", err)
			}
		}
		return nil
	})
}

func init() {
	engine.Register(engine.Migration{
		Name:           "v1.6.1-writing-daily",
		Description:    "写作增删量按日期和小说汇总，移除逐次保存与章节明细",
		Destructive:    false,
		NeedsMigration: needsWritingMigration,
		PreSchemaSteps: []engine.Step{writingDailyStep{}},
	})
}

package migrate

import (
	"log/slog"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/migrate/engine"
)

func TestIsNewDatabaseRequiresNoBusinessTables(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}

	isNew, err := isNewDatabase(db)
	if err != nil {
		t.Fatal(err)
	}
	if !isNew {
		t.Fatal("空数据库应视为新库")
	}

	if err := db.Exec(`CREATE TABLE writing_log (id INTEGER PRIMARY KEY)`).Error; err != nil {
		t.Fatal(err)
	}
	isNew, err = isNewDatabase(db)
	if err != nil {
		t.Fatal(err)
	}
	if isNew {
		t.Fatal("任一业务表存在时不应视为新库")
	}
}

func TestMarkPlannedDonePrecedesBusinessSchemaChanges(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`CREATE TABLE chapters (id INTEGER PRIMARY KEY, chapter_number INTEGER NOT NULL)`,
		`CREATE TABLE time_entries (id INTEGER PRIMARY KEY, source_chapter INTEGER)`,
	} {
		if err := db.Exec(query).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.AutoMigrate(&engine.MigrateState{}); err != nil {
		t.Fatal(err)
	}

	plans, err := engine.BuildPlans(db, slog.Default(), false, engine.Registry)
	if err != nil {
		t.Fatal(err)
	}
	if err := markPlannedDone(db, slog.Default(), plans); err != nil {
		t.Fatal(err)
	}

	var done int64
	if err := db.Model(&engine.MigrateState{}).
		Where("migration = ? AND status = ?", "v1.2.0-chapter-number-column-names", "done").
		Count(&done).Error; err != nil {
		t.Fatal(err)
	}
	if done != 2 {
		t.Fatalf("v1.2.0 应在业务 schema 变化前登记全部步骤: got %d, want 2", done)
	}

	// 模拟 v1.6.0 的 AutoMigrate 已加入当前 source_chapter_id 列后进程中断。
	if err := db.Exec(`ALTER TABLE time_entries ADD COLUMN source_chapter_id INTEGER`).Error; err != nil {
		t.Fatal(err)
	}
	plans, err = engine.BuildPlans(db, slog.Default(), false, engine.Registry)
	if err != nil {
		t.Fatalf("中间 schema 重启不应重新探测 v1.2.0: %v", err)
	}

	actions := make(map[string]engine.PlanAction, len(plans))
	for _, plan := range plans {
		actions[plan.Migration.Name] = plan.Action
	}
	if actions["v1.2.0-chapter-number-column-names"] != engine.PlanSkip {
		t.Fatalf("v1.2.0 中间 schema 重启应跳过: action=%v", actions["v1.2.0-chapter-number-column-names"])
	}
	if actions["v1.6.0-chapter-id-refactor"] != engine.PlanRun {
		t.Fatalf("v1.6.0 中间 schema 重启应继续执行: action=%v", actions["v1.6.0-chapter-id-refactor"])
	}
}

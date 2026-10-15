package migrate

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/migrate/engine"
	"github.com/sigpanic/goink/internal/migrate/upgrade/fixture"
	"github.com/sigpanic/goink/internal/testsupport"
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

func TestRunPersistsNoOpBeforeInterruptedAutoMigrate(t *testing.T) {
	gitBin := testsupport.RequireGit(t)
	dataDir := testsupport.Isolate(t)
	t.Setenv("GOINK_TESTING", "1")
	t.Setenv("GOINK_GIT_BIN", gitBin)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	config.Set(&config.AppConfig{})
	if err := fixture.Extract(filepath.Join("upgrade", "testdata", "v1.5.0-fresh.zip"), dataDir); err != nil {
		t.Fatal(err)
	}
	openDB := func() *gorm.DB {
		t.Helper()
		db, err := gorm.Open(sqlite.Open(config.GlobalDBPath()), &gorm.Config{})
		if err != nil {
			t.Fatal(err)
		}
		sqlDB, err := db.DB()
		if err != nil {
			t.Fatal(err)
		}
		sqlDB.SetMaxOpenConns(1)
		t.Cleanup(func() { _ = sqlDB.Close() })
		return db
	}
	db := openDB()
	interrupted := errors.New("interrupted after adding source_chapter_id")
	// 让真实 DDL 先落盘，再中断 auto；此时新旧列共存，尚未进入数据重写步骤。
	if err := db.Callback().Raw().After("gorm:raw").Register("test:interrupt-auto", func(tx *gorm.DB) {
		query := tx.Statement.SQL.String()
		if tx.Error == nil && strings.Contains(query, "ALTER TABLE `time_entries`") && strings.Contains(query, "`source_chapter_id`") {
			tx.AddError(interrupted)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := Run(db, slog.Default()); !errors.Is(err, interrupted) {
		t.Fatalf("首次运行应在真实 auto 加列后中断: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}

	// 用新连接模拟重启，不沿用首次运行的回调或内存状态。
	db = openDB()
	if !db.Migrator().HasColumn("time_entries", "source_chapter") || !db.Migrator().HasColumn("time_entries", "source_chapter_id") {
		t.Fatal("中断样本应保留新旧 source_chapter 列共存的结构")
	}
	var states []engine.MigrateState
	if err := db.Where("migration = ?", "v1.2.0-chapter-number-column-names").Find(&states).Error; err != nil {
		t.Fatal(err)
	}
	if len(states) != 3 {
		t.Fatalf("v1.2.0 应在后续 auto 之前登记全部步骤: got %d, want 3", len(states))
	}
	for _, state := range states {
		if state.Status != "done" || state.StartedAt != nil {
			t.Fatalf("v1.2.0 应由真实 Run 判定无需执行并登记完成: %+v", state)
		}
	}
	var auto engine.MigrateState
	if err := db.Where("migration = ? AND step = ?", "v1.6.0-chapter-id-refactor", "auto").First(&auto).Error; err != nil {
		t.Fatal(err)
	}
	if auto.Status != "failed" || !strings.Contains(auto.Error, interrupted.Error()) {
		t.Fatalf("v1.6.0 auto 应保留失败记录供重启恢复: %+v", auto)
	}
	if err := Run(db, slog.Default()); err != nil {
		t.Fatalf("混合 schema 重启应跳过已登记的 v1.2.0，并恢复 v1.6.0: %v", err)
	}
	if db.Migrator().HasColumn("time_entries", "source_chapter") || db.Migrator().HasColumn("chapters", "chapter_number") {
		t.Fatal("恢复后应完成历史列清理")
	}
	var matched int64
	if err := db.Table("time_entries").Where("(id = 1 AND source_chapter_id = 11) OR (id = 2 AND source_chapter_id = 21)").Count(&matched).Error; err != nil {
		t.Fatal(err)
	}
	if matched != 2 {
		t.Fatalf("恢复后应将历史章节号正确转换为稳定 ID: matched=%d", matched)
	}
	for _, migration := range engine.Registry {
		for _, step := range migration.AllSteps() {
			var state engine.MigrateState
			if err := db.Where("migration = ? AND step = ?", migration.Name, step.Key()).First(&state).Error; err != nil {
				t.Fatal(err)
			}
			if state.Status != "done" {
				t.Fatalf("恢复后全部注册步骤都应完成: %+v", state)
			}
		}
	}
}

func TestRunRejectsLegacyInterruptionBeforeBusinessChanges(t *testing.T) {
	db := openSequentialTestDB(t)
	if err := db.AutoMigrate(&engine.MigrateState{}); err != nil {
		t.Fatal(err)
	}
	state := engine.MigrateState{
		Migration: "v1.2.0-chapter-number-column-names",
		Step:      "1-rename-chapter-number-columns",
		Status:    "running",
	}
	if err := db.Create(&state).Error; err != nil {
		t.Fatal(err)
	}
	if err := Run(db, slog.Default()); !errors.Is(err, engine.ErrLegacyMigrationIncomplete) {
		t.Fatalf("应提示先用旧版完成迁移: %v", err)
	}
	if db.Migrator().HasTable("chapters") || db.Migrator().HasTable("writing_log") {
		t.Fatal("拒绝旧中断状态时不应创建业务表")
	}
	var states []engine.MigrateState
	if err := db.Find(&states).Error; err != nil {
		t.Fatal(err)
	}
	if len(states) != 1 || states[0].Status != "running" || states[0].Step != state.Step {
		t.Fatalf("旧状态应保留给旧版恢复: %+v", states)
	}
}

type sequentialTestStep struct {
	key string
	run func(*gorm.DB) error
}

func (s sequentialTestStep) Key() string                           { return s.key }
func (s sequentialTestStep) Run(db *gorm.DB, _ *slog.Logger) error { return s.run(db) }

type sequentialFirstModel struct {
	ID     int64 `gorm:"primaryKey"`
	Legacy string
}

func (sequentialFirstModel) TableName() string { return "sequential_fixture" }

type sequentialSecondModel struct {
	ID    int64 `gorm:"primaryKey"`
	Value string
}

func (sequentialSecondModel) TableName() string { return "sequential_fixture" }

func TestRunReevaluatesEachGroupAfterPreviousSchema(t *testing.T) {
	db := openSequentialTestDB(t)
	original := engine.Registry
	t.Cleanup(func() { engine.Registry = original })
	engine.Registry = []engine.Migration{
		{
			Name:           "first",
			NeedsMigration: func(db *gorm.DB) (bool, error) { return !db.Migrator().HasTable(&sequentialFirstModel{}), nil },
			PreSchemaSteps: []engine.Step{sequentialTestStep{key: "pre", run: func(db *gorm.DB) error {
				return db.Exec("CREATE TABLE sequential_input AS SELECT 'seed' AS value").Error
			}}},
			SchemaStep: engine.AutoMigrateStep{Models: func() []any { return []any{&sequentialFirstModel{}} }},
			PostSchemaSteps: []engine.Step{sequentialTestStep{key: "post", run: func(db *gorm.DB) error {
				return db.Exec("INSERT INTO sequential_fixture (id, legacy) SELECT 1, value FROM sequential_input").Error
			}}},
		},
		{
			Name: "second",
			NeedsMigration: func(db *gorm.DB) (bool, error) {
				return db.Migrator().HasColumn(&sequentialFirstModel{}, "legacy"), nil
			},
			PreSchemaSteps: []engine.Step{sequentialTestStep{key: "pre", run: func(db *gorm.DB) error {
				return db.Exec("UPDATE sequential_fixture SET legacy = legacy || '-pre'").Error
			}}},
			SchemaStep: engine.AutoMigrateStep{Models: func() []any { return []any{&sequentialSecondModel{}} }},
			PostSchemaSteps: []engine.Step{sequentialTestStep{key: "post", run: func(db *gorm.DB) error {
				if err := db.Exec("UPDATE sequential_fixture SET value = legacy").Error; err != nil {
					return err
				}
				return db.Exec("ALTER TABLE sequential_fixture DROP COLUMN legacy").Error
			}}},
		},
	}
	// 起始时 second 无旧表可处理；必须等 first 建表并写入数据后再判定。
	for attempt := 0; attempt < 2; attempt++ {
		if err := Run(db, slog.Default()); err != nil {
			t.Fatalf("第 %d 次运行: %v", attempt+1, err)
		}
		var row sequentialSecondModel
		if err := db.First(&row, 1).Error; err != nil {
			t.Fatal(err)
		}
		if row.Value != "seed-pre" || db.Migrator().HasColumn(&sequentialFirstModel{}, "legacy") {
			t.Fatalf("必须按组完成 pre/auto/post 且重启不重放历史 schema: %+v", row)
		}
	}
}

func openSequentialTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	// 保留一张业务表，避免测试被新库初始化短路。
	if err := db.Exec("CREATE TABLE existing_business (id INTEGER PRIMARY KEY)").Error; err != nil {
		t.Fatal(err)
	}
	return db
}

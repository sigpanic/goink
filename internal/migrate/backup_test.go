package migrate

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/migrate/engine"
	"github.com/sigpanic/goink/internal/testsupport"
)

func TestBackupSQLiteDatabaseIncludesCommittedWALData(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "source.db")
	source, err := gorm.Open(sqlite.Open(sourcePath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open source database: %v", err)
	}
	sourceSQL, err := source.DB()
	if err != nil {
		t.Fatalf("get source sql.DB: %v", err)
	}
	t.Cleanup(func() { _ = sourceSQL.Close() })
	sourceSQL.SetMaxOpenConns(1)

	if err := source.Exec("PRAGMA journal_mode=WAL").Error; err != nil {
		t.Fatalf("enable WAL: %v", err)
	}
	if err := source.Exec("PRAGMA wal_autocheckpoint=0").Error; err != nil {
		t.Fatalf("disable automatic checkpoint: %v", err)
	}
	if err := source.Exec("CREATE TABLE entries (value TEXT NOT NULL)").Error; err != nil {
		t.Fatalf("create table: %v", err)
	}
	if err := source.Exec("INSERT INTO entries (value) VALUES (?)", "only-in-wal").Error; err != nil {
		t.Fatalf("insert WAL fixture: %v", err)
	}

	walInfo, err := os.Stat(sourcePath + "-wal")
	if err != nil {
		t.Fatalf("stat source WAL: %v", err)
	}
	if walInfo.Size() == 0 {
		t.Fatal("WAL fixture was unexpectedly empty")
	}

	mainFileCopyPath := filepath.Join(t.TempDir(), "main-file-only.db")
	if err := copyFile(sourcePath, mainFileCopyPath); err != nil {
		t.Fatalf("copy source main database file: %v", err)
	}
	mainFileCopy, err := sql.Open("sqlite3", mainFileCopyPath)
	if err != nil {
		t.Fatalf("open main-file-only copy: %v", err)
	}
	t.Cleanup(func() { _ = mainFileCopy.Close() })
	var copiedValue string
	if err := mainFileCopy.QueryRow("SELECT value FROM entries").Scan(&copiedValue); err == nil {
		t.Fatalf("main database file unexpectedly contained WAL row %q", copiedValue)
	}

	backupPath := filepath.Join(t.TempDir(), "backup.db")
	if err := backupSQLiteDatabase(context.Background(), source, backupPath); err != nil {
		t.Fatalf("backup SQLite database: %v", err)
	}

	backupDB, err := sql.Open("sqlite3", backupPath)
	if err != nil {
		t.Fatalf("open backup database: %v", err)
	}
	t.Cleanup(func() { _ = backupDB.Close() })

	var value string
	if err := backupDB.QueryRow("SELECT value FROM entries").Scan(&value); err != nil {
		t.Fatalf("read WAL-backed row from backup: %v", err)
	}
	if value != "only-in-wal" {
		t.Errorf("backup value = %q, want %q", value, "only-in-wal")
	}
}

func TestRunStopsMigrationWhenRequiredBackupFails(t *testing.T) {
	dataDir := testsupport.Isolate(t)
	config.Set(&config.AppConfig{})

	dbPath := config.GlobalDBPath()
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get legacy sql.DB: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	for _, query := range []string{
		`CREATE TABLE novels (id INTEGER PRIMARY KEY, title TEXT NOT NULL)`,
		`CREATE TABLE chapters (id INTEGER PRIMARY KEY, novel_id INTEGER NOT NULL, chapter_number INTEGER NOT NULL)`,
	} {
		if err := db.Exec(query).Error; err != nil {
			t.Fatalf("create legacy schema: %v", err)
		}
	}

	if err := os.WriteFile(filepath.Join(dataDir, "backups"), []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("block backup directory: %v", err)
	}

	err = Run(db, slog.Default())
	if err == nil {
		t.Fatal("migrate.Run succeeded despite destructive migration backup failure")
	}
	if !db.Migrator().HasColumn("chapters", "chapter_number") {
		t.Fatal("destructive migration ran after backup failure and removed chapter_number")
	}
	if db.Migrator().HasColumn("chapters", "volume_id") {
		t.Fatal("destructive migration ran after backup failure and added volume_id")
	}
}

func TestRunBacksUpWholeUpgradeOnceBeforeChanges(t *testing.T) {
	for _, delayed := range []bool{false, true} {
		name := "initially-required"
		if delayed {
			name = "required-after-earlier-group"
		}
		t.Run(name, func(t *testing.T) {
			dataDir := testsupport.Isolate(t)
			config.Set(&config.AppConfig{})
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
			for _, query := range []string{
				"CREATE TABLE backup_fixture (value TEXT NOT NULL)",
				"INSERT INTO backup_fixture VALUES ('original')",
			} {
				if err := db.Exec(query).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := db.AutoMigrate(&engine.MigrateState{}); err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&engine.MigrateState{Migration: "v1.0.0-base", Step: "base", Status: "done"}).Error; err != nil {
				t.Fatal(err)
			}
			novelFile := filepath.Join(dataDir, "novels", "1", "chapters", "001.md")
			if err := os.MkdirAll(filepath.Dir(novelFile), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(novelFile, []byte("original"), 0644); err != nil {
				t.Fatal(err)
			}
			blocker := filepath.Join(dataDir, "interrupt-upgrade")
			if err := os.WriteFile(blocker, nil, 0644); err != nil {
				t.Fatal(err)
			}
			originalRegistry := engine.Registry
			t.Cleanup(func() { engine.Registry = originalRegistry })
			interrupted := errors.New("interrupted at last migration")
			needsLater := func(db *gorm.DB) (bool, error) {
				return !delayed || db.Migrator().HasTable("created_by_upgrade"), nil
			}
			engine.Registry = []engine.Migration{
				{Name: "v1.0.0-base", PreSchemaSteps: []engine.Step{sequentialTestStep{key: "base", run: func(_ *gorm.DB) error {
					return errors.New("completed base must not run")
				}}}},
				{Name: "v1.1.0-prepare", NeedsMigration: func(_ *gorm.DB) (bool, error) { return true, nil },
					PreSchemaSteps: []engine.Step{sequentialTestStep{key: "prepare", run: func(db *gorm.DB) error {
						if err := db.Exec("UPDATE backup_fixture SET value = 'prepared'").Error; err != nil {
							return err
						}
						return db.Exec("CREATE TABLE IF NOT EXISTS created_by_upgrade (id INTEGER)").Error
					}}}},
				{Name: "v1.2.0-first", RequiresBackup: true, NeedsMigration: needsLater,
					PreSchemaSteps: []engine.Step{sequentialTestStep{key: "first", run: func(db *gorm.DB) error {
						return db.Exec("UPDATE backup_fixture SET value = 'first-destructive'").Error
					}}}},
				{Name: "v1.3.0-middle", NeedsMigration: func(_ *gorm.DB) (bool, error) { return true, nil },
					PreSchemaSteps: []engine.Step{sequentialTestStep{key: "middle", run: func(_ *gorm.DB) error {
						return os.WriteFile(novelFile, []byte("changed"), 0644)
					}}}},
				{Name: "v1.4.0-last", RequiresBackup: true, NeedsMigration: needsLater,
					PreSchemaSteps: []engine.Step{sequentialTestStep{key: "last", run: func(db *gorm.DB) error {
						if _, err := os.Stat(blocker); err == nil {
							return interrupted
						} else if !errors.Is(err, os.ErrNotExist) {
							return err
						}
						return db.Exec("UPDATE backup_fixture SET value = 'completed'").Error
					}}}},
			}
			if err := Run(db, slog.Default()); !errors.Is(err, interrupted) {
				t.Fatalf("首次应在最后一组中断: %v", err)
			}
			backupRoot := filepath.Join(dataDir, "backups")
			backup := filepath.Join(backupRoot, "v1.0.0-upgrade-to-v1.4.0")
			entries, err := os.ReadDir(backupRoot)
			if err != nil || len(entries) != 1 || entries[0].Name() != filepath.Base(backup) {
				t.Fatalf("多个破坏性组应共用一份起始备份: entries=%v err=%v", entries, err)
			}
			backupDBPath := filepath.Join(backup, "novel-agent.db")
			backupDB, err := sql.Open("sqlite3", backupDBPath+"?mode=ro")
			if err != nil {
				t.Fatal(err)
			}
			var value string
			if err := backupDB.QueryRow("SELECT value FROM backup_fixture").Scan(&value); err != nil {
				t.Fatal(err)
			}
			if err := backupDB.Close(); err != nil {
				t.Fatal(err)
			}
			if value != "original" {
				t.Fatalf("备份必须早于所有组的业务修改: value=%q", value)
			}
			if content, err := os.ReadFile(filepath.Join(backup, "novels", "1", "chapters", "001.md")); err != nil || string(content) != "original" {
				t.Fatalf("备份必须保留原始小说文件: content=%q err=%v", content, err)
			}
			before, err := os.ReadFile(backupDBPath)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(blocker); err != nil {
				t.Fatal(err)
			}
			sqlDB, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			if err := sqlDB.Close(); err != nil {
				t.Fatal(err)
			}
			db = openDB()
			if err := Run(db, slog.Default()); err != nil {
				t.Fatalf("重启后应复用原备份并继续迁移: %v", err)
			}
			entries, err = os.ReadDir(backupRoot)
			if err != nil || len(entries) != 1 {
				t.Fatalf("起点推进后不能重复备份: entries=%v err=%v", entries, err)
			}
			after, err := os.ReadFile(backupDBPath)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("重试不能覆盖起始数据库快照: err=%v", err)
			}
			if err := db.Raw("SELECT value FROM backup_fixture").Scan(&value).Error; err != nil || value != "completed" {
				t.Fatalf("重试后应完成整轮迁移: value=%q err=%v", value, err)
			}
			if content, err := os.ReadFile(filepath.Join(backup, "novels", "1", "chapters", "001.md")); err != nil || string(content) != "original" {
				t.Fatalf("重试不能覆盖起始小说文件: content=%q err=%v", content, err)
			}

			// 后续新增迁移目标时另建备份，不复用上一轮的起始快照。
			engine.Registry = append(engine.Registry, engine.Migration{
				Name: "v1.5.0-next", RequiresBackup: true,
				NeedsMigration: func(_ *gorm.DB) (bool, error) { return true, nil },
				PreSchemaSteps: []engine.Step{sequentialTestStep{key: "next", run: func(db *gorm.DB) error {
					return db.Exec("UPDATE backup_fixture SET value = 'next'").Error
				}}},
			})
			if err := Run(db, slog.Default()); err != nil {
				t.Fatalf("新目标应建立新一轮备份: %v", err)
			}
			entries, err = os.ReadDir(backupRoot)
			if err != nil || len(entries) != 2 {
				t.Fatalf("新一轮应新增一份备份: entries=%v err=%v", entries, err)
			}
			backupDB, err = sql.Open("sqlite3", filepath.Join(backupRoot, "v1.4.0-upgrade-to-v1.5.0", "novel-agent.db")+"?mode=ro")
			if err != nil {
				t.Fatal(err)
			}
			if err := backupDB.QueryRow("SELECT value FROM backup_fixture").Scan(&value); err != nil {
				t.Fatal(err)
			}
			if err := backupDB.Close(); err != nil {
				t.Fatal(err)
			}
			if value != "completed" {
				t.Fatalf("新备份应保存上一轮完成后的状态: value=%q", value)
			}
		})
	}
}

func TestPrepareUpgradeBackupIncludesCompletedLaterGroupRequiringBackup(t *testing.T) {
	dataDir := testsupport.Isolate(t)
	config.Set(&config.AppConfig{})
	db, err := gorm.Open(sqlite.Open(config.GlobalDBPath()), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&engine.MigrateState{}); err != nil {
		t.Fatal(err)
	}
	registry := []engine.Migration{
		{Name: "v1.8.0-pending", NeedsMigration: func(_ *gorm.DB) (bool, error) { return true, nil },
			PreSchemaSteps: []engine.Step{sequentialTestStep{key: "pending", run: func(_ *gorm.DB) error {
				return errors.New("backup inspection must not execute steps")
			}}}},
		{Name: "v1.9.0-completed", RequiresBackup: true,
			PreSchemaSteps: []engine.Step{sequentialTestStep{key: "completed"}}},
	}
	if err := db.Create(&engine.MigrateState{Migration: registry[1].Name, Step: "completed", Status: "done"}).Error; err != nil {
		t.Fatal(err)
	}
	// 即使后组已有完成记录，也按首个待执行组之后的备份要求保守备份。
	if err := prepareUpgradeBackup(db, slog.Default(), false, registry); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "backups", "legacy-upgrade-to-v1.9.0", "novel-agent.db")); err != nil {
		t.Fatalf("后组的完成状态不应排除起始备份: %v", err)
	}
}

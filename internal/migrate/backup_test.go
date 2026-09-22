package migrate

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/platform"
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

func TestRunStopsDestructiveMigrationWhenBackupFails(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("GOINK_DATA_DIR", dataDir)
	platform.ResetDataDirCache()
	t.Cleanup(platform.ResetDataDirCache)
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

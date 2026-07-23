//go:build cgo && e2e

package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/migrate"
	"github.com/sigpanic/goink/internal/platform"
	"github.com/sigpanic/goink/internal/rag"
	"github.com/sigpanic/goink/internal/storage"
)

func TestAppLifecycle_InitWithConfig(t *testing.T) {
	// Verify the singletons initialized in TestMain are functional.
	// This test validates the same lifecycle flow as app/handler.go initWithConfig,
	// but without re-initializing singletons (which would fail due to sync.Once).

	// 1. Global config should already be set
	dataDir := platform.DataDir()
	cfg := &config.AppConfig{DataDir: dataDir}
	config.Set(cfg)

	// 2. Embedder should already be initialized
	embedder, err := rag.GetEmbedder()
	if err != nil {
		t.Fatalf("GetEmbedder() failed: %v", err)
	}
	if embedder == nil {
		t.Fatal("GetEmbedder() returned nil")
	}

	// 3. Open a separate database to verify the storage/migrate lifecycle
	dbPath := config.GlobalDBPath()
	t.Logf("DB path: %s", dbPath)
	db, err := storage.Open(dbPath, testLogger(t))
	if err != nil {
		t.Fatalf("storage.Open() failed: %v", err)
	}
	t.Cleanup(func() {
		storage.Close(db)
		os.Remove(dbPath)
	})

	// 4. Run auto-migrations
	if err := migrate.Run(db, testLogger(t)); err != nil {
		t.Fatalf("migrate.Run() failed: %v", err)
	}

	// 5. Load settings
	settings, err := config.LoadSettings(db)
	if err != nil {
		t.Fatalf("config.LoadSettings() failed: %v", err)
	}
	t.Logf("Settings loaded: approval_mode=%s", settings.ApprovalMode)

	// 6. VectorStore should already be initialized
	vs := rag.GetVectorStore()
	if vs == nil {
		t.Fatal("GetVectorStore() returned nil")
	}

	t.Log("Full app lifecycle verification completed successfully")
}

func TestAppLifecycle_NovelDirectoryCreation(t *testing.T) {
	dataDir := platform.DataDir()

	// Verify DataDir is correct
	expectedDir := os.Getenv("GOINK_DATA_DIR")
	if dataDir != expectedDir {
		t.Errorf("DataDir() = %q, expected %q", dataDir, expectedDir)
	}

	// Verify necessary subdirectories can be created
	for _, sub := range []string{"novels", "skills", "models", "runtime"} {
		p := filepath.Join(dataDir, sub)
		if err := os.MkdirAll(p, 0755); err != nil {
			t.Errorf("MkdirAll %s: %v", sub, err)
		}
	}

	t.Log("Novel directory structure OK")
}

func TestAppLifecycle_DatabaseOperations(t *testing.T) {
	// Open database and verify basic operations
	dbPath := config.GlobalDBPath()
	db, err := storage.Open(dbPath, testLogger(t))
	if err != nil {
		t.Fatalf("storage.Open() failed: %v", err)
	}
	t.Cleanup(func() {
		storage.Close(db)
		os.Remove(dbPath)
	})

	// Run migrations
	if err := migrate.Run(db, testLogger(t)); err != nil {
		t.Fatalf("migrate.Run() failed: %v", err)
	}

	// Verify database is functional
	sqlDB, _ := db.DB()
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("database ping failed: %v", err)
	}

	// Verify WAL mode
	var mode string
	db.Raw("PRAGMA journal_mode").Scan(&mode)
	if mode != "wal" {
		t.Errorf("journal_mode = %q, expected 'wal'", mode)
	}

	t.Log("Database operations OK")
}

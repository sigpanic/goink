package app

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"novel/internal/agent"
	"novel/internal/approval"
	"novel/internal/chapter"
	"novel/internal/character"
	"novel/internal/config"
	"novel/internal/llm"
	"novel/internal/location"
	"novel/internal/mcp_tools"
	"novel/internal/migrate"
	"novel/internal/novel"
	"novel/internal/reader"
	"novel/internal/rollback"
	"novel/internal/session"
	"novel/internal/skill"
	"novel/internal/storage"
	"novel/internal/storyarc"
	"novel/internal/style"
	"novel/internal/timeline"
	"novel/internal/writing"
)

// setupTestApp creates a fully initialized App with a temp directory,
// in-memory SQLite, and all stores. Resources are cleaned up via t.Cleanup.
func setupTestApp(t *testing.T) *App {
	t.Helper()

	tmpDir := t.TempDir()
	t.Setenv("GOINK_DATA_DIR", tmpDir)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Set global config so config.DataDirPath() etc. resolve to tmpDir.
	cfg := &config.AppConfig{DataDir: tmpDir}
	config.Set(cfg)

	// Ensure novel sub-directories exist.
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "novels"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "skills"), 0o755))

	// In-memory SQLite (no CGO needed).
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err, "open in-memory SQLite")

	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	})

	// Run auto-migration (pure GORM, no CGO).
	require.NoError(t, migrate.Run(db, logger), "run migrations")

	// Load settings (creates default row).
	settings, err := config.LoadSettings(db)
	require.NoError(t, err, "load settings")

	// Register operation log hooks.
	require.NoError(t, storage.RegisterOplogHooks(db))

	// Domain stores.
	novelStore := novel.NewStore(db, logger)
	chapterStore := chapter.NewStore(db, logger)
	characterStore := character.NewStore(db, logger)
	sessionStore := session.NewStore(db, logger)
	timelineStore := timeline.NewStore(db, logger)
	storyarcStore := storyarc.NewStore(db, logger)
	locationStore := location.NewStore(db, logger)
	readerStore := reader.NewStore(db, logger)
	turnCommitStore := rollback.NewStore(db, logger)
	writingStore := writing.NewStore(db, logger)
	styleStore := style.NewStore(db, logger)

	skillStore, err := skill.NewStore(logger, filepath.Join(tmpDir, "skills"))
	require.NoError(t, err, "init skill store")

	// MCP tool registry.
	registry := mcp_tools.NewRegistry(logger)
	mcp_tools.RegisterAllTools(registry)

	// LLM client with empty providers (no real API calls in tests).
	providers := llm.Merge(llm.Builtin, &llm.UserLLMConfig{})
	llmClient := llm.NewClient(providers, logger)

	// Approval service.
	approvals := approval.NewService(logger, settings.ApprovalMode)

	// Cancel manager.
	cancelMgr := agent.NewCancelManager()

	// Agent.
	ag := agent.New(llmClient, registry, sessionStore, db, approvals, logger, skillStore, cancelMgr)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	app := &App{
		ctx:       ctx,
		cancel:    cancel,
		logger:    logger,
		cfg:       cfg,
		settings:  settings,
		db:        db,
		llmClient: llmClient,
		agent:     ag,
		cancelMgr: cancelMgr,
		registry:  registry,
		approvals: approvals,

		novel:      novelStore,
		chapter:    chapterStore,
		character:  characterStore,
		session:    sessionStore,
		skill:      skillStore,
		style:      styleStore,
		timeline:   timelineStore,
		storyarc:   storyarcStore,
		location:   locationStore,
		reader:     readerStore,
		turnCommit: turnCommitStore,
		writing:    writingStore,
	}

	return app
}

// createTestNovel creates a novel in the database and returns it.
// It also ensures the novel's directory exists on disk.
func createTestNovel(t *testing.T, app *App) *novel.Novel {
	t.Helper()

	n := &novel.Novel{
		Title:       "Test Novel",
		Genre:       "fantasy",
		Description: "A novel for testing",
	}
	require.NoError(t, app.db.Create(n).Error, "create test novel")

	// Ensure the novel's directory exists (normally done by the git init flow).
	novelDir := config.NovelDirPath(n.ID)
	require.NoError(t, os.MkdirAll(novelDir, 0o755), "create novel dir")

	return n
}

//go:build cgo

package migrate_test

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/git"
	"github.com/sigpanic/goink/internal/migrate"
	"github.com/sigpanic/goink/internal/platform"
)

// TestRunMigratesLegacyDatabaseEndToEnd 是 migrate.Run 的总集成契约：从 v1.6.0
// 之前的真实 SQLite schema、真实数据和旧文件命名开始，只调用一次公开入口，验证
// 历史字段改名、v160 数据转换、文件/Git 迁移和最终 schema。后续新增迁移步骤时，
// 应在这里补充初始结构、样本数据和最终断言。
func TestRunMigratesLegacyDatabaseEndToEnd(t *testing.T) {
	dataDir := setupMigrationIntegrationEnv(t)
	db := openMigrationIntegrationDB(t)
	execSQL := func(query string) {
		t.Helper()
		if err := db.Exec(query).Error; err != nil {
			t.Fatalf("exec %q: %v", query, err)
		}
	}

	// v1.6.0 前的全部相关旧表。source/resolved/writing/character 的 *_id
	// 在该版本只是章节号的误名；这里故意不预建任何 v1.6.0 新列。
	for _, query := range []string{
		`CREATE TABLE novels (id INTEGER PRIMARY KEY, title TEXT NOT NULL, genre TEXT, description TEXT, created_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE chapters (id INTEGER PRIMARY KEY, novel_id INTEGER NOT NULL, chapter_number INTEGER NOT NULL, title TEXT, summary TEXT, word_count INTEGER DEFAULT 0, created_at DATETIME, updated_at DATETIME)`,
		`CREATE UNIQUE INDEX uk_novel_chapter ON chapters(novel_id, chapter_number)`,
		`CREATE TABLE time_entries (id INTEGER PRIMARY KEY, novel_id INTEGER NOT NULL, category TEXT NOT NULL, status TEXT NOT NULL, title TEXT NOT NULL, content TEXT, detail_json TEXT, target_chapter INTEGER NOT NULL, importance INTEGER DEFAULT 3, source_chapter_id INTEGER, source TEXT, resolved_chapter_id INTEGER, created_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE story_arcs (id INTEGER PRIMARY KEY, novel_id INTEGER NOT NULL, name TEXT NOT NULL, description TEXT, arc_type TEXT NOT NULL, importance INTEGER DEFAULT 1, status TEXT NOT NULL, reactivate_at TEXT, created_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE arc_nodes (id INTEGER PRIMARY KEY, novel_id INTEGER NOT NULL, story_arc_id INTEGER NOT NULL, title TEXT NOT NULL, description TEXT, target_chapter INTEGER DEFAULT 0, actual_chapter INTEGER DEFAULT 0, status TEXT NOT NULL DEFAULT 'pending', created_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE reader_perspectives (id INTEGER PRIMARY KEY, novel_id INTEGER NOT NULL, type TEXT NOT NULL, content TEXT NOT NULL, related_truth TEXT, planted_chapter INTEGER NOT NULL, revealed_chapter INTEGER DEFAULT 0, created_at DATETIME)`,
		`CREATE TABLE writing_log (id INTEGER PRIMARY KEY, date TEXT NOT NULL, novel_id INTEGER NOT NULL DEFAULT 0, chapter_id INTEGER NOT NULL DEFAULT 0, word_delta INTEGER NOT NULL, created_at DATETIME)`,
		`CREATE INDEX idx_writing_log_chapter_id ON writing_log(chapter_id)`,
		`CREATE TABLE character_relations (id INTEGER PRIMARY KEY, novel_id INTEGER NOT NULL, source_character_id INTEGER NOT NULL, target_character_id INTEGER NOT NULL, relation_describe TEXT NOT NULL, description TEXT, chapter_id INTEGER, is_current BOOLEAN NOT NULL, created_at DATETIME)`,
	} {
		execSQL(query)
	}

	execSQL(`INSERT INTO novels (id, title) VALUES (1, 'n1'), (2, 'n2')`)
	execSQL(`INSERT INTO chapters (id, novel_id, chapter_number, title) VALUES (1, 1, 1, 'c1'), (2, 1, 2, 'c2'), (3, 1, 3, 'c3'), (4, 2, 1, 'c1')`)
	execSQL(`INSERT INTO time_entries (id, novel_id, category, status, title, target_chapter, importance, source_chapter_id, resolved_chapter_id) VALUES (1, 1, 'foreshadowing', 'pending', 'f1', 2, 3, 1, 0), (2, 1, 'foreshadowing', 'resolved', 'f2', 99, 3, 3, 2), (3, 2, 'foreshadowing', 'pending', 'f3', 1, 3, 1, 0)`)
	execSQL(`INSERT INTO story_arcs (id, novel_id, name, arc_type, status) VALUES (1, 1, 'arc', 'main', 'active')`)
	execSQL(`INSERT INTO arc_nodes (id, novel_id, story_arc_id, title, target_chapter, actual_chapter, status) VALUES (1, 1, 1, 'a1', 3, 0, 'pending'), (2, 1, 1, 'a2', 99, 1, 'pending')`)
	execSQL(`INSERT INTO reader_perspectives (id, novel_id, type, content, planted_chapter, revealed_chapter) VALUES (1, 1, 'known', 'p1', 1, 0), (2, 2, 'suspense', 'p2', 99, 1)`)
	execSQL(`INSERT INTO writing_log (id, date, novel_id, chapter_id, word_delta) VALUES (1, '2026-01-01', 1, 2, 100), (2, '2026-01-01', 1, 0, -50)`)
	execSQL(`INSERT INTO character_relations (id, novel_id, source_character_id, target_character_id, relation_describe, chapter_id, is_current) VALUES (1, 1, 1, 2, '朋友', 3, 1), (2, 1, 1, 2, '旧识', 0, 0)`)

	novelDir := filepath.Join(dataDir, "novels", "1")
	writeLegacyFile(t, novelDir, "chapters/001.md", "chapter one")
	writeLegacyFile(t, novelDir, "outlines/001.md", "outline one")

	if err := migrate.Run(db, slog.Default()); err != nil {
		t.Fatalf("migrate.Run: %v", err)
	}

	count := func(query string) int {
		t.Helper()
		var n int
		if err := db.Raw(query).Scan(&n).Error; err != nil {
			t.Fatalf("query %q: %v", query, err)
		}
		return n
	}
	for _, assertion := range []struct {
		name  string
		query string
	}{
		{"timeline IDs and reading target", `SELECT COUNT(*) FROM time_entries WHERE id = 1 AND target_reading_number = 2 AND source_chapter_id = 1 AND resolved_chapter_id IS NULL`},
		{"timeline resolved ID", `SELECT COUNT(*) FROM time_entries WHERE id = 2 AND target_reading_number = 99 AND source_chapter_id = 3 AND resolved_chapter_id = 2`},
		{"timeline cross-novel ID", `SELECT COUNT(*) FROM time_entries WHERE id = 3 AND target_reading_number = 1 AND source_chapter_id = 4`},
		{"arc node pending", `SELECT COUNT(*) FROM arc_nodes WHERE id = 1 AND target_reading_number = 3 AND actual_chapter_id IS NULL`},
		{"arc node IDs and reading target", `SELECT COUNT(*) FROM arc_nodes WHERE id = 2 AND target_reading_number = 99 AND actual_chapter_id = 1`},
		{"reader planted ID", `SELECT COUNT(*) FROM reader_perspectives WHERE id = 1 AND planted_chapter_id = 1 AND revealed_chapter_id IS NULL`},
		{"reader IDs and orphan", `SELECT COUNT(*) FROM reader_perspectives WHERE id = 2 AND planted_chapter_id IS NULL AND revealed_chapter_id = 4`},
		{"writing log ID", `SELECT COUNT(*) FROM writing_log WHERE id = 1 AND chapter_id = 2`},
		{"writing log zero", `SELECT COUNT(*) FROM writing_log WHERE id = 2 AND chapter_id IS NULL`},
		{"character relation ID", `SELECT COUNT(*) FROM character_relations WHERE id = 1 AND chapter_id = 3`},
		{"character relation zero", `SELECT COUNT(*) FROM character_relations WHERE id = 2 AND chapter_id IS NULL`},
	} {
		if n := count(assertion.query); n == 0 {
			t.Fatalf("迁移数据断言失败: %s", assertion.name)
		}
	}
	if n := count(`SELECT COUNT(*) FROM chapters WHERE (id = 1 AND sort_order = 1) OR (id = 2 AND sort_order = 2) OR (id = 3 AND sort_order = 3) OR (id = 4 AND sort_order = 1)`); n != 4 {
		t.Fatalf("sort_order 未按旧章节号初始化: matched=%d", n)
	}

	for _, rel := range []string{"chapters/id_1.md", "outlines/id_1.md", "volumes/.gitkeep"} {
		if _, err := os.Stat(filepath.Join(novelDir, rel)); err != nil {
			t.Fatalf("迁移后文件 %s 不存在: %v", rel, err)
		}
	}
	if _, err := os.Stat(filepath.Join(novelDir, "chapters/001.md")); !os.IsNotExist(err) {
		t.Fatal("旧章节文件应被改名移除")
	}
	repo, err := git.New(1, "", "", slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	if dirty, err := repo.HasUncommitted(); err != nil || dirty {
		t.Fatalf("迁移后的小说仓库应干净: dirty=%v err=%v", dirty, err)
	}

	for _, legacy := range []struct{ table, column string }{
		{"chapters", "chapter_number"},
		{"time_entries", "target_chapter"}, {"time_entries", "source_chapter"}, {"time_entries", "resolved_chapter"},
		{"arc_nodes", "target_chapter"}, {"arc_nodes", "actual_chapter"},
		{"reader_perspectives", "planted_chapter"}, {"reader_perspectives", "revealed_chapter"},
		{"writing_log", "chapter_number"}, {"character_relations", "chapter_number"},
	} {
		if db.Migrator().HasColumn(legacy.table, legacy.column) {
			t.Fatalf("旧列仍存在: %s.%s", legacy.table, legacy.column)
		}
	}
	if db.Migrator().HasIndex("chapters", "uk_novel_chapter") {
		t.Fatal("uk_novel_chapter 应随旧列删除")
	}
	if !db.Migrator().HasIndex("writing_log", "idx_writing_log_chapter_id") {
		t.Fatal("当前 writing_log.chapter_id 索引应在迁移后存在")
	}
	for _, table := range []string{"volumes", "migrate_state"} {
		if !db.Migrator().HasTable(table) {
			t.Fatalf("迁移后表不存在: %s", table)
		}
	}
	if n := count(`SELECT COUNT(*) FROM migrate_state WHERE migration = 'v1.6.0-chapter-id-refactor' AND status = 'done'`); n != 4 {
		t.Fatalf("v160 迁移步骤完成数 = %d, want 4", n)
	}

	if err := migrate.Run(db, slog.Default()); err != nil {
		t.Fatalf("幂等 migrate.Run: %v", err)
	}
	if err := db.Exec("DELETE FROM migrate_state").Error; err != nil {
		t.Fatal(err)
	}
	if err := migrate.Run(db, slog.Default()); err != nil {
		t.Fatalf("状态丢失后 migrate.Run: %v", err)
	}
	if !db.Migrator().HasColumn("time_entries", "source_chapter_id") ||
		db.Migrator().HasColumn("time_entries", "source_chapter") ||
		!db.Migrator().HasColumn("writing_log", "chapter_id") ||
		db.Migrator().HasColumn("writing_log", "chapter_number") {
		t.Fatal("状态丢失重跑不应回退最终的 id schema")
	}
}

func setupMigrationIntegrationEnv(t *testing.T) string {
	t.Helper()
	gitSrc, err := exec.LookPath("git")
	if err != nil {
		t.Skip("系统无 git，跳过")
	}
	dataDir := t.TempDir()
	t.Setenv("GOINK_TESTING", "1")
	t.Setenv("GOINK_DATA_DIR", dataDir)
	t.Setenv("HOME", t.TempDir())
	platform.ResetDataDirCache()
	config.Set(&config.AppConfig{})

	gitDir := filepath.Join(dataDir, "runtime", "git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	gitWrapper := fmt.Sprintf("#!/bin/sh\nexec %q \"$@\"\n", gitSrc)
	if err := os.WriteFile(filepath.Join(gitDir, "git"), []byte(gitWrapper), 0o755); err != nil {
		t.Fatal(err)
	}
	return dataDir
}

func openMigrationIntegrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "legacy.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func writeLegacyFile(t *testing.T, novelDir, rel, content string) {
	t.Helper()
	path := filepath.Join(novelDir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

//go:build cgo && upgrade

// Package upgrade_test treats the migration entry point as a process boundary.
// Historical schemas live exclusively in immutable archives, never in current
// model declarations or reconstructed DDL. Run from the worktree root with:
// go test -tags=upgrade -count=1 -timeout=15m ./internal/migrate/upgrade
package upgrade_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/sigpanic/goink/internal/chapter"
	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/llm/adapter"
	"github.com/sigpanic/goink/internal/migrate"
	"github.com/sigpanic/goink/internal/migrate/upgrade/fixture"
	"github.com/sigpanic/goink/internal/novel"
	"github.com/sigpanic/goink/internal/storage"
	"github.com/sigpanic/goink/internal/writing"
)

func TestUpgradeWorker(t *testing.T) {
	if os.Getenv("GOINK_UPGRADE_WORKER") != "1" {
		return
	}
	dir := os.Getenv("GOINK_DATA_DIR")
	if dir == "" || !filepath.IsAbs(dir) {
		t.Fatal("worker requires an absolute isolated data directory")
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	config.Set(&config.AppConfig{})
	db, err := storage.Open(config.GlobalDBPath(), log)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := storage.Close(db); err != nil {
			t.Error(err)
		}
	}()
	if err := migrate.Run(db, log); err != nil {
		t.Fatal(err)
	}
	settings, err := config.LoadSettings(db)
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("GOINK_UPGRADE_MODE") != "probe" {
		return
	}
	ctx := context.Background()
	novels := novel.NewStore(db, log)
	listed, err := novels.List(ctx, novel.ListNovelsOptions{})
	if err != nil || listed.Total != 2 {
		t.Fatalf("current novel Store: result=%v error=%v", listed, err)
	}
	chapters, err := chapter.NewStore(db, log).ListAllByNovel(ctx, nil, 1)
	if err != nil || len(chapters) != 2 || chapters[0].ID != 11 || chapters[1].ID != 12 {
		t.Fatalf("current chapter Store: chapters=%v error=%v", chapters, err)
	}
	if err := novels.DB.WithContext(ctx).Create(&novel.Novel{ID: 1001, Title: "升级后新建小说"}).Error; err != nil {
		t.Fatal(err)
	}
	settings.DefaultModelSelection = adapter.ModelSelection{
		AdapterName:  adapter.ProtocolOpenAIChat,
		ProviderName: "probe-provider",
		ModelID:      "upgrade-write",
	}
	if err := config.SaveSettings(db, settings); err != nil {
		t.Fatal(err)
	}
	writing.NewStore(db, log).LogChanges(ctx, 1, 11, 7, 2)
}

func TestHistoricalDirectUpgrades(t *testing.T) {
	samples := historicalFixtures(t)
	gitBin := requireGit(t)
	reference := t.TempDir()
	if err := runWorker(t, reference, gitBin, "migrate", "reference"); err != nil {
		t.Fatal(err)
	}
	wantSchema := readSchema(t, filepath.Join(reference, "novel-agent.db"))
	for _, sample := range samples {
		t.Run(sample.Tag+"/"+sample.Origin, func(t *testing.T) {
			t.Logf("historical source %s (%s), origin %s", sample.Tag, sample.Commit, sample.Origin)
			dir := prepare(t, sample)
			original := dumpDatabase(t, filepath.Join(dir, "novel-agent.db"), false)
			originalSchema := readSchema(t, filepath.Join(dir, "novel-agent.db"))
			originalFiles := fileHashes(t, filepath.Join(dir, "novels"))
			if err := runWorker(t, dir, gitBin, "migrate", "first"); err != nil {
				t.Fatal(err)
			}
			assertContract(t, dir, sample, gitBin)
			assertSchema(t, readSchema(t, filepath.Join(dir, "novel-agent.db")), wantSchema)
			assertBackups(t, dir, sample, original, originalFiles, requiresBackup(originalSchema), gitBin)
			before := dumpDatabase(t, filepath.Join(dir, "novel-agent.db"), true)
			files := fileHashes(t, filepath.Join(dir, "novels"))
			heads := gitHeads(t, dir, gitBin)
			if err := runWorker(t, dir, gitBin, "migrate", "restart"); err != nil {
				t.Fatal(err)
			}
			if after := dumpDatabase(t, filepath.Join(dir, "novel-agent.db"), true); !reflect.DeepEqual(before, after) {
				t.Fatalf("restart changed persisted rows\nbefore=%v\nafter=%v", before, after)
			}
			if after := fileHashes(t, filepath.Join(dir, "novels")); !reflect.DeepEqual(files, after) {
				t.Fatal("restart changed novel files")
			}
			if after := gitHeads(t, dir, gitBin); !reflect.DeepEqual(heads, after) {
				t.Fatal("restart created additional Git commits")
			}
			// Removing progress is an external corruption scenario, rather than a
			// call into the engine or a particular migration's private step.
			business := dumpDatabase(t, filepath.Join(dir, "novel-agent.db"), false)
			mutate(t, dir, "DELETE FROM migrate_state")
			if err := runWorker(t, dir, gitBin, "migrate", "lost-state"); err != nil {
				t.Fatal(err)
			}
			assertContract(t, dir, sample, gitBin)
			if after := dumpDatabase(t, filepath.Join(dir, "novel-agent.db"), false); !reflect.DeepEqual(business, after) {
				t.Fatal("lost-state restart changed business data")
			}
			if after := gitHeads(t, dir, gitBin); !reflect.DeepEqual(heads, after) {
				t.Fatal("lost-state restart changed Git history")
			}
			probeDate := time.Now().Format(time.DateOnly)
			if err := runWorker(t, dir, gitBin, "probe", "store-probe"); err != nil {
				t.Fatal(err)
			}
			withDB(t, filepath.Join(dir, "novel-agent.db"), func(db *sql.DB) {
				assertCount(t, db, "new novel", `SELECT COUNT(*) FROM novels WHERE id=1001 AND title='升级后新建小说'`, 1)
				assertCount(t, db, "updated settings", `SELECT COUNT(*) FROM app_config WHERE id=1 AND default_adapter_name='openai_chat' AND default_provider_name='probe-provider' AND default_model_id='upgrade-write'`, 1)
				assertCount(t, db, "new writing activity", `SELECT COUNT(*) FROM writing_log WHERE date IN (?, ?) AND novel_id=1 AND word_delta=5 AND words_added=7 AND words_deleted=2`, 1, probeDate, time.Now().Format(time.DateOnly))
			})
		})
	}
}

func TestHistoricalFailureRecovery(t *testing.T) {
	samples := historicalFixtures(t)
	var sample fixture.Fixture
	for _, f := range samples {
		if f.Tag == "v1.0.0" && f.Origin == "fresh" {
			sample = f
		}
	}
	if sample.Archive == "" {
		t.Fatal("recovery requires the oldest supported sample")
	}
	gitBin := requireGit(t)
	for _, failure := range []string{"backup", "file"} {
		t.Run(failure, func(t *testing.T) {
			dir := prepare(t, sample)
			before := dumpDatabase(t, filepath.Join(dir, "novel-agent.db"), false)
			blocker := filepath.Join(dir, "backups")
			if failure == "file" {
				blocker = filepath.Join(dir, "novels", "1", "volumes")
			}
			if err := os.WriteFile(blocker, []byte("blocked by a regular file"), 0644); err != nil {
				t.Fatal(err)
			}
			if err := runWorker(t, dir, gitBin, "migrate", "blocked"); err == nil {
				t.Fatal("upgrade unexpectedly succeeded with a blocked path")
			}
			if failure == "backup" {
				after := dumpDatabase(t, filepath.Join(dir, "novel-agent.db"), false)
				// migrate_state may have been created, but business schema/data must
				// remain unchanged when a required backup cannot be taken.
				if !reflect.DeepEqual(before, after) {
					t.Fatal("backup failure changed business schema/data")
				}
			} else {
				withDB(t, filepath.Join(dir, "novel-agent.db"), func(db *sql.DB) {
					assertCount(t, db, "partial migration retained old input", `SELECT COUNT(*) FROM chapters WHERE chapter_number=1`, 2)
				})
			}
			if err := os.Remove(blocker); err != nil {
				t.Fatal(err)
			}
			if err := runWorker(t, dir, gitBin, "migrate", "recovered"); err != nil {
				t.Fatal(err)
			}
			assertContract(t, dir, sample, gitBin)
			if err := runWorker(t, dir, gitBin, "migrate", "recovered-restart"); err != nil {
				t.Fatal(err)
			}
			assertContract(t, dir, sample, gitBin)
		})
	}
}

func TestHistoricalUncommittedFiles(t *testing.T) {
	samples := historicalFixtures(t)
	var sample fixture.Fixture
	for _, f := range samples {
		if f.Tag == "v1.0.0" && f.Origin == "fresh" {
			sample = f
		}
	}
	if sample.Archive == "" {
		t.Fatal("missing oldest supported sample")
	}
	dir := prepare(t, sample)
	gitBin := requireGit(t)
	want := "chapters：小说1，章节11\n未提交的正文修订\n"
	path := filepath.Join(dir, "novels", "1", "chapters", "001.md")
	if err := os.WriteFile(path, []byte(want), 0644); err != nil {
		t.Fatal(err)
	}
	if err := runWorker(t, dir, gitBin, "migrate", "dirty-upgrade"); err != nil {
		t.Fatal(err)
	}
	assertContract(t, dir, sample, gitBin, map[string]string{"novels/1/chapters/id_11.md": want})
	// A checkpoint must preserve the revision under its original path before
	// renaming. Looking at observable history avoids depending on commit text.
	repo := filepath.Join(dir, "novels", "1")
	history := gitOutput(t, gitBin, repo, "log", "--format=%H", "--", "chapters/001.md")
	preserved := false
	for _, hash := range strings.Fields(history) {
		cmd := exec.Command(gitBin, "show", hash+":chapters/001.md")
		cmd.Dir, cmd.Env = repo, workerEnv(dir, gitBin, "")
		if content, err := cmd.Output(); err == nil && string(content) == want {
			preserved = true
		}
	}
	if !preserved {
		t.Fatal("uncommitted revision was not checkpointed before file migration")
	}
}

func prepare(t *testing.T, sample fixture.Fixture) string {
	t.Helper()
	dir := t.TempDir()
	if err := fixture.Extract(filepath.Join("testdata", sample.Archive), dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "novel-agent.db")); err != nil {
		t.Fatal(err)
	}
	// Copy diagnostics before TempDir cleanup. Only synthetic fixture data is
	// saved, and successful cases leave no database artifacts behind.
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		report := os.Getenv("GOINK_UPGRADE_REPORT_DIR")
		if report == "" {
			return
		}
		dest := filepath.Join(report, safeName(t.Name()), "data")
		if err := copyTree(dir, dest); err != nil {
			t.Logf("save failure artifact: %v", err)
		}
	})
	return dir
}

func runWorker(t *testing.T, dir, gitBin, mode, attempt string) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestUpgradeWorker$", "-test.v")
	cmd.Env = workerEnv(dir, gitBin, mode)
	out, err := cmd.CombinedOutput()
	target := os.Getenv("GOINK_UPGRADE_TARGET_COMMIT")
	if target == "" {
		target = "local working tree"
	}
	out = append([]byte(fmt.Sprintf("case: %s\nattempt: %s\ntarget: %s\n", t.Name(), attempt, target)), out...)
	if report := os.Getenv("GOINK_UPGRADE_REPORT_DIR"); report != "" {
		path := filepath.Join(report, safeName(t.Name()), attempt+".log")
		if mkdirErr := os.MkdirAll(filepath.Dir(path), 0755); mkdirErr != nil {
			t.Fatal(mkdirErr)
		}
		if writeErr := os.WriteFile(path, out, 0644); writeErr != nil {
			t.Fatal(writeErr)
		}
	}
	if err != nil {
		return fmt.Errorf("%s worker: %w\n%s", attempt, err, out)
	}
	return nil
}

func workerEnv(dir, gitBin, mode string) []string {
	// Replace inherited values rather than relying on duplicate environment keys,
	// including Windows' case-insensitive environment names.
	values := map[string]string{"GOINK_DATA_DIR": dir, "GOINK_GIT_BIN": gitBin, "GOINK_TESTING": "1", "GOINK_UPGRADE_WORKER": "1", "GOINK_UPGRADE_MODE": mode,
		"GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": os.DevNull, "GIT_TERMINAL_PROMPT": "0"}
	env := []string{}
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		if _, replace := values[strings.ToUpper(key)]; !replace {
			env = append(env, item)
		}
	}
	for key, value := range values {
		env = append(env, key+"="+value)
	}
	return env
}

func requireGit(t *testing.T) string {
	t.Helper()
	bin, err := exec.LookPath("git")
	if err != nil {
		t.Fatal("real Git is required; upgrade coverage must not skip:", err)
	}
	return bin
}

func assertContract(t *testing.T, dir string, sample fixture.Fixture, gitBin string, contents ...map[string]string) {
	t.Helper()
	withDB(t, filepath.Join(dir, "novel-agent.db"), func(db *sql.DB) {
		for name, count := range map[string]int{"novels": 2, "chapters": 3, "time_entries": 2, "story_arcs": 1, "arc_nodes": 1, "reader_perspectives": 2, "characters": 2, "character_relations": 2, "preference_items": 2, "sessions": 1, "messages": 3} {
			assertCount(t, db, name+" row count", "SELECT COUNT(*) FROM "+quote(name), count)
		}
		assertCount(t, db, "novels preserved", `SELECT COUNT(*) FROM novels WHERE genre='玄幻' AND description='保留中文与 UTF-8' AND ((id=1 AND title='升级样本1') OR (id=2 AND title='升级样本2'))`, 2)
		assertCount(t, db, "chapter order and IDs", `SELECT COUNT(*) FROM chapters WHERE summary='章节摘要' AND word_count=8 AND ((id=11 AND novel_id=1 AND sort_order=1) OR (id=12 AND novel_id=1 AND sort_order=2) OR (id=21 AND novel_id=2 AND sort_order=1))`, 3)
		assertCount(t, db, "timeline stable IDs", `SELECT COUNT(*) FROM time_entries WHERE target_reading_number=999 AND ((id=1 AND source_chapter_id=11 AND resolved_chapter_id=12) OR (id=2 AND source_chapter_id=21 AND resolved_chapter_id IS NULL))`, 2)
		assertCount(t, db, "arc future reading position", `SELECT COUNT(*) FROM arc_nodes WHERE id=1 AND target_reading_number=999 AND actual_chapter_id=12`, 1)
		assertCount(t, db, "reader references and orphan", `SELECT COUNT(*) FROM reader_perspectives WHERE (id=1 AND planted_chapter_id=11 AND revealed_chapter_id IS NULL) OR (id=2 AND planted_chapter_id IS NULL AND revealed_chapter_id=21)`, 2)
		assertCount(t, db, "append-only relationship history", `SELECT COUNT(*) FROM character_relations WHERE source_character_id=31 AND target_character_id=32 AND relation_describe='关系历史' AND ((id=1 AND is_current=1 AND chapter_id=12) OR (id=2 AND is_current=0 AND chapter_id IS NULL))`, 2)
		assertCount(t, db, "global and novel preferences", `SELECT COUNT(*) FROM preference_items WHERE content='使用短句' AND ((id=1 AND is_global=1 AND novel_id=0) OR (id=2 AND is_global=0 AND novel_id=1))`, 2)
		assertCount(t, db, "settings", `SELECT COUNT(*) FROM app_config WHERE id=1 AND last_novel_id=2 AND selected_model_key='fixture-model' AND approval_mode='manual'`, 1)
		assertCount(t, db, "session", `SELECT COUNT(*) FROM sessions WHERE session_id='upgrade-session' AND title='历史对话' AND active_version=2`, 1)
		assertCount(t, db, "append-only audit messages", `SELECT COUNT(*) FROM messages WHERE session_id='upgrade-session' AND role='user' AND ((id=1 AND content='消息1' AND version=2 AND to_api=1 AND to_frontend=0) OR (id=2 AND content='消息2' AND version=1 AND to_api=0 AND to_frontend=1) OR (id=3 AND content='消息3' AND version=2 AND to_api=0 AND to_frontend=0))`, 3)
		assertCount(t, db, "daily writing counts", `SELECT COUNT(*) FROM writing_log WHERE date='2026-01-01' AND novel_id=1 AND word_delta=50 AND words_added=? AND words_deleted=?`, 1, sample.WritingAdded, sample.WritingDeleted)
		assertCount(t, db, "daily writing row count", `SELECT COUNT(*) FROM writing_log`, 1)
		assertCount(t, db, "unfinished migration states", `SELECT COUNT(*) FROM migrate_state WHERE status <> 'done'`, 0)
		var states int
		if err := db.QueryRow(`SELECT COUNT(*) FROM migrate_state`).Scan(&states); err != nil || states == 0 {
			t.Fatalf("missing completed progress: states=%d error=%v", states, err)
		}
		var integrity string
		if err := db.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil || integrity != "ok" {
			t.Fatalf("database integrity: %s %v", integrity, err)
		}
	})
	for _, c := range []struct{ id, novel int }{{11, 1}, {12, 1}, {21, 2}} {
		for _, kind := range []string{"chapters", "outlines"} {
			path := filepath.Join(dir, "novels", fmt.Sprint(c.novel), kind, fmt.Sprintf("id_%d.md", c.id))
			data, err := os.ReadFile(path)
			want := fmt.Sprintf("%s：小说%d，章节%d\n", kind, c.novel, c.id)
			if len(contents) > 0 {
				key := fmt.Sprintf("novels/%d/%s/id_%d.md", c.novel, kind, c.id)
				if override, exists := contents[0][key]; exists {
					want = override
				}
			}
			if err != nil || string(data) != want {
				t.Fatalf("file content %s: got %q error=%v", path, data, err)
			}
		}
	}
	for _, id := range []string{"1", "2"} {
		repo := filepath.Join(dir, "novels", id)
		gitOutput(t, gitBin, repo, "merge-base", "--is-ancestor", sample.GitHeads[id], "HEAD")
		gitOutput(t, gitBin, repo, "fsck", "--no-reflogs")
		if out := gitOutput(t, gitBin, repo, "status", "--porcelain"); out != "" {
			t.Fatalf("dirty upgraded repository %s: %s", id, out)
		}
		for _, kind := range []string{"chapters", "outlines"} {
			legacy, err := filepath.Glob(filepath.Join(repo, kind, "[0-9]*.md"))
			if err != nil || len(legacy) != 0 {
				t.Fatalf("legacy file names remain: %v %v", legacy, err)
			}
		}
	}
}

func requiresBackup(schema map[string]table) bool {
	for _, input := range [][2]string{{"novels", "dir_path"}, {"chapters", "chapter_number"}} {
		if _, exists := schema[input[0]].Columns[input[1]]; exists {
			return true
		}
	}
	return false
}

func historicalFixtures(t *testing.T) []fixture.Fixture {
	t.Helper()
	m, err := fixture.Load("testdata")
	if err != nil {
		t.Fatal(err)
	}
	samples := m.Historical(os.Getenv("UPGRADE_TARGET"))
	if len(samples) == 0 {
		t.Fatal("no historical fixtures before the requested target")
	}
	return samples
}

func assertBackups(t *testing.T, dir string, sample fixture.Fixture, original map[string][]string, originalFiles map[string]string, required bool, gitBin string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(dir, "backups"))
	if os.IsNotExist(err) && !required {
		return
	}
	if err != nil {
		t.Fatal("expected historical migration backup:", err)
	}
	complete := 0
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".tmp-") {
			t.Fatalf("incomplete backup: %s", entry.Name())
		}
		backup := filepath.Join(dir, "backups", entry.Name())
		got := dumpDatabase(t, filepath.Join(backup, "novel-agent.db"), false)
		if !reflect.DeepEqual(original, got) {
			t.Fatalf("backup %s does not preserve original business data/schema", entry.Name())
		}
		if files := fileHashes(t, filepath.Join(backup, "novels")); !reflect.DeepEqual(originalFiles, files) {
			t.Fatalf("backup %s changed original novel file contents", entry.Name())
		}
		for _, id := range []string{"1", "2"} {
			if head := gitOutput(t, gitBin, filepath.Join(backup, "novels", id), "rev-parse", "HEAD"); head != sample.GitHeads[id] {
				t.Fatal("backup does not preserve original Git head")
			}
		}
		complete++
	}
	if required && complete != 1 {
		t.Fatalf("upgrade requires exactly one complete backup, got %d", complete)
	}
}

type column struct {
	Name, Type string
	NotNull    int
	Default    sql.NullString
	PrimaryKey int
}
type index struct {
	Unique  bool
	Columns []string
}
type table struct {
	Columns map[string]column
	Indexes map[string]index
}

func readSchema(t *testing.T, path string) map[string]table {
	t.Helper()
	schema := map[string]table{}
	withDB(t, path, func(db *sql.DB) {
		for _, name := range tableNames(t, db, true) {
			current := table{Columns: map[string]column{}, Indexes: map[string]index{}}
			rows, err := db.Query("PRAGMA table_info(" + quote(name) + ")")
			if err != nil {
				t.Fatal(err)
			}
			for rows.Next() {
				var cid int
				var c column
				if err := rows.Scan(&cid, &c.Name, &c.Type, &c.NotNull, &c.Default, &c.PrimaryKey); err != nil {
					t.Fatal(err)
				}
				c.Type = strings.ToLower(c.Type)
				current.Columns[c.Name] = c
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			rows.Close()
			rows, err = db.Query("PRAGMA index_list(" + quote(name) + ")")
			if err != nil {
				t.Fatal(err)
			}
			for rows.Next() {
				var seq, unique, partial int
				var indexName, origin string
				if err := rows.Scan(&seq, &indexName, &unique, &origin, &partial); err != nil {
					t.Fatal(err)
				}
				if origin != "pk" {
					current.Indexes[indexName] = index{Unique: unique == 1}
				}
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			rows.Close()
			for indexName, idx := range current.Indexes {
				rows, err := db.Query("PRAGMA index_info(" + quote(indexName) + ")")
				if err != nil {
					t.Fatal(err)
				}
				for rows.Next() {
					var seq, cid int
					var columnName string
					if err := rows.Scan(&seq, &cid, &columnName); err != nil {
						t.Fatal(err)
					}
					idx.Columns = append(idx.Columns, columnName)
				}
				if err := rows.Err(); err != nil {
					t.Fatal(err)
				}
				rows.Close()
				current.Indexes[indexName] = idx
			}
			schema[name] = current
		}
	})
	return schema
}

func assertSchema(t *testing.T, got, want map[string]table) {
	t.Helper()
	for name, expected := range want {
		actual, exists := got[name]
		if !exists {
			t.Fatalf("missing current table %s", name)
		}
		for key, c := range expected.Columns {
			if actual.Columns[key] != c {
				t.Errorf("column %s.%s: got=%+v want=%+v", name, key, actual.Columns[key], c)
			}
		}
		for key, idx := range expected.Indexes {
			if !reflect.DeepEqual(actual.Indexes[key], idx) {
				t.Errorf("index %s.%s: got=%+v want=%+v", name, key, actual.Indexes[key], idx)
			}
		}
	}
	// Additive legacy fields may remain by design. Only fields retired by a
	// migration are forbidden; do not compare sqlite_master CREATE SQL verbatim.
	for _, retired := range [][2]string{{"novels", "dir_path"}, {"chapters", "chapter_number"}, {"time_entries", "source_chapter"}, {"time_entries", "resolved_chapter"}, {"time_entries", "target_chapter"}, {"arc_nodes", "target_chapter"}, {"arc_nodes", "actual_chapter"}, {"reader_perspectives", "planted_chapter"}, {"reader_perspectives", "revealed_chapter"}, {"writing_log", "chapter_id"}, {"writing_log", "chapter_number"}, {"character_relations", "chapter_number"}} {
		if _, exists := got[retired[0]].Columns[retired[1]]; exists {
			t.Errorf("retired column remains: %s.%s", retired[0], retired[1])
		}
	}
	if t.Failed() {
		t.FailNow()
	}
}

func withDB(t *testing.T, path string, check func(*sql.DB)) {
	t.Helper()
	dsn := (&url.URL{Scheme: "file", OmitHost: true, Path: filepath.ToSlash(path), RawQuery: "mode=ro"}).String()
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	check(db)
}

func assertCount(t *testing.T, db *sql.DB, name, query string, want int, args ...any) {
	t.Helper()
	var got int
	if err := db.QueryRow(query, args...).Scan(&got); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if got != want {
		t.Fatalf("%s: got %d want %d", name, got, want)
	}
}

func tableNames(t *testing.T, db *sql.DB, includeState bool) []string {
	t.Helper()
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		if name != "migrate_state" || includeState {
			names = append(names, name)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return names
}

func dumpDatabase(t *testing.T, path string, includeState bool) map[string][]string {
	t.Helper()
	result := map[string][]string{}
	withDB(t, path, func(db *sql.DB) {
		for _, name := range tableNames(t, db, includeState) {
			rows, err := db.Query("SELECT * FROM " + quote(name))
			if err != nil {
				t.Fatal(err)
			}
			columns, err := rows.Columns()
			if err != nil {
				t.Fatal(err)
			}
			// Including column names makes empty tables and schema changes visible.
			encoded, _ := json.Marshal(columns)
			result[name] = []string{string(encoded)}
			var records []string
			for rows.Next() {
				values := make([]any, len(columns))
				pointers := make([]any, len(columns))
				for i := range values {
					pointers[i] = &values[i]
				}
				if err := rows.Scan(pointers...); err != nil {
					t.Fatal(err)
				}
				encoded, err := json.Marshal(values)
				if err != nil {
					t.Fatal(err)
				}
				records = append(records, string(encoded))
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			rows.Close()
			sort.Strings(records)
			result[name] = append(result[name], records...)
		}
	})
	return result
}

func mutate(t *testing.T, dir, query string) {
	t.Helper()
	db, err := sql.Open("sqlite3", filepath.Join(dir, "novel-agent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(query); err != nil {
		t.Fatal(err)
	}
}

func quote(value string) string { return `"` + strings.ReplaceAll(value, `"`, `""`) + `"` }
func safeName(value string) string {
	return strings.NewReplacer("/", "_", "\\", "_", ":", "_").Replace(value)
}

func gitOutput(t *testing.T, bin, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = workerEnv(filepath.Dir(filepath.Dir(dir)), bin, "")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Git %v in %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

func gitHeads(t *testing.T, dir, gitBin string) map[string]string {
	t.Helper()
	heads := map[string]string{}
	for _, id := range []string{"1", "2"} {
		heads[id] = gitOutput(t, gitBin, filepath.Join(dir, "novels", id), "rev-parse", "HEAD")
	}
	return heads
}

func fileHashes(t *testing.T, dir string) map[string]string {
	t.Helper()
	hashes := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		hash, err := fixture.Hash(path)
		if err != nil {
			return err
		}
		hashes[filepath.ToSlash(rel)] = hash
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return hashes
}

func copyTree(src, dest string) error {
	return filepath.WalkDir(src, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0644)
	})
}

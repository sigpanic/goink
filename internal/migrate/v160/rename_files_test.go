//go:build cgo

package v160_test

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/sigpanic/goink/internal/chapter"
	"github.com/sigpanic/goink/internal/git"
	"github.com/sigpanic/goink/internal/migrate"
	"github.com/sigpanic/goink/internal/novel"
)

// TestRenameFilesMigration 模拟"1.1-1.4 已跑、1.5/1.6 未跑"的老用户库：
// schema 已含新列，novel 仓库里章节/大纲文件仍是旧 num 命名，sort_order 全 0。
// 完整跑 migrate.Run 链路，验证 sort_order 初始化 + volumes/ 创建 + 文件 rename + git commit + 幂等重跑。
func TestRenameFilesMigration(t *testing.T) {
	dataDir := setupMigrateEnv(t)
	db := openMigrateDB(t)
	log := slog.Default()

	models := []any{&novel.Novel{}, &chapter.Chapter{}}
	for _, m := range models {
		if err := db.AutoMigrate(m); err != nil {
			t.Fatalf("AutoMigrate %T: %v", m, err)
		}
	}

	exec := func(sql string) {
		t.Helper()
		if err := db.Exec(sql).Error; err != nil {
			t.Fatalf("exec: %s\n%v", sql, err)
		}
	}

	// 老数据：novel1 章 1/2/3 + num 不连续的 5（无文件，测源不存在跳过）
	exec(`INSERT INTO novels (id, title) VALUES (1,'n1')`)
	exec(`INSERT INTO chapters (id, novel_id, chapter_number, sort_order, title) VALUES
		(1,1,1,0,'c1'),(2,1,2,0,'c2'),(3,1,3,0,'c3'),(5,1,5,0,'c5')`)

	// 假 novel 仓库：正文 1/2 有文件（3/5 无）、大纲只有 1 有（2/3/5 无，测大纲缺失跳过）
	novelDir := filepath.Join(dataDir, "novels", "1")
	mustWrite := func(rel, content string) {
		t.Helper()
		p := filepath.Join(novelDir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("chapters/001.md", "ch1 content")
	mustWrite("chapters/002.md", "ch2 content")
	mustWrite("outlines/001.md", "outline1")

	if err := migrate.Run(db, log); err != nil {
		t.Fatalf("migrate.Run: %v", err)
	}

	exists := func(rel string) bool {
		_, err := os.Stat(filepath.Join(novelDir, rel))
		return err == nil
	}

	// 1. sort_order 初始化 = chapter_number（含不连续 num=5）；不存在的 num 也初始化
	var so []struct{ ID, Num, Sort int }
	if err := db.Raw("SELECT id, chapter_number AS num, sort_order AS sort FROM chapters ORDER BY id").Scan(&so).Error; err != nil {
		t.Fatal(err)
	}
	for _, r := range so {
		if r.Sort != r.Num {
			t.Fatalf("sort_order 错误: id=%d num=%d sort=%d", r.ID, r.Num, r.Sort)
		}
	}

	// 2. 文件 rename：正文 001.md→1.md、002.md→2.md；大纲 001.md→1.md
	if !exists("chapters/1.md") || exists("chapters/001.md") {
		t.Fatal("chapters/1.md 应存在且 001.md 应消失")
	}
	if !exists("chapters/2.md") || exists("chapters/002.md") {
		t.Fatal("chapters/2.md 应存在且 002.md 应消失")
	}
	if !exists("outlines/1.md") || exists("outlines/001.md") {
		t.Fatal("outlines/1.md 应存在且 001.md 应消失")
	}
	// 源不存在的章节（3/5）不产生目标文件
	if exists("chapters/5.md") {
		t.Fatal("chapters/5.md 不应存在（源 005.md 不存在）")
	}
	// 内容完整迁移
	data, _ := os.ReadFile(filepath.Join(novelDir, "chapters", "1.md"))
	if string(data) != "ch1 content" {
		t.Fatalf("chapters/1.md 内容错误: %q", data)
	}

	// 3. volumes/ + .gitkeep
	if !exists("volumes/.gitkeep") {
		t.Fatal("volumes/.gitkeep 应存在")
	}

	// 4. git commit：rename + volumes 已提交，工作区干净
	repo, err := git.New(1, "", "", log)
	if err != nil {
		t.Fatal(err)
	}
	uncommitted, err := repo.HasUncommitted()
	if err != nil {
		t.Fatal(err)
	}
	if uncommitted {
		t.Fatal("git 工作区应有未提交变更（rename + volumes 应已 commit）")
	}

	// 幂等重跑：状态不变、无错误、无新增 git 变更
	if err := migrate.Run(db, log); err != nil {
		t.Fatalf("幂等重跑 migrate.Run: %v", err)
	}
	if exists("chapters/001.md") {
		t.Fatal("幂等重跑后 chapters/001.md 不应复活")
	}
	if !exists("chapters/1.md") {
		t.Fatal("幂等重跑后 chapters/1.md 丢失")
	}
	uncommitted, err = repo.HasUncommitted()
	if err != nil {
		t.Fatal(err)
	}
	if uncommitted {
		t.Fatal("幂等重跑后 git 不应有新增变更")
	}
}

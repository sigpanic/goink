//go:build cgo

package v160

import (
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/chapter"
	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/git"
	"github.com/sigpanic/goink/internal/novel"
	"github.com/sigpanic/goink/internal/platform"
)

func TestMigrateRenameFilesCanRerunBeforeStateRecorded(t *testing.T) {
	gitBin, err := exec.LookPath("git")
	if err != nil {
		t.Skip("系统无 git，跳过")
	}
	dataDir := t.TempDir()
	t.Setenv("GOINK_TESTING", "1")
	t.Setenv("GOINK_DATA_DIR", dataDir)
	t.Setenv("GOINK_GIT_BIN", gitBin)
	t.Setenv("HOME", t.TempDir())
	platform.ResetDataDirCache()
	config.Set(&config.AppConfig{})

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "migrate.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	for _, model := range []any{&novel.Novel{}, &chapter.Chapter{}} {
		if err := db.AutoMigrate(model); err != nil {
			t.Fatalf("AutoMigrate %T: %v", model, err)
		}
	}
	if err := db.Exec("ALTER TABLE chapters ADD COLUMN chapter_number INTEGER NOT NULL DEFAULT 0").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO novels (id, title) VALUES (1, 'n1')").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO chapters (id, novel_id, chapter_number, sort_order, title) VALUES (42, 1, 1, 0, 'c1')").Error; err != nil {
		t.Fatal(err)
	}

	novelDir := filepath.Join(dataDir, "novels", "1")
	for rel, content := range map[string]string{
		"chapters/001.md": "chapter content",
		"outlines/001.md": "outline content",
	} {
		path := filepath.Join(novelDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	log := slog.Default()
	if err := migrateRenameFiles(db, log); err != nil {
		t.Fatalf("首次文件迁移: %v", err)
	}
	// 这里故意不写 migrate_state，模拟文件和 Git 副作用已经完成、进程却在框架
	// 写入 step done 前中断。第二次必须安全重跑同一函数。
	if err := migrateRenameFiles(db, log); err != nil {
		t.Fatalf("状态未记录时重跑文件迁移: %v", err)
	}

	for _, rel := range []string{"chapters/id_42.md", "outlines/id_42.md", "volumes/.gitkeep"} {
		if _, err := os.Stat(filepath.Join(novelDir, rel)); err != nil {
			t.Fatalf("重跑后文件 %s 不存在: %v", rel, err)
		}
	}
	if _, err := os.Stat(filepath.Join(novelDir, "chapters/001.md")); !os.IsNotExist(err) {
		t.Fatal("重跑后旧章节文件不应复活")
	}
	var sortOrder int
	if err := db.Table("chapters").Select("sort_order").Where("id = ?", 42).Scan(&sortOrder).Error; err != nil {
		t.Fatal(err)
	}
	if sortOrder != 1 {
		t.Fatalf("sort_order = %d, want 1", sortOrder)
	}
	repo, err := git.New(1, "", "", log)
	if err != nil {
		t.Fatal(err)
	}
	if dirty, err := repo.HasUncommitted(); err != nil || dirty {
		t.Fatalf("重跑后 Git 工作区应干净: dirty=%v err=%v", dirty, err)
	}
}

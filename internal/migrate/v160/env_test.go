//go:build cgo

package v160_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/platform"
)

// setupMigrateEnv 准备真实迁移环境（两个迁移测试共享）：
//   - GOINK_DATA_DIR 指向临时目录（platform.DataDir 由它覆盖）
//   - bundled git 二进制放到 DataDir/runtime/git/git（GOINK_TESTING 模式只认这个路径）
//   - config 单例置非 nil（git.New 依赖 config.Get()）
//
// 返回 dataDir。
func setupMigrateEnv(t *testing.T) string {
	t.Helper()
	gitSrc, err := exec.LookPath("git")
	if err != nil {
		t.Skip("系统无 git，跳过")
	}
	dataDir := t.TempDir()
	t.Setenv("GOINK_TESTING", "1")
	t.Setenv("GOINK_DATA_DIR", dataDir)
	platform.ResetDataDirCache()
	config.Set(&config.AppConfig{})

	bin, err := os.ReadFile(gitSrc)
	if err != nil {
		t.Fatal(err)
	}
	gitDst := filepath.Join(dataDir, "runtime", "git")
	if err := os.MkdirAll(gitDst, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDst, "git"), bin, 0o755); err != nil {
		t.Fatal(err)
	}
	return dataDir
}

// openMigrateDB 打开独立的 sqlite 测试库（每个测试独立文件）。
func openMigrateDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "migrate.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return db
}

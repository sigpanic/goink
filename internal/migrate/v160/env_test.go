//go:build cgo

package v160_test

import (
	"path/filepath"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/testsupport"
)

// setupMigrateEnv 准备真实迁移环境（两个迁移测试共享）：
//   - GOINK_DATA_DIR 指向临时目录（platform.DataDir 由它覆盖）
//   - GOINK_GIT_BIN 指向系统 git（测试模式下 ResolveGit 优先返回它）
//   - config 单例置非 nil（git.New 依赖 config.Get()）
//
// 返回 dataDir。
func setupMigrateEnv(t *testing.T) string {
	t.Helper()
	gitSrc := testsupport.RequireGit(t)
	dataDir := testsupport.Isolate(t)
	t.Setenv("GOINK_TESTING", "1")
	// 迁移会 git commit 文件改名，必须有真实 git；测试模式默认只认 DataDir 下的
	// bundled git，故显式注入系统 git，而非伪造一份 bundled 布局（Windows 上伪造不出来）。
	t.Setenv("GOINK_GIT_BIN", gitSrc)
	// 隔离用户全局 Git 配置（如 commit.gpgSign=true）；迁移会在临时小说仓库中
	// 创建提交，测试不应依赖开发机的 GPG agent 或身份配置。
	t.Setenv("HOME", t.TempDir())
	config.Set(&config.AppConfig{})
	return dataDir
}

// openMigrateDB 打开独立的 sqlite 测试库（每个测试独立文件）。
func openMigrateDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "migrate.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	// 显式关闭连接：Windows 无法删除仍被占用的 db 文件，不关会让 t.TempDir() 清理失败。
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

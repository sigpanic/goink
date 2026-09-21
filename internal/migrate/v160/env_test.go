//go:build cgo

package v160_test

import (
	"fmt"
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
	// 隔离用户全局 Git 配置（如 commit.gpgSign=true）；迁移会在临时小说仓库中
	// 创建提交，测试不应依赖开发机的 GPG agent 或身份配置。
	t.Setenv("HOME", t.TempDir())
	platform.ResetDataDirCache()
	config.Set(&config.AppConfig{})

	gitDst := filepath.Join(dataDir, "runtime", "git")
	if err := os.MkdirAll(gitDst, 0o755); err != nil {
		t.Fatal(err)
	}
	// 不能直接复制 git 可执行文件：Git 会据 argv[0] 推导自身的 libexec 路径，复制后
	// 在临时 runtime 目录找不到资源。包装器保留系统 git 的真实位置，同时满足
	// GOINK_TESTING 对 DataDir/runtime/git/git 的查找约定。
	gitWrapper := fmt.Sprintf("#!/bin/sh\nexec %q \"$@\"\n", gitSrc)
	if err := os.WriteFile(filepath.Join(gitDst, "git"), []byte(gitWrapper), 0o755); err != nil {
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
	// 显式关闭连接：Windows 无法删除仍被占用的 db 文件，不关会让 t.TempDir() 清理失败。
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

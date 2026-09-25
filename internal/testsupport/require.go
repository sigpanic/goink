package testsupport

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
)

// RequireEnvOrExit 断言进程级必需的环境变量已设置，未设置时向 stderr 打印 FATAL 并
// 以状态码 1 退出，返回变量值。
//
// 用于 TestMain 这类拿不到 *testing.T、无法 t.Skip 的场合：e2e 必须在跑任何用例前
// 就拒绝未标记、未隔离的运行环境，否则会写到真实数据目录。
func RequireEnvOrExit(name, hint string) string {
	value := os.Getenv(name)
	if value == "" {
		fmt.Fprintf(os.Stderr, "FATAL: %s env var not set; %s\n", name, hint)
		os.Exit(1)
	}
	return value
}

// RequireEnv 返回环境变量 name 的值；未设置时跳过当前测试，返回值为空串。
func RequireEnv(t *testing.T, name string) string {
	t.Helper()

	value := os.Getenv(name)
	if value == "" {
		t.Skipf("%s not set", name)
	}
	return value
}

// RequireGit 返回系统 git 的可执行文件路径；系统无 git 时跳过当前测试。
//
// 调用方通常紧接着把返回值注入 GOINK_GIT_BIN：测试模式下 ResolveGit 默认只认
// DataDir 下的 bundled git，而测试代码造不出 bundled 布局（Windows 上尤其如此）。
func RequireGit(t *testing.T) string {
	t.Helper()

	gitBin, err := exec.LookPath("git")
	if err != nil {
		t.Skip("系统无 git，跳过")
	}
	return gitBin
}

package platform

import (
	"os"
	"strings"
	"testing"
)

// TestDataDir_TestBinaryFallback 验证兜底：测试二进制里未显式设置 GOINK_DATA_DIR
// 时，DataDir 不得返回真实数据目录（~/Goink），而是回退到进程独立的临时目录。
func TestDataDir_TestBinaryFallback(t *testing.T) {
	// 空值等价于未设置：resolveDataDir 只在非空时才采用 GOINK_DATA_DIR。
	t.Setenv("GOINK_DATA_DIR", "")

	got := DataDir()
	if got == "" {
		t.Fatal("DataDir() 返回空字符串")
	}
	if !strings.HasPrefix(got, os.TempDir()) {
		t.Fatalf("测试二进制下 DataDir() 应回退到临时目录，实际: %q", got)
	}
}

// TestDataDir_NoCacheInTestBinary 验证测试二进制下 DataDir 不做缓存：切换
// GOINK_DATA_DIR 后立即生效，无需调用 ResetDataDirCache。
func TestDataDir_NoCacheInTestBinary(t *testing.T) {
	first := t.TempDir()
	t.Setenv("GOINK_DATA_DIR", first)
	if got := DataDir(); got != first {
		t.Fatalf("DataDir() = %q, want %q", got, first)
	}

	second := t.TempDir()
	t.Setenv("GOINK_DATA_DIR", second)
	if got := DataDir(); got != second {
		t.Fatalf("切换 GOINK_DATA_DIR 后 DataDir() = %q, want %q（缓存未失效）", got, second)
	}
}

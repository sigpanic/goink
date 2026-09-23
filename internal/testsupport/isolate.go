// Package testsupport 提供测试共用的环境隔离与环境前提检查辅助函数。
// 只应被 _test.go 文件引用，不要在生产代码里 import。
package testsupport

import (
	"testing"

	"github.com/sigpanic/goink/internal/platform"
)

// Isolate 把平台数据目录隔离到 t 的临时目录，返回该目录路径。
//
// 它同时重置 DataDir 缓存并在测试结束时再次重置：DataDir 在测试二进制下本就不做
// 缓存，这两次重置是为了让本函数不依赖 DataDir 的缓存实现细节。
//
// 需要额外隔离项（例如隔离 HOME 以切断用户全局 git 配置）的测试，在本函数之后
// 自行调用 t.Setenv 追加。
func Isolate(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	t.Setenv("GOINK_DATA_DIR", dir)
	platform.ResetDataDirCache()
	t.Cleanup(platform.ResetDataDirCache)

	return dir
}

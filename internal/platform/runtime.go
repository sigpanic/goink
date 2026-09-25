package platform

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// AppDir 返回当前可执行文件所在的目录。
func AppDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("platform: 获取可执行文件路径失败: %w", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return "", fmt.Errorf("platform: 解析可执行文件符号链接失败: %w", err)
	}
	return filepath.Dir(exe), nil
}

// ResolveGit 返回 git 可执行文件的路径。
// 搜索顺序: app 自带 runtime/git/ → 用户数据目录 runtime/git/ → 系统 PATH。
// 每个候选路径都会验证可执行性（git --version），不可用则跳过继续 fallback。
//
// 当环境变量 GOINK_E2E_STRICT=1 时，仅从 DataDir 的 bundled 路径查找，
// 不 fallback 到系统 PATH，找不到直接报错。E2E 专属，确保测试使用 bundled git。
//
// 当环境变量 GOINK_GIT_BIN 非空时，优先返回它指定的路径（需通过 git --version 验证），
// 优先级高于以上所有查找顺序。与 GOINK_DATA_DIR 同级：供测试显式注入 git 路径。
func ResolveGit() (string, error) {
	// 0. 显式注入：测试指定 git 路径，优先级最高，验证失败直接报错不静默 fallback
	if bin := os.Getenv("GOINK_GIT_BIN"); bin != "" {
		if err := verifyGit(bin); err != nil {
			return "", fmt.Errorf("git: GOINK_GIT_BIN 指定的 git 不可用 (%s): %w", bin, err)
		}
		return bin, nil
	}

	// GOINK_E2E_STRICT 模式：只查 DataDir bundled 路径，不做任何 fallback
	if os.Getenv("GOINK_E2E_STRICT") != "" {
		path := dataDirBundledGitPath()
		if verifyGit(path) == nil {
			return path, nil
		}
		return "", fmt.Errorf("git: GOINK_E2E_STRICT 模式下未找到 bundled git (%s)", path)
	}

	// 1. app 自带的 bundled git
	if appDir, err := AppDir(); err == nil {
		if path := bundledGitPath(appDir); verifyGit(path) == nil {
			return path, nil
		}
	}

	// 2. 用户数据目录下的 runtime/git/ (开发模式或手动安装)
	dataGit := filepath.Join(DataDir(), "runtime", "git", gitBinName())
	if verifyGit(dataGit) == nil {
		return dataGit, nil
	}

	// 3. 系统 PATH
	if path, err := exec.LookPath("git"); err == nil {
		return path, nil
	}

	return "", fmt.Errorf("git: 找不到可用的 git 可执行文件，请安装 Git")
}

// dataDirBundledGitPath 返回 DataDir 下 bundled git 的路径，
// 使用与生产环境相同的目录结构（Windows 为 MinGit 的 mingw64/bin/git.exe）。
func dataDirBundledGitPath() string {
	dataDir := DataDir()
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(dataDir, "runtime", "git", "mingw64", "bin", "git.exe")
	default:
		return filepath.Join(dataDir, "runtime", "git", "git")
	}
}

// verifyGit 验证 git 可执行文件是否存在且能正常运行。
func verifyGit(path string) error {
	if _, err := os.Stat(path); err != nil {
		return err
	}
	cmd := exec.Command(path, "--version")
	SetPlatformAttr(cmd)
	return cmd.Run()
}

// gitBinName 返回当前平台 git 二进制文件名。
func gitBinName() string {
	if runtime.GOOS == "windows" {
		return "git.exe"
	}
	return "git"
}

// ResolveOnnxLib 返回 ONNX Runtime 动态库的路径。
// 优先 app 自带的 runtime/，然后用户数据目录 runtime/，最后系统路径。
//
// 当环境变量 GOINK_E2E_STRICT=1 时，仅从 DataDir 的 runtime/ 查找，
// 不 fallback 到系统路径，找不到直接报错。E2E 专属，确保测试使用 bundled ONNX。
func ResolveOnnxLib() (string, error) {
	libName := onnxLibName()

	// GOINK_E2E_STRICT 模式：只查 DataDir runtime 路径，不做任何 fallback
	if os.Getenv("GOINK_E2E_STRICT") != "" {
		p := filepath.Join(DataDir(), "runtime", libName)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
		return "", fmt.Errorf("platform: GOINK_E2E_STRICT 模式下未找到 ONNX Runtime (%s)", p)
	}

	appDir, err := AppDir()
	if err == nil {
		for _, dir := range bundledRuntimeDirs(appDir) {
			p := filepath.Join(dir, libName)
			if _, err := os.Stat(p); err == nil {
				return p, nil
			}
		}
	}

	dataRuntime := filepath.Join(DataDir(), "runtime", libName)
	if _, err := os.Stat(dataRuntime); err == nil {
		return dataRuntime, nil
	}

	for _, p := range systemOnnxPaths(libName) {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("platform: ONNX Runtime 库未找到（%s）", libName)
}

var (
	dataDirOnce  sync.Once
	dataDirCache string
)

// DataDir 返回应用数据目录（绝对路径）。
//
// Windows: exe 目录可写时返回 exe 目录（portable 模式，旧用户零迁移）；
// 不可写（如装到 Program Files）时 fallback 到 %LOCALAPPDATA%\Goink。
// 其他平台: 返回 ~/Goink/。
// 开发模式下 exe 位于临时目录时，所有平台统一返回 ~/Goink/。
// 环境变量 GOINK_DATA_DIR 可覆盖以上逻辑，用于集成测试。
//
// 结果用 sync.Once 缓存，避免每次调用都做可写性检测。
// 测试二进制下不做缓存：测试用 t.Setenv 切换 GOINK_DATA_DIR，而 t.Setenv 只在测试
// 结束时恢复环境变量、不会恢复这里的缓存，缓存会让后一个测试拿到前一个测试已删除
// 的临时目录。
func DataDir() string {
	if testing.Testing() {
		return resolveDataDir()
	}
	dataDirOnce.Do(func() {
		dataDirCache = resolveDataDir()
	})
	return dataDirCache
}

// ResetDataDirCache 重置 DataDir 的 sync.Once 缓存，供测试使用。
// 测试用例用 t.Setenv("GOINK_DATA_DIR", ...) 设置不同的临时目录后，
// 需要调用此函数重置缓存，否则 DataDir() 仍返回第一次调用的缓存值。
// 生产代码不应调用此函数。
func ResetDataDirCache() {
	dataDirOnce = sync.Once{}
	dataDirCache = ""
}

// resolveDataDir 实际计算 DataDir，由 DataDir 调用。
func resolveDataDir() string {
	if dir := os.Getenv("GOINK_DATA_DIR"); dir != "" {
		return dir
	}
	// 兜底：测试二进制绝不能落到真实数据目录。未显式设置 GOINK_DATA_DIR 时回退到
	// 进程独立的临时目录，保证测试不会写到真实数据（小说 git 仓库、SQLite 库）。
	// 这里只保证"不落到真实目录"：该路径是可写的普通临时路径，调用方（如
	// config.Load 里的 MkdirAll）会把它创建出来，遗留的临时目录交由系统清理。
	// 按 PID 区分是因为 go test 会并行运行多个包的测试二进制。
	if testing.Testing() {
		return filepath.Join(os.TempDir(), fmt.Sprintf("goink-test-%d", os.Getpid()))
	}
	if runtime.GOOS == "windows" {
		if dir, err := AppDir(); err == nil {
			tmp := os.TempDir()
			if !strings.HasPrefix(strings.ToLower(dir), strings.ToLower(tmp)) {
				if isWritableDir(dir) {
					return dir
				}
				// exe 目录不可写（如装到 Program Files），fallback 到 %LOCALAPPDATA%\Goink
				// 直接读 LOCALAPPDATA 环境变量，语义明确，不依赖 os.UserCacheDir() 的 "Cache" 语义。
				// 不用 os.UserConfigDir()，它在 Windows 返回 %AppData%（Roaming 漫游目录），
				// 数据库/日志这些不该漫游的数据应该放 %LOCALAPPDATA%（Local 本地目录）。
				if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
					return filepath.Join(localAppData, "Goink")
				}
			}
		}
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Goink")
}

// isWritableDir 检测目录是否可写：创建临时文件测试，删除后返回。
// 比 os.MkdirAll 更准确：能区分"目录存在但不可写"和"目录可写"两种情况。
func isWritableDir(dir string) bool {
	f, err := os.CreateTemp(dir, ".goink-write-test-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}

// bundledGitPath 返回自带的 git 完整路径。
func bundledGitPath(appDir string) string {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(appDir, "runtime", "git", "mingw64", "bin", "git.exe")
	case "darwin":
		// macOS .app bundle 中 runtime 在 Contents/Resources/ 下
		return filepath.Join(appDir, "..", "Resources", "runtime", "git", "git")
	default:
		return filepath.Join(appDir, "runtime", "git", "git")
	}
}

// bundledRuntimeDirs 返回自带的 runtime 目录列表，按优先级排列。
func bundledRuntimeDirs(appDir string) []string {
	switch runtime.GOOS {
	case "darwin":
		// macOS .app bundle: runtime 在 Contents/Resources/，
		// AppDir 返回 Contents/MacOS/，所以用 ../Resources/runtime/
		return []string{
			filepath.Join(appDir, "..", "Resources", "runtime"),
			filepath.Join(appDir, "runtime"),
		}
	default:
		return []string{filepath.Join(appDir, "runtime")}
	}
}

// BundledModelsDir 返回打包自带的模型目录路径（绝对路径）。
func BundledModelsDir(appDir string) string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(appDir, "..", "Resources", "runtime", "models")
	default:
		return filepath.Join(appDir, "runtime", "models")
	}
}

func onnxLibName() string {
	switch runtime.GOOS {
	case "windows":
		return "onnxruntime.dll"
	case "darwin":
		return "libonnxruntime.dylib"
	default:
		return "libonnxruntime.so"
	}
}

func systemOnnxPaths(lib string) []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{
			"/usr/local/lib/" + lib,
			"/usr/lib/" + lib,
		}
	default:
		return []string{
			"/usr/lib/" + lib,
			"/usr/local/lib/" + lib,
		}
	}
}

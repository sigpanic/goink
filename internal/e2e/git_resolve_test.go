//go:build cgo && e2e

package e2e

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/sigpanic/goink/internal/platform"
)

func TestResolveGit_NoSystemFallback(t *testing.T) {
	// In GOINK_TESTING mode, ResolveGit must NOT use system PATH fallback
	// Even if system git is available, it must return the bundled path
	gitBin, err := platform.ResolveGit()
	if err != nil {
		t.Fatalf("ResolveGit() failed: %v", err)
	}
	dataDir := filepath.Clean(platform.DataDir())
	if !strings.HasPrefix(filepath.Clean(gitBin), dataDir) {
		t.Errorf("ResolveGit() returned non-bundled path: %s, expected under %s", gitBin, dataDir)
	}
}

func TestResolveGit_ReturnsBundledPath(t *testing.T) {
	gitBin, err := platform.ResolveGit()
	if err != nil {
		t.Fatalf("ResolveGit() failed: %v", err)
	}

	// The returned path should NOT be a system PATH binary
	// On CI, system git is hidden, so ResolveGit must find bundled git
	if filepath.IsAbs(gitBin) {
		// Good, it's an absolute path (bundled git)
		t.Logf("Resolved git: %s", gitBin)
	} else {
		t.Errorf("ResolveGit() returned relative path: %s, expected absolute bundled path", gitBin)
	}

	// Verify it's in the runtime/git directory structure
	dataDir := platform.DataDir()
	if !strings.Contains(gitBin, "runtime") {
		t.Errorf("Resolved git path %s doesn't contain 'runtime', expected bundled path under %s", gitBin, dataDir)
	}
}

func TestResolveGit_BinaryWorks(t *testing.T) {
	gitBin, err := platform.ResolveGit()
	if err != nil {
		t.Fatalf("ResolveGit() failed: %v", err)
	}

	out, err := exec.Command(gitBin, "--version").CombinedOutput()
	if err != nil {
		t.Fatalf("bundled git --version failed: %v\n%s", err, out)
	}

	versionStr := string(out)
	if !strings.Contains(versionStr, "git version") {
		t.Errorf("unexpected git --version output: %s", versionStr)
	}
	t.Logf("Bundled git version: %s", strings.TrimSpace(versionStr))
}

func TestResolveGit_PlatformSpecificPath(t *testing.T) {
	gitBin, err := platform.ResolveGit()
	if err != nil {
		t.Fatalf("ResolveGit() failed: %v", err)
	}

	var expectedSubpath string
	switch runtime.GOOS {
	case "windows":
		expectedSubpath = filepath.Join("runtime", "git", "mingw64", "bin", "git.exe")
	case "darwin":
		// macOS: could be app bundle or DataDir fallback
		expectedSubpath = filepath.Join("runtime", "git", "git")
	default:
		expectedSubpath = filepath.Join("runtime", "git", "git")
	}

	if !strings.Contains(gitBin, expectedSubpath) {
		t.Errorf("Resolved git path %s doesn't contain expected subpath %q", gitBin, expectedSubpath)
	}
}

func TestResolveOnnxLib_ReturnsPath(t *testing.T) {
	onnxLib, err := platform.ResolveOnnxLib()
	if err != nil {
		t.Fatalf("ResolveOnnxLib() failed: %v", err)
	}
	t.Logf("Resolved ONNX lib: %s", onnxLib)

	// Verify it contains the expected library name
	var expectedName string
	switch runtime.GOOS {
	case "windows":
		expectedName = "onnxruntime.dll"
	case "darwin":
		expectedName = "libonnxruntime"
	default:
		expectedName = "libonnxruntime"
	}
	if !strings.Contains(onnxLib, expectedName) {
		t.Errorf("ONNX lib path %s doesn't contain %q", onnxLib, expectedName)
	}
}

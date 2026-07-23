package git

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/sigpanic/goink/internal/config"
)

// ── 文件路径 ──────────────────────────────────────────────
// 此处都是相对路径，位于小说目录之下的，小说目录由config决定
func ChapterPath(num int) string {
	return fmt.Sprintf("chapters/%03d.md", num)
}

func GoinkPath() string {
	return "goink.md"
}

func CoverPath() string {
	return "cover.jpg"
}

func PlanPath(scope string) string {
	return fmt.Sprintf("plans/%s.md", scope)
}

func OutlinePath(num int) string {
	return fmt.Sprintf("outlines/%03d.md", num)
}

// ── 文件读写 ──────────────────────────────────────────────
// path 为相对于小说仓库根目录的路径，如 "chapters/001.md"、"goink.md"。

func ReadFile(novelID int64, path string) (string, error) {
	fullPath, err := ResolvePath(path, novelID)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(fullPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("%w: %s", os.ErrNotExist, path)
		}
		return "", fmt.Errorf("git: read %s: %w", path, err)
	}
	return string(data), nil
}

func WriteFile(novelID int64, path, content string) error {
	fullPath, err := ResolvePath(path, novelID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return fmt.Errorf("git: mkdir for %s: %w", path, err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("git: write %s: %w", path, err)
	}
	return nil
}

var ErrPathEscape = errors.New("git: path escapes novel directory")

func novelDir(novelID int64) string {
	return config.NovelDirPath(novelID)
}

// ResolvePath 将用户输入路径解析为真实文件系统路径。
// ~/.goink/ 展开到用户目录；其他相对路径基于小说目录。
func ResolvePath(path string, novelID int64) (string, error) {
	if strings.HasPrefix(path, "~/.goink/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("git: 获取用户目录失败: %w", err)
		}
		base := filepath.Join(home, ".goink")
		rel := strings.TrimPrefix(path, "~/.goink/")
		return SafePath(base, rel)
	}

	dir := novelDir(novelID)
	return SafePath(dir, path)
}

// SafePath 对给定的上级目录和相对路径求最终路径，如果路径跳出上级目录则返回 error。
func SafePath(base, rel string) (string, error) {
	absBase, err := filepath.Abs(base)
	if err != nil {
		return "", fmt.Errorf("git: resolve base: %w", err)
	}
	full := filepath.Clean(filepath.Join(absBase, rel))
	// Windows 文件系统不区分大小写，用 ToLower 做大小写无关前缀比较防止路径穿越绕过
	if runtime.GOOS == "windows" {
		prefix := absBase + string(filepath.Separator)
		if !strings.HasPrefix(strings.ToLower(full), strings.ToLower(prefix)) && !strings.EqualFold(full, absBase) {
			return "", fmt.Errorf("%w: %s", ErrPathEscape, rel)
		}
	} else {
		if !strings.HasPrefix(full, absBase+string(filepath.Separator)) && full != absBase {
			return "", fmt.Errorf("%w: %s", ErrPathEscape, rel)
		}
	}
	return full, nil
}

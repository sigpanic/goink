package git

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/sigpanic/goink/internal/config"
)

// ── 文件路径 ──────────────────────────────────────────────
// 此处都是相对路径，位于小说目录之下的，小说目录由config决定。
// 章节文件以 id 命名（chapters/id_{id}.md）：id_ 前缀将 id 命名空间与
// 历史 num 纯数字命名空间（chapters/001.md）隔离，保证迁移 rename 判断结构性可靠。

// ChapterPath 返回章节正文文件相对路径（按章节 id）。
func ChapterPath(id int64) string {
	return fmt.Sprintf("chapters/id_%d.md", id)
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

// OutlinePath 返回章节大纲文件相对路径（按章节 id）。
func OutlinePath(id int64) string {
	return fmt.Sprintf("outlines/id_%d.md", id)
}

// VolumePath 返回卷纲文件相对路径（按卷 id）。
func VolumePath(volumeID int64) string {
	return fmt.Sprintf("volumes/%d.md", volumeID)
}

// ChapterPathRef 是章节正文或章节大纲虚拟路径的解析结果。
// VolumeID 仅来自两级容错路径，不参与物理文件定位；IsNew 表示 new.md 创建通道。
type ChapterPathRef struct {
	ID        int64
	VolumeID  int64
	IsOutline bool
	IsNew     bool
}

var chapterLikePathRe = regexp.MustCompile(`^(chapters|outlines)/(?:([0-9]+)/)?(?:id_([0-9]+)|new)\.md$`)
var volumePathRe = regexp.MustCompile(`^volumes/([1-9][0-9]*)\.md$`)

// ParseChapterLikePath 解析 rw_tools 支持的章节正文或大纲虚拟路径。
// 支持扁平主格式和带卷 ID 的容错别名；旧的纯数字章节号路径不被接受。
func ParseChapterLikePath(path string) (ChapterPathRef, bool) {
	matches := chapterLikePathRe.FindStringSubmatch(path)
	if matches == nil {
		return ChapterPathRef{}, false
	}

	ref := ChapterPathRef{IsOutline: matches[1] == "outlines"}
	if matches[2] != "" {
		volumeID, err := strconv.ParseInt(matches[2], 10, 64)
		if err != nil {
			return ChapterPathRef{}, false
		}
		ref.VolumeID = volumeID
	}
	if matches[3] == "" {
		ref.IsNew = true
		return ref, true
	}

	id, err := strconv.ParseInt(matches[3], 10, 64)
	if err != nil {
		return ChapterPathRef{}, false
	}
	ref.ID = id
	return ref, true
}

// ParseVolumePath 解析卷纲虚拟路径。卷 ID 必须是规范的正整数表示。
func ParseVolumePath(path string) (int64, bool) {
	matches := volumePathRe.FindStringSubmatch(path)
	if matches == nil {
		return 0, false
	}
	volumeID, err := strconv.ParseInt(matches[1], 10, 64)
	if err != nil {
		return 0, false
	}
	return volumeID, true
}

// ── 文件读写 ──────────────────────────────────────────────
// path 为相对于小说仓库根目录的路径，如 "chapters/id_1.md"、"goink.md"。

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

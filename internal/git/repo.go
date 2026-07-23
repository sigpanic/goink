package git

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/platform"
)

// Repo 管理单部小说的 Git 仓库，提供文件读写和版本控制。
type Repo struct {
	dir    string
	gitBin string
	logger *slog.Logger
}

// CommitInfo 是 git log 的单条记录。
type CommitInfo struct {
	Hash         string    `json:"hash"`
	ShortHash    string    `json:"shortHash"`
	Message      string    `json:"message"`
	Time         time.Time `json:"time"`
	AuthorName   string    `json:"authorName"`
	AuthorEmail  string    `json:"authorEmail"`
	FilesChanged int       `json:"filesChanged"`
	Insertions   int       `json:"insertions"`
	Deletions    int       `json:"deletions"`
}

// FileDiff 表示一次 commit 中单个文件的变更。
type FileDiff struct {
	Path            string `json:"path"`
	ChangeType      string `json:"changeType"`
	OriginalContent string `json:"original"`
	ModifiedContent string `json:"modified"`
}

// New 为指定小说打开已有仓库，不存在则 git init + 首次空 commit。
// gitName 和 gitEmail 用于设置仓库级 git config。空字符串时使用默认值。
// logger 为 nil 时使用 slog.Default()。
func New(novelID int64, gitName, gitEmail string, logger *slog.Logger) (*Repo, error) {
	cfg := config.Get()
	if cfg == nil {
		return nil, fmt.Errorf("git: config not initialized")
	}

	gitBin, err := platform.ResolveGit()
	if err != nil {
		return nil, fmt.Errorf("git: 找不到 git 可执行文件: %w", err)
	}

	if gitName == "" {
		gitName = "Goink"
	}
	if gitEmail == "" {
		gitEmail = "goink@local"
	}

	dir := config.NovelDirPath(novelID)
	if logger == nil {
		logger = slog.Default()
	}
	r := &Repo{dir: dir, gitBin: gitBin, logger: logger}

	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("git: stat .git: %w", err)
		}
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("git: create novel dir: %w", err)
		}
		if _, stderr, err := r.runInDir("init", dir); err != nil {
			return nil, fmt.Errorf("git: init: %s: %w", stderr, err)
		}
		// 仓库级 git config，避免用户未全局配置时 commit 失败
		if _, stderr, err := r.runInDir("config", "user.name", gitName); err != nil {
			return nil, fmt.Errorf("git: config user.name: %s: %w", stderr, err)
		}
		if _, stderr, err := r.runInDir("config", "user.email", gitEmail); err != nil {
			return nil, fmt.Errorf("git: config user.email: %s: %w", stderr, err)
		}
		gitkeep := filepath.Join(dir, "chapters", ".gitkeep")
		if err := os.MkdirAll(filepath.Dir(gitkeep), 0755); err != nil {
			return nil, fmt.Errorf("git: create chapters dir: %w", err)
		}
		if err := os.WriteFile(gitkeep, nil, 0644); err != nil {
			return nil, fmt.Errorf("git: write .gitkeep: %w", err)
		}
		if _, _, err := r.runInDir("add", "chapters/.gitkeep"); err != nil {
			return nil, fmt.Errorf("git: stage .gitkeep: %w", err)
		}
		if _, _, err := r.runInDir("commit", "-m", "initial commit"); err != nil {
			return nil, fmt.Errorf("git: initial commit: %w", err)
		}
	}

	return r, nil
}

// SetGitConfig 在已有仓库上更新仓库级 user.name / user.email 配置。
func (r *Repo) SetGitConfig(gitName, gitEmail string) error {
	if _, stderr, err := r.runInDir("config", "user.name", gitName); err != nil {
		return fmt.Errorf("git: config user.name: %s: %w", stderr, err)
	}
	if _, stderr, err := r.runInDir("config", "user.email", gitEmail); err != nil {
		return fmt.Errorf("git: config user.email: %s: %w", stderr, err)
	}
	return nil
}

// ── Diff ──────────────────────────────────────────────────

// DiffContent 对比当前工作区文件与 proposed 内容，返回 unified diff。
// 文件不存在时以空内容为基准。用临时文件 + git diff --no-index 实现。
func (r *Repo) DiffContent(relPath, proposed string) (string, error) {
	fromPath := relPath
	fullPath := filepath.Join(r.dir, relPath)

	if _, err := os.Stat(fullPath); errors.Is(err, os.ErrNotExist) {
		empty, err := os.CreateTemp("", "git-diff-empty-*")
		if err != nil {
			return "", fmt.Errorf("git: diff: create empty temp: %w", err)
		}
		empty.Close()
		defer os.Remove(empty.Name())
		fromPath = empty.Name()
	}

	tmp, err := os.CreateTemp("", "git-diff-*"+filepath.Ext(relPath))
	if err != nil {
		return "", fmt.Errorf("git: diff: create temp: %w", err)
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.WriteString(proposed); err != nil {
		tmp.Close()
		return "", fmt.Errorf("git: diff: write temp: %w", err)
	}
	tmp.Close()

	stdout, stderr, err := r.runInDir("diff", "--no-index", "--", fromPath, tmp.Name())
	// git diff 有差异时 exit 1，stdout 有内容；exit >1 才是真正的错误
	if err != nil && stdout == "" {
		return "", fmt.Errorf("git: diff: %s: %w", stderr, err)
	}
	stdout = strings.ReplaceAll(stdout, filepath.ToSlash(tmp.Name()), "/"+relPath)
	if fromPath != relPath {
		stdout = strings.ReplaceAll(stdout, filepath.ToSlash(fromPath), "/dev/null")
	}
	return stdout, nil
}

// ── Git 操作 ──────────────────────────────────────────────

func (r *Repo) StageAll() error {
	_, stderr, err := r.runInDir("add", "-A")
	if err != nil {
		return fmt.Errorf("git: stage all: %s: %w", stderr, err)
	}
	return nil
}

func (r *Repo) Commit(msg string) (string, error) {
	_, stderr, err := r.runInDir("commit", "-m", msg)
	if err != nil {
		return "", fmt.Errorf("git: commit: %s: %w", stderr, err)
	}
	hash, _, err := r.runInDir("rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("git: rev-parse after commit: %s: %w", stderr, err)
	}
	return strings.TrimSpace(hash), nil
}

func (r *Repo) HasUncommitted() (bool, error) {
	out, _, err := r.runInDir("status", "--porcelain")
	if err != nil {
		return false, fmt.Errorf("git: status: %w", err)
	}
	return strings.TrimSpace(out) != "", nil
}

// RevertNoCommit 逆序 revert 所有 hash，仅暂存不提交。冲突时自动 abort。
// 调用方负责最终 commit（或 abort）。
func (r *Repo) RevertNoCommit(hashes []string) error {
	for i := len(hashes) - 1; i >= 0; i-- {
		_, stderr, err := r.runInDir("revert", "--no-commit", hashes[i])
		if err != nil {
			if _, _, abortErr := r.runInDir("revert", "--abort"); abortErr != nil {
				return fmt.Errorf("git: revert %s: %s: %w; abort 也失败: %v", hashes[i], stderr, err, abortErr)
			}
			return fmt.Errorf("git: revert %s: %s: %w", hashes[i], stderr, err)
		}
	}
	return nil
}

// RevertAbort 取消进行中的 revert，丢弃所有暂存的 revert 内容。
func (r *Repo) RevertAbort() error {
	_, stderr, err := r.runInDir("revert", "--abort")
	if err != nil {
		return fmt.Errorf("git: revert --abort: %s: %w", stderr, err)
	}
	return nil
}

func (r *Repo) Log(relPath string, n int) ([]CommitInfo, error) {
	args := []string{"log", "--format=%H%x00%s%x00%ct"}
	if n > 0 {
		args = append(args, "-n", strconv.Itoa(n))
	}
	if relPath != "" {
		args = append(args, "--", relPath)
	}

	stdout, stderr, err := r.runInDir(args...)
	if err != nil {
		return nil, fmt.Errorf("git: log: %s: %w", stderr, err)
	}
	return parseLog(stdout), nil
}

func parseLog(out string) []CommitInfo {
	if strings.TrimSpace(out) == "" {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	var commits []CommitInfo
	for _, line := range lines {
		parts := strings.SplitN(line, "\x00", 3)
		if len(parts) < 3 {
			continue
		}
		ts, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			continue
		}
		commits = append(commits, CommitInfo{
			Hash:    parts[0],
			Message: strings.SplitN(parts[1], "\n", 2)[0],
			Time:    time.Unix(ts, 0),
		})
	}
	sort.Slice(commits, func(i, j int) bool {
		return commits[i].Time.Before(commits[j].Time)
	})
	return commits
}

// LogDetailed 返回带详细统计的提交历史，按时间降序（最新在前）。
// afterHash 非空时，返回该 commit 之后的更早提交（基于 git log <afterHash> --skip=1）。
func (r *Repo) LogDetailed(n int, afterHash string) ([]CommitInfo, error) {
	var args []string
	if afterHash != "" {
		args = []string{"log", afterHash, "--skip=1", "-n", strconv.Itoa(n),
			"--format=---%n%H%n%h%n%an%n%ae%n%s%n%ct",
			"--numstat"}
	} else {
		args = []string{"log", "-n", strconv.Itoa(n),
			"--format=---%n%H%n%h%n%an%n%ae%n%s%n%ct",
			"--numstat"}
	}
	stdout, stderr, err := r.runInDir(args...)
	if err != nil {
		if strings.Contains(stderr, "does not have any commits yet") {
			return nil, nil
		}
		return nil, fmt.Errorf("git: log detailed: %s: %w", stderr, err)
	}
	return parseDetailedLog(stdout), nil
}

func parseDetailedLog(out string) []CommitInfo {
	out = strings.TrimSpace(out)
	if out == "" {
		return nil
	}
	blocks := strings.Split(out, "\n---\n")
	var commits []CommitInfo
	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		// 第一个 block 可能以 "---" 开头（因前导的分隔符）
		if strings.HasPrefix(block, "---") {
			block = strings.TrimPrefix(block, "---")
			block = strings.TrimSpace(block)
		}
		lines := strings.Split(block, "\n")
		if len(lines) < 6 {
			continue
		}
		hash := strings.TrimSpace(lines[0])
		shortHash := strings.TrimSpace(lines[1])
		authorName := strings.TrimSpace(lines[2])
		authorEmail := strings.TrimSpace(lines[3])
		subject := strings.TrimSpace(lines[4])
		ts, err := strconv.ParseInt(strings.TrimSpace(lines[5]), 10, 64)
		if err != nil {
			continue
		}

		var filesChanged, insertions, deletions int
		for _, line := range lines[6:] {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, "\t", 3)
			if len(parts) < 3 {
				continue
			}
			add := parts[0]
			del := parts[1]
			if add != "-" {
				if n, err := strconv.Atoi(add); err == nil {
					insertions += n
				}
			}
			if del != "-" {
				if n, err := strconv.Atoi(del); err == nil {
					deletions += n
				}
			}
			filesChanged++
		}

		commits = append(commits, CommitInfo{
			Hash:         hash,
			ShortHash:    shortHash,
			Message:      subject,
			Time:         time.Unix(ts, 0),
			AuthorName:   authorName,
			AuthorEmail:  authorEmail,
			FilesChanged: filesChanged,
			Insertions:   insertions,
			Deletions:    deletions,
		})
	}
	// 降序：最新在前
	sort.Slice(commits, func(i, j int) bool {
		return commits[i].Time.After(commits[j].Time)
	})
	return commits
}

// FileEntry 表示 commit 中一个变更文件的简要信息（不含内容）。
type FileEntry struct {
	Path       string `json:"path"`
	ChangeType string `json:"changeType"` // "added" | "modified" | "deleted" | "renamed"
	OldPath    string `json:"oldPath"`    // 仅 renamed 时有值，表示重命名前的路径
}

// commitFileChanges 获取指定 commit 的变更文件列表（不含内容）。
// 返回是否有父 commit 和文件变更条目。
func (r *Repo) commitFileChanges(hash string) (bool, []FileEntry, error) {
	hasParent := true
	if _, _, err := r.runInDir("rev-parse", hash+"^"); err != nil {
		hasParent = false
	}

	if !hasParent {
		treeOut, _, err := r.runInDir("ls-tree", "-r", "--name-only", hash)
		if err != nil {
			return false, nil, fmt.Errorf("git: ls-tree %s: %w", hash, err)
		}
		var entries []FileEntry
		for _, p := range strings.Split(strings.TrimSpace(treeOut), "\n") {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			entries = append(entries, FileEntry{Path: p, ChangeType: "added"})
		}
		return false, entries, nil
	}

	diffOut, _, err := r.runInDir("diff-tree", "--no-commit-id", "-r", "-M", "--name-status", hash)
	if err != nil {
		return true, nil, fmt.Errorf("git: diff-tree %s: %w", hash, err)
	}
	var entries []FileEntry
	for _, line := range strings.Split(strings.TrimSpace(diffOut), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 2 {
			continue
		}
		status := parts[0]
		changeTypeStr := "modified"
		var oldPath string
		switch {
		case strings.HasPrefix(status, "A"):
			changeTypeStr = "added"
		case strings.HasPrefix(status, "D"):
			changeTypeStr = "deleted"
		case strings.HasPrefix(status, "R"):
			changeTypeStr = "renamed"
			if len(parts) >= 3 {
				oldPath = parts[1]
			}
		}
		// 非 rename: parts[1] 是文件路径；rename: parts[1] 是旧路径, parts[2] 是新路径
		filePath := parts[1]
		if changeTypeStr == "renamed" && len(parts) >= 3 {
			filePath = parts[2]
		}
		entries = append(entries, FileEntry{Path: filePath, ChangeType: changeTypeStr, OldPath: oldPath})
	}
	return true, entries, nil
}

// showFileContent 执行 git show 并处理文件不存在的错误。
func (r *Repo) showFileContent(ref string) (string, error) {
	content, _, err := r.runInDir("show", ref)
	if err != nil {
		if isNotExistInGit(err) {
			r.logger.Debug("git show: file not in commit", "ref", ref)
			return "", nil
		}
		return "", err
	}
	return content, nil
}

// CommitFileList 返回指定 commit 的详细信息和变更文件列表（不含文件内容）。
func (r *Repo) CommitFileList(hash string) (*CommitInfo, []FileEntry, error) {
	logOut, _, err := r.runInDir("log", "-n", "1",
		"--format=---%n%H%n%h%n%an%n%ae%n%s%n%ct", hash)
	if err != nil {
		return nil, nil, fmt.Errorf("git: commit info %s: %w", hash, err)
	}
	parsed := parseDetailedLog(logOut)
	if len(parsed) == 0 {
		return nil, nil, fmt.Errorf("git: commit %s not found", hash)
	}
	commit := parsed[0]

	_, entries, err := r.commitFileChanges(hash)
	if err != nil {
		return nil, nil, err
	}
	return &commit, entries, nil
}

// ShowFile 返回指定 commit 中某个文件的前后内容。
// 对于 added 文件 original 为空，对于 deleted 文件 modified 为空。
func (r *Repo) ShowFile(hash, filePath string) (*FileDiff, error) {
	_, entries, err := r.commitFileChanges(hash)
	if err != nil {
		return nil, err
	}

	var entry *FileEntry
	for i := range entries {
		if entries[i].Path == filePath {
			entry = &entries[i]
			break
		}
	}
	if entry == nil {
		return nil, fmt.Errorf("git: file %s not found in commit %s", filePath, hash)
	}

	var original, modified string
	switch entry.ChangeType {
	case "added":
		modified, err = r.showFileContent(hash + ":" + filePath)
		if err != nil {
			return nil, fmt.Errorf("git show %s:%s: %w", hash, filePath, err)
		}
	case "modified":
		original, err = r.showFileContent(hash + "^:" + filePath)
		if err != nil {
			return nil, fmt.Errorf("git show %s^:%s: %w", hash, filePath, err)
		}
		modified, err = r.showFileContent(hash + ":" + filePath)
		if err != nil {
			return nil, fmt.Errorf("git show %s:%s: %w", hash, filePath, err)
		}
	case "deleted":
		original, err = r.showFileContent(hash + "^:" + filePath)
		if err != nil {
			return nil, fmt.Errorf("git show %s^:%s: %w", hash, filePath, err)
		}
	case "renamed":
		original, err = r.showFileContent(hash + "^:" + entry.OldPath)
		if err != nil {
			return nil, fmt.Errorf("git show %s^:%s: %w", hash, entry.OldPath, err)
		}
		modified, err = r.showFileContent(hash + ":" + filePath)
		if err != nil {
			return nil, fmt.Errorf("git show %s:%s: %w", hash, filePath, err)
		}
	}

	return &FileDiff{
		Path:            filePath,
		ChangeType:      entry.ChangeType,
		OriginalContent: original,
		ModifiedContent: modified,
	}, nil
}

// isNotExistInGit 判断 git show 错误是否为文件不存在（正常情况）。
// git 子进程强制 LC_ALL=C，所有输出均为英文。
func isNotExistInGit(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "does not exist") ||
		strings.Contains(msg, "not found")
}

// ── CLI ───────────────────────────────────────────────────

func (r *Repo) runInDir(args ...string) (stdout, stderr string, err error) {
	return runCmd(r.gitBin, r.dir, args...)
}

func runCmd(gitBin, dir string, args ...string) (stdout, stderr string, err error) {
	cmd := exec.Command(gitBin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	platform.SetPlatformAttr(cmd)
	err = cmd.Run()
	return outBuf.String(), errBuf.String(), err
}

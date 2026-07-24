package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	// 确保系统 git 可用
	if _, err := exec.LookPath("git"); err != nil {
		panic("git not found in PATH, skipping git tests")
	}
	os.Exit(m.Run())
}

// testRepo 创建一个临时 git 仓库用于测试。
// 返回 Repo 实例、根目录、清理函数。
func testRepo(t *testing.T, commits int) (*Repo, string, func()) {
	t.Helper()

	// GIT_* 环境变量（含 GIT_AUTHOR_*）已在 runCmd.filteredGitEnv 统一剥离，
	// 此处无需再手动清理 hook 注入的 env。
	dir, err := os.MkdirTemp("", "git-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}

	r := &Repo{dir: dir, gitBin: "git"}

	// git init
	_, stderr, err := r.runInDir("init")
	if err != nil {
		os.RemoveAll(dir)
		t.Fatalf("git init: %s: %v", stderr, err)
	}

	// 仓库级 config
	r.runInDir("config", "user.name", "Goink")
	r.runInDir("config", "user.email", "goink@local")

	// 创建 commits
	for i := 1; i <= commits; i++ {
		content := []byte("content" + "\n" + strings.Repeat("line ", i*10))
		filename := filepath.Join(dir, "file.txt")
		if err := os.WriteFile(filename, content, 0644); err != nil {
			os.RemoveAll(dir)
			t.Fatalf("write file: %v", err)
		}
		_, stderr, err := r.runInDir("add", ".")
		if err != nil {
			os.RemoveAll(dir)
			t.Fatalf("git add: %s: %v", stderr, err)
		}
		_, stderr, err = r.runInDir("commit", "-m", "commit "+time.Unix(int64(1000+i), 0).Format("2006-01-02"))
		if err != nil {
			os.RemoveAll(dir)
			t.Fatalf("git commit: %s: %v", stderr, err)
		}
	}

	return r, dir, func() { os.RemoveAll(dir) }
}

func TestLogDetailed_Basic(t *testing.T) {
	r, _, cleanup := testRepo(t, 3)
	defer cleanup()

	commits, err := r.LogDetailed(10, "")
	if err != nil {
		t.Fatalf("LogDetailed: %v", err)
	}
	if len(commits) != 3 {
		t.Fatalf("expected 3 commits, got %d", len(commits))
	}

	// 验证降序（最新在前）
	if commits[0].Time.Before(commits[1].Time) {
		t.Error("expected newest commit first, got ascending order")
	}

	// 验证字段
	c := commits[0]
	if c.Hash == "" {
		t.Error("expected non-empty hash")
	}
	if c.ShortHash == "" {
		t.Error("expected non-empty short hash")
	}
	if c.AuthorName != "Goink" {
		t.Errorf("expected author Goink, got %q", c.AuthorName)
	}
	if c.Insertions <= 0 {
		t.Errorf("expected positive insertions, got %d", c.Insertions)
	}
	if c.FilesChanged <= 0 {
		t.Errorf("expected positive files changed, got %d", c.FilesChanged)
	}
}

func TestLogDetailed_CursorPagination(t *testing.T) {
	r, _, cleanup := testRepo(t, 5)
	defer cleanup()

	// 第一页：2 条
	page1, err := r.LogDetailed(2, "")
	if err != nil {
		t.Fatalf("LogDetailed page1: %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("expected 2 commits on page 1, got %d", len(page1))
	}

	// 第二页：以 page1 最后一笔为游标
	cursor := page1[len(page1)-1].Hash
	page2, err := r.LogDetailed(2, cursor)
	if err != nil {
		t.Fatalf("LogDetailed page2: %v", err)
	}
	if len(page2) != 2 {
		t.Fatalf("expected 2 commits on page 2, got %d", len(page2))
	}

	// 验证两页不重叠（游标 commit 不出现）
	if page2[0].Hash == cursor || page2[1].Hash == cursor {
		t.Error("cursor commit should not appear in page 2")
	}

	// 第三页：最后一笔
	cursor2 := page2[len(page2)-1].Hash
	page3, err := r.LogDetailed(2, cursor2)
	if err != nil {
		t.Fatalf("LogDetailed page3: %v", err)
	}
	if len(page3) != 1 {
		t.Fatalf("expected 1 commit on page 3 (the last one), got %d", len(page3))
	}

	// 四页：没有更多了
	cursor3 := page3[0].Hash
	page4, err := r.LogDetailed(2, cursor3)
	if err != nil {
		t.Fatalf("LogDetailed page4: %v", err)
	}
	if len(page4) != 0 {
		t.Fatalf("expected 0 commits on page 4, got %d", len(page4))
	}
}

func TestLogDetailed_N(t *testing.T) {
	r, _, cleanup := testRepo(t, 10)
	defer cleanup()

	// 限制数量
	commits, err := r.LogDetailed(3, "")
	if err != nil {
		t.Fatalf("LogDetailed: %v", err)
	}
	if len(commits) != 3 {
		t.Fatalf("expected 3 commits, got %d", len(commits))
	}
}

func TestLogDetailed_EmptyRepo(t *testing.T) {
	r, _, cleanup := testRepo(t, 0)
	defer cleanup()

	// 空 repo 没有 commit，LogDetailed 应返回空列表而非报错
	commits, err := r.LogDetailed(10, "")
	if err != nil {
		t.Fatalf("LogDetailed on empty repo: %v", err)
	}
	if len(commits) != 0 {
		t.Fatalf("expected 0 commits on empty repo, got %d", len(commits))
	}
}

func TestLogDetailed_WithFiles(t *testing.T) {
	r, dir, cleanup := testRepo(t, 2)
	defer cleanup()

	// 为第二个 commit 写入多个文件，验证统计
	os.WriteFile(filepath.Join(dir, "ch01.md"), []byte("chapter 1\nhello\nworld"), 0644)
	os.WriteFile(filepath.Join(dir, "ch02.md"), []byte("chapter 2\nhello\nworld\nfoo\nbar"), 0644)
	r.runInDir("add", ".")
	r.runInDir("commit", "-m", "add two chapters")

	commits, err := r.LogDetailed(1, "")
	if err != nil {
		t.Fatalf("LogDetailed: %v", err)
	}
	if len(commits) == 0 {
		t.Fatal("expected at least 1 commit")
	}

	c := commits[0]
	if c.FilesChanged < 2 {
		t.Errorf("expected at least 2 files changed, got %d", c.FilesChanged)
	}
	if c.Insertions <= 0 {
		t.Errorf("expected positive insertions, got %d", c.Insertions)
	}
	if c.Deletions != 0 {
		t.Errorf("expected 0 deletions for new files, got %d", c.Deletions)
	}
}

func TestLogDetailed_AfterHashNotFound(t *testing.T) {
	r, _, cleanup := testRepo(t, 3)
	defer cleanup()

	_, err := r.LogDetailed(5, "0000000000000000000000000000000000000000")
	if err == nil {
		t.Error("expected error for nonexistent afterHash, got nil")
	}
}

func TestLogDetailed_CursorAtLastCommit(t *testing.T) {
	r, _, cleanup := testRepo(t, 3)
	defer cleanup()

	all, _ := r.LogDetailed(10, "")
	last := all[len(all)-1]

	next, err := r.LogDetailed(10, last.Hash)
	if err != nil {
		t.Fatalf("LogDetailed after last commit: %v", err)
	}
	if len(next) != 0 {
		t.Fatalf("expected 0 commits after the last one, got %d", len(next))
	}
}

func TestCommitFileList_ShowFile(t *testing.T) {
	r, dir, cleanup := testRepo(t, 1)
	defer cleanup()

	// 添加一个章节文件并 commit
	os.MkdirAll(filepath.Join(dir, "chapters"), 0755)
	os.WriteFile(filepath.Join(dir, "chapters", "001.md"), []byte("# Chapter 1\n\nOnce upon a time..."), 0644)
	r.runInDir("add", ".")
	r.runInDir("commit", "-m", "add chapter 1")

	// 再修改文件并 commit
	os.WriteFile(filepath.Join(dir, "chapters", "001.md"), []byte("# Chapter 1\n\nOnce upon a time...\n\nThe End."), 0644)
	r.runInDir("add", ".")
	r.runInDir("commit", "-m", "update chapter 1")
	stdout, _, _ := r.runInDir("rev-parse", "HEAD")
	hash := strings.TrimSpace(stdout)

	// 先用 CommitFileList 获取文件列表
	commit, entries, err := r.CommitFileList(hash)
	if err != nil {
		t.Fatalf("CommitFileList: %v", err)
	}
	if commit == nil {
		t.Fatal("expected non-nil commit")
	}
	if len(entries) == 0 {
		t.Fatal("expected at least 1 file entry")
	}

	e := entries[0]
	if e.Path != "chapters/001.md" {
		t.Errorf("expected chapters/001.md, got %q", e.Path)
	}
	if e.ChangeType != "modified" {
		t.Errorf("expected modified, got %q", e.ChangeType)
	}

	// 再用 ShowFile 获取具体 diff
	f, err := r.ShowFile(hash, "chapters/001.md")
	if err != nil {
		t.Fatalf("ShowFile: %v", err)
	}
	if f.OriginalContent == "" {
		t.Error("expected non-empty original content")
	}
	if f.ModifiedContent == "" {
		t.Error("expected non-empty modified content")
	}
	if !strings.Contains(f.ModifiedContent, "The End") {
		t.Errorf("modified content should contain 'The End', got %q", f.ModifiedContent)
	}
}

func TestCommitFileList_RootCommit(t *testing.T) {
	r, dir, cleanup := testRepo(t, 0)
	defer cleanup()

	// 初始化后的第一个 commit
	os.MkdirAll(filepath.Join(dir, "chapters"), 0755)
	os.WriteFile(filepath.Join(dir, "chapters", "001.md"), []byte("init content"), 0644)
	r.runInDir("add", ".")
	r.runInDir("commit", "-m", "first commit")
	stdout, _, _ := r.runInDir("rev-parse", "HEAD")
	hash := strings.TrimSpace(stdout)

	commit, entries, err := r.CommitFileList(hash)
	if err != nil {
		t.Fatalf("CommitFileList on root commit: %v", err)
	}
	if commit == nil {
		t.Fatal("expected non-nil commit on root commit")
	}
	if len(entries) == 0 {
		t.Fatal("expected at least 1 file entry")
	}

	e := entries[0]
	if e.ChangeType != "added" {
		t.Errorf("expected added for root commit, got %q", e.ChangeType)
	}

	// ShowFile 验证 added 文件 original 为空
	f, err := r.ShowFile(hash, e.Path)
	if err != nil {
		t.Fatalf("ShowFile on root commit: %v", err)
	}
	if f.OriginalContent != "" {
		t.Error("expected original empty for root commit")
	}
}

func TestCommitFileList_DeletedFile(t *testing.T) {
	r, dir, cleanup := testRepo(t, 1)
	defer cleanup()

	// 创建文件后删除，验证 Deleted 类型
	os.WriteFile(filepath.Join(dir, "draft.md"), []byte("draft content"), 0644)
	r.runInDir("add", ".")
	r.runInDir("commit", "-m", "add draft")

	os.Remove(filepath.Join(dir, "draft.md"))
	r.runInDir("add", ".")
	r.runInDir("commit", "-m", "delete draft")
	stdout, _, _ := r.runInDir("rev-parse", "HEAD")
	hash := strings.TrimSpace(stdout)

	// CommitFileList 获取文件列表
	_, entries, err := r.CommitFileList(hash)
	if err != nil {
		t.Fatalf("CommitFileList: %v", err)
	}

	found := false
	for _, e := range entries {
		if e.Path == "draft.md" {
			found = true
			if e.ChangeType != "deleted" {
				t.Errorf("expected deleted change type, got %q", e.ChangeType)
			}
			break
		}
	}
	if !found {
		t.Error("deleted file not found in CommitFileList result")
	}

	// ShowFile 验证 deleted 文件的 original 有内容、modified 为空
	f, err := r.ShowFile(hash, "draft.md")
	if err != nil {
		t.Fatalf("ShowFile: %v", err)
	}
	if f.OriginalContent == "" {
		t.Error("expected original content for deleted file")
	}
	if f.ModifiedContent != "" {
		t.Error("expected empty modified content for deleted file")
	}
}

func TestCommitFileList_CommitInfo(t *testing.T) {
	r, dir, cleanup := testRepo(t, 1)
	defer cleanup()

	msg := "test commit message"
	os.WriteFile(filepath.Join(dir, "note.txt"), []byte("hello"), 0644)
	r.runInDir("add", ".")
	r.runInDir("commit", "-m", msg)
	stdout, _, _ := r.runInDir("rev-parse", "HEAD")
	hash := strings.TrimSpace(stdout)

	commit, _, err := r.CommitFileList(hash)
	if err != nil {
		t.Fatalf("CommitFileList: %v", err)
	}
	if commit == nil {
		t.Fatal("expected non-nil commit")
	}
	if commit.Hash != hash {
		t.Errorf("hash mismatch: expected %q, got %q", hash, commit.Hash)
	}
	if commit.Message != msg {
		t.Errorf("message mismatch: expected %q, got %q", msg, commit.Message)
	}
	if commit.ShortHash == "" {
		t.Error("expected non-empty shortHash")
	}
	if commit.AuthorName == "" {
		t.Error("expected non-empty authorName")
	}
	if commit.Time.IsZero() {
		t.Error("expected non-zero time")
	}
}

func TestShowFile_NotFound(t *testing.T) {
	r, dir, cleanup := testRepo(t, 1)
	defer cleanup()

	os.WriteFile(filepath.Join(dir, "note.txt"), []byte("hello"), 0644)
	r.runInDir("add", ".")
	r.runInDir("commit", "-m", "add note")
	stdout, _, _ := r.runInDir("rev-parse", "HEAD")
	hash := strings.TrimSpace(stdout)

	_, err := r.ShowFile(hash, "nonexistent.txt")
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
}

func TestCommitFileList_MultipleFiles(t *testing.T) {
	r, dir, cleanup := testRepo(t, 1)
	defer cleanup()

	// 一次 commit 添加多个文件
	os.MkdirAll(filepath.Join(dir, "chapters"), 0755)
	os.WriteFile(filepath.Join(dir, "chapters", "001.md"), []byte("chapter 1"), 0644)
	os.WriteFile(filepath.Join(dir, "chapters", "002.md"), []byte("chapter 2"), 0644)
	os.WriteFile(filepath.Join(dir, "note.txt"), []byte("a note"), 0644)
	r.runInDir("add", ".")
	r.runInDir("commit", "-m", "add multiple files")
	stdout, _, _ := r.runInDir("rev-parse", "HEAD")
	hash := strings.TrimSpace(stdout)

	_, entries, err := r.CommitFileList(hash)
	if err != nil {
		t.Fatalf("CommitFileList: %v", err)
	}
	if len(entries) < 3 {
		t.Fatalf("expected at least 3 file entries, got %d", len(entries))
	}

	// 验证所有条目都是 added
	for _, e := range entries {
		if e.ChangeType != "added" {
			t.Errorf("expected added for all files in new commit, got %q for %q", e.ChangeType, e.Path)
		}
	}

	// 验证具体文件路径存在
	paths := make(map[string]bool)
	for _, e := range entries {
		paths[e.Path] = true
	}
	for _, p := range []string{"chapters/001.md", "chapters/002.md", "note.txt"} {
		if !paths[p] {
			t.Errorf("expected file %q in entries", p)
		}
	}
}

func TestCommitFileList_RenamedFile(t *testing.T) {
	r, dir, cleanup := testRepo(t, 1)
	defer cleanup()

	// 创建文件并 commit
	os.WriteFile(filepath.Join(dir, "old.txt"), []byte("some content"), 0644)
	r.runInDir("add", ".")
	r.runInDir("commit", "-m", "add old.txt")

	// 重命名文件（git mv），加 -M 时 diff-tree 会显示 R 而不是 D+A
	r.runInDir("mv", "old.txt", "new.txt")
	r.runInDir("commit", "-m", "rename old.txt to new.txt")
	stdout, _, _ := r.runInDir("rev-parse", "HEAD")
	hash := strings.TrimSpace(stdout)

	_, entries, err := r.CommitFileList(hash)
	if err != nil {
		t.Fatalf("CommitFileList: %v", err)
	}

	// 加 -M 时应显示为 renamed 条目
	found := false
	for _, e := range entries {
		if e.Path == "new.txt" && e.ChangeType == "renamed" {
			found = true
			if e.OldPath != "old.txt" {
				t.Errorf("expected oldPath=old.txt, got %q", e.OldPath)
			}
			break
		}
	}
	if !found {
		// 有些 git 版本 -M 仍可能拆为 D+A，这是可接受的降级
		t.Log("renamed entry not found (git may not detect rename); checking D+A fallback")
		pathMap := make(map[string]string)
		for _, e := range entries {
			pathMap[e.Path] = e.ChangeType
		}
		if pathMap["new.txt"] != "added" || pathMap["old.txt"] != "deleted" {
			t.Errorf("expected new.txt=added and old.txt=deleted as fallback, got %v", pathMap)
		}
	}

	// ShowFile 对 renamed 文件应返回 original=旧内容, modified=新内容
	f, err := r.ShowFile(hash, "new.txt")
	if err != nil {
		t.Fatalf("ShowFile: %v", err)
	}
	if f.ChangeType == "renamed" {
		if f.OriginalContent == "" {
			t.Error("expected non-empty original content for renamed file")
		}
		if f.ModifiedContent == "" {
			t.Error("expected non-empty modified content for renamed file")
		}
	}
}

func TestParseDetailedLog(t *testing.T) {
	input := `---
abc123def456abc123def456abc123def456abc123
abc123d
Author Name
author@email.com
feat: add user login
1234567890
5	3	chapters/001.md
2	0	chapters/002.md

---
def789abc123def789abc123def789abc123def789
def789a
Another Author
another@test.com
fix typo
1234567000
1	1	chapters/001.md
`

	commits := parseDetailedLog(input)
	if len(commits) != 2 {
		t.Fatalf("expected 2 commits, got %d", len(commits))
	}

	// 验证第一条
	c0 := commits[0]
	if c0.Hash != "abc123def456abc123def456abc123def456abc123" {
		t.Errorf("hash mismatch: %q", c0.Hash)
	}
	if c0.ShortHash != "abc123d" {
		t.Errorf("shortHash mismatch: %q", c0.ShortHash)
	}
	if c0.AuthorName != "Author Name" {
		t.Errorf("author mismatch: %q", c0.AuthorName)
	}
	if c0.Message != "feat: add user login" {
		t.Errorf("message mismatch: %q", c0.Message)
	}
	if c0.FilesChanged != 2 {
		t.Errorf("expected 2 files, got %d", c0.FilesChanged)
	}
	if c0.Insertions != 7 {
		t.Errorf("expected 7 insertions, got %d", c0.Insertions)
	}
	if c0.Deletions != 3 {
		t.Errorf("expected 3 deletions, got %d", c0.Deletions)
	}
}

func TestParseDetailedLog_Empty(t *testing.T) {
	commits := parseDetailedLog("")
	if commits != nil {
		t.Errorf("expected nil for empty input, got %v", commits)
	}
}

func TestParseDetailedLog_BinaryFile(t *testing.T) {
	// --numstat 对二进制文件输出 "-" 而不是数字
	input := `---
hash12345678901234567890123456789012345678
hash123
Author
a@b.com
add image
1234567890
-	-	cover.jpg
0	0	.gitkeep
`

	commits := parseDetailedLog(input)
	if len(commits) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(commits))
	}

	c := commits[0]
	if c.FilesChanged != 2 {
		t.Errorf("expected 2 files, got %d", c.FilesChanged)
	}
	// 二进制文件的 "-" 应转为 0
	if c.Insertions != 0 {
		t.Errorf("expected 0 insertions (binary file), got %d", c.Insertions)
	}
	if c.Deletions != 0 {
		t.Errorf("expected 0 deletions (binary file), got %d", c.Deletions)
	}
}

//go:build cgo && e2e

package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sigpanic/goink/internal/git"
)

// createE2ERepo creates a novel directory and initializes a git repo using bundled git.
// Uses a unique novel ID derived from the test name.
func createE2ERepo(t *testing.T) (*git.Repo, int64, func()) {
	t.Helper()

	// Use a deterministic but unique novel ID for each test
	novelID := int64(9000 + hashTestName(t.Name()))

	// Ensure clean state
	dir := novelDir(novelID)
	os.RemoveAll(dir)

	repo, err := git.New(novelID, "E2E Test", "e2e@test", nil)
	if err != nil {
		t.Fatalf("git.New() failed: %v", err)
	}

	return repo, novelID, func() {}
}

// hashTestName returns a small hash from a test name for use as novel ID offset.
func hashTestName(name string) int {
	h := 0
	for _, c := range name {
		h = h*31 + int(c)
	}
	if h < 0 {
		h = -h
	}
	return h % 999
}

// novelDir returns the directory path for a given novel ID in the test data dir.
func novelDir(novelID int64) string {
	return filepath.Join(os.Getenv("GOINK_DATA_DIR"), "novels", fmt.Sprintf("%d", novelID))
}

func TestGitNew_InitWithBundledGit(t *testing.T) {
	repo, _, cleanup := createE2ERepo(t)
	defer cleanup()

	// Verify repo is functional by checking HasUncommitted
	hasUncommitted, err := repo.HasUncommitted()
	if err != nil {
		t.Fatalf("HasUncommitted() failed: %v", err)
	}
	t.Logf("HasUncommitted after init: %v", hasUncommitted)
}

func TestGitCommit_WithBundledGit(t *testing.T) {
	repo, novelID, cleanup := createE2ERepo(t)
	defer cleanup()

	// Write a file
	dir := novelDir(novelID)
	chapterDir := filepath.Join(dir, "chapters")
	os.MkdirAll(chapterDir, 0755)
	os.WriteFile(filepath.Join(chapterDir, "001.md"), []byte("# 第一章\n\n这是测试内容。"), 0644)

	// Stage and commit
	if err := repo.StageAll(); err != nil {
		t.Fatalf("StageAll() failed: %v", err)
	}
	hash, err := repo.Commit("添加第一章")
	if err != nil {
		t.Fatalf("Commit() failed: %v", err)
	}
	if hash == "" {
		t.Error("Commit() returned empty hash")
	}
	t.Logf("Commit hash: %s", hash)
}

func TestGitLog_WithBundledGit(t *testing.T) {
	repo, novelID, cleanup := createE2ERepo(t)
	defer cleanup()

	dir := novelDir(novelID)

	// Create multiple commits
	for i := 1; i <= 3; i++ {
		content := fmt.Sprintf("# 第%d章\n\n内容%d", i, i)
		os.MkdirAll(filepath.Join(dir, "chapters"), 0755)
		os.WriteFile(filepath.Join(dir, "chapters", fmt.Sprintf("%03d.md", i)), []byte(content), 0644)
		repo.StageAll()
		repo.Commit(fmt.Sprintf("添加第%d章", i))
	}

	// Test simple Log
	commits, err := repo.Log("", 0)
	if err != nil {
		t.Fatalf("Log() failed: %v", err)
	}
	if len(commits) < 4 { // 3 chapter commits + 1 initial commit
		t.Errorf("expected at least 4 commits, got %d", len(commits))
	}

	// Test LogDetailed
	detailed, err := repo.LogDetailed(10, "")
	if err != nil {
		t.Fatalf("LogDetailed() failed: %v", err)
	}
	if len(detailed) < 4 {
		t.Errorf("expected at least 4 detailed commits, got %d", len(detailed))
	}

	// Verify ordering (newest first)
	if len(detailed) >= 2 {
		if detailed[0].Time.Before(detailed[1].Time) {
			t.Error("expected newest commit first, got ascending order")
		}
	}

	// Verify fields are populated
	for _, c := range detailed {
		if c.Hash == "" {
			t.Error("expected non-empty hash")
		}
		if c.Message == "" {
			t.Error("expected non-empty message")
		}
	}
}

func TestGitCommitFileList_ShowFile_WithBundledGit(t *testing.T) {
	repo, novelID, cleanup := createE2ERepo(t)
	defer cleanup()

	dir := novelDir(novelID)

	// Create and commit a chapter file
	os.MkdirAll(filepath.Join(dir, "chapters"), 0755)
	os.WriteFile(filepath.Join(dir, "chapters", "001.md"), []byte("# 第一章\n\n这是原始内容。"), 0644)
	repo.StageAll()
	hash1, err := repo.Commit("添加第一章")
	if err != nil {
		t.Fatalf("first Commit() failed: %v", err)
	}

	// Modify and commit again
	os.WriteFile(filepath.Join(dir, "chapters", "001.md"), []byte("# 第一章\n\n这是修改后的内容。"), 0644)
	repo.StageAll()
	hash2, err := repo.Commit("修改第一章")
	if err != nil {
		t.Fatalf("second Commit() failed: %v", err)
	}

	// Test CommitFileList on the second commit
	commit, entries, err := repo.CommitFileList(hash2)
	if err != nil {
		t.Fatalf("CommitFileList() failed: %v", err)
	}
	if commit == nil {
		t.Fatal("expected non-nil commit info")
	}
	if len(entries) == 0 {
		t.Fatal("expected at least 1 file entry")
	}

	found := false
	for _, e := range entries {
		if e.Path == "chapters/001.md" {
			found = true
			if e.ChangeType != "modified" {
				t.Errorf("expected modified, got %q", e.ChangeType)
			}
		}
	}
	if !found {
		t.Error("chapters/001.md not found in file entries")
	}

	// Test ShowFile
	diff, err := repo.ShowFile(hash2, "chapters/001.md")
	if err != nil {
		t.Fatalf("ShowFile() failed: %v", err)
	}
	if diff.OriginalContent == "" {
		t.Error("expected non-empty original content for modified file")
	}
	if diff.ModifiedContent == "" {
		t.Error("expected non-empty modified content")
	}
	if !strings.Contains(diff.ModifiedContent, "修改后的内容") {
		t.Errorf("modified content should contain '修改后的内容', got %q", diff.ModifiedContent)
	}

	// Test ShowFile on added file (first commit)
	diff1, err := repo.ShowFile(hash1, "chapters/001.md")
	if err != nil {
		t.Fatalf("ShowFile() for first commit failed: %v", err)
	}
	if diff1.ChangeType != "added" {
		t.Errorf("expected added for first commit, got %q", diff1.ChangeType)
	}
}

func TestGitRevert_WithBundledGit(t *testing.T) {
	repo, novelID, cleanup := createE2ERepo(t)
	defer cleanup()

	dir := novelDir(novelID)

	// Create and commit a file
	os.MkdirAll(filepath.Join(dir, "chapters"), 0755)
	os.WriteFile(filepath.Join(dir, "chapters", "001.md"), []byte("# 第一章\n\n内容"), 0644)
	repo.StageAll()
	hash, err := repo.Commit("添加第一章")
	if err != nil {
		t.Fatalf("Commit() failed: %v", err)
	}

	// Revert the commit (no-commit mode)
	if err := repo.RevertNoCommit([]string{hash}); err != nil {
		t.Fatalf("RevertNoCommit() failed: %v", err)
	}

	// Abort the revert
	if err := repo.RevertAbort(); err != nil {
		t.Fatalf("RevertAbort() failed: %v", err)
	}
}

func TestGitDiffContent_WithBundledGit(t *testing.T) {
	repo, _, cleanup := createE2ERepo(t)
	defer cleanup()

	// DiffContent compares working tree file with proposed content
	diff, err := repo.DiffContent("chapters/new.md", "# 新章节\n\n这是新内容。")
	if err != nil {
		t.Fatalf("DiffContent() failed: %v", err)
	}
	// File doesn't exist yet, so diff should show all content as added
	if diff == "" {
		t.Error("expected non-empty diff for new file")
	}
	t.Logf("Diff output:\n%s", diff)
}

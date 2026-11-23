package utils

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeTestFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), mode))
	require.NoError(t, os.Chmod(path, mode))
}

func TestMoveFile_SameDevice(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.md")
	dst := filepath.Join(dir, "dst.md")
	writeTestFile(t, src, "hello", 0o640)

	require.NoError(t, MoveFile(src, dst))

	data, err := os.ReadFile(dst)
	require.NoError(t, err)
	require.Equal(t, "hello", string(data))

	_, err = os.Lstat(src)
	require.True(t, errors.Is(err, fs.ErrNotExist))

	if runtime.GOOS != "windows" {
		info, err := os.Stat(dst)
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o640), info.Mode().Perm())
	}
}

func TestMoveFile_TargetExists(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.md")
	dst := filepath.Join(dir, "dst.md")
	writeTestFile(t, src, "source", 0o644)
	writeTestFile(t, dst, "target", 0o644)

	err := MoveFile(src, dst)
	require.True(t, errors.Is(err, fs.ErrExist))

	srcData, err := os.ReadFile(src)
	require.NoError(t, err)
	require.Equal(t, "source", string(srcData))

	dstData, err := os.ReadFile(dst)
	require.NoError(t, err)
	require.Equal(t, "target", string(dstData))
}

func TestMoveFile_SourceMissing(t *testing.T) {
	dir := t.TempDir()
	err := MoveFile(filepath.Join(dir, "missing.md"), filepath.Join(dir, "dst.md"))
	require.True(t, errors.Is(err, fs.ErrNotExist))
}

func TestMoveFile_SourceIsDirectory(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	require.NoError(t, os.Mkdir(src, 0o755))

	err := MoveFile(src, dst)
	require.ErrorContains(t, err, "not a regular file")
	require.DirExists(t, src)
}

func TestMoveFile_TargetIsDirectory(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.md")
	dst := filepath.Join(dir, "dst")
	writeTestFile(t, src, "source", 0o644)
	require.NoError(t, os.Mkdir(dst, 0o755))

	err := MoveFile(src, dst)
	require.True(t, errors.Is(err, fs.ErrExist))
}

func TestMoveByCopy(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.md")
	dst := filepath.Join(dir, "dst.md")
	writeTestFile(t, src, "hello", 0o640)

	info, err := os.Lstat(src)
	require.NoError(t, err)
	require.NoError(t, moveByCopy(src, dst, info))

	data, err := os.ReadFile(dst)
	require.NoError(t, err)
	require.Equal(t, "hello", string(data))

	_, err = os.Lstat(src)
	require.True(t, errors.Is(err, fs.ErrNotExist))

	if runtime.GOOS != "windows" {
		dstInfo, err := os.Stat(dst)
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o640), dstInfo.Mode().Perm())
	}
}

func TestMoveByCopy_TargetExists(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.md")
	dst := filepath.Join(dir, "dst.md")
	writeTestFile(t, src, "source", 0o644)
	writeTestFile(t, dst, "target", 0o644)

	info, err := os.Lstat(src)
	require.NoError(t, err)
	err = moveByCopy(src, dst, info)
	require.True(t, errors.Is(err, fs.ErrExist))

	srcData, err := os.ReadFile(src)
	require.NoError(t, err)
	require.Equal(t, "source", string(srcData))

	dstData, err := os.ReadFile(dst)
	require.NoError(t, err)
	require.Equal(t, "target", string(dstData))
}

func TestMoveByCopy_PartialMove(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("需要非 root 的 Unix 环境来制造删源失败")
	}

	srcDir := t.TempDir()
	dstDir := t.TempDir()
	src := filepath.Join(srcDir, "src.md")
	dst := filepath.Join(dstDir, "dst.md")
	writeTestFile(t, src, "hello", 0o644)

	info, err := os.Lstat(src)
	require.NoError(t, err)

	require.NoError(t, os.Chmod(srcDir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(srcDir, 0o755) })

	err = moveByCopy(src, dst, info)
	require.True(t, errors.Is(err, ErrPartialMove))

	require.FileExists(t, dst)
	require.FileExists(t, src)
}

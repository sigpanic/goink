package utils

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"time"
)

// ErrPartialMove 表示目标文件已成功写入，但源文件删除失败。
// 调用方可用 errors.Is 判断这种半完成状态。
var ErrPartialMove = errors.New("utils: move: target written but source removal failed")

// MoveFile 将普通文件从 src 移动到 dst，不覆盖已存在的目标。
//
// 语义：
//   - dst 已存在（包括目录、符号链接）时返回包装了 fs.ErrExist 的错误；
//   - src 不存在时返回包装了 fs.ErrNotExist 的错误；
//   - 只支持普通文件，目录和符号链接会返回错误；
//   - 调用方需保证 dst 的父目录已存在；
//   - 同一文件系统内使用 os.Rename；跨设备时退化为复制 + 删除源文件，
//     复制路径使用 O_EXCL 创建目标，因此同样不会覆盖。
//
// os.Rename 在目标存在时会直接覆盖，且标准库没有可移植的 no-replace 语义，
// 所以这里先检查目标是否存在。该检查在单用户桌面场景下接受极小的竞态。
func MoveFile(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return fmt.Errorf("utils: move %q to %q: %w", src, dst, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("utils: move %q to %q: source is not a regular file", src, dst)
	}

	if _, err := os.Lstat(dst); err == nil {
		return fmt.Errorf("utils: move %q to %q: %w", src, dst, fs.ErrExist)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("utils: move %q to %q: %w", src, dst, err)
	}

	if err := os.Rename(src, dst); err == nil {
		return nil
	} else if !isCrossDevice(err) {
		return fmt.Errorf("utils: move %q to %q: %w", src, dst, err)
	}

	return moveByCopy(src, dst, info)
}

// moveByCopy 读取 src 并以 O_EXCL 创建 dst，成功后删除 src。
// 仅由 MoveFile 在 os.Rename 跨设备失败时调用，也可单独测试。
func moveByCopy(src, dst string, info fs.FileInfo) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("utils: move %q to %q: %w", src, dst, err)
	}

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		_ = in.Close()
		return fmt.Errorf("utils: move %q to %q: %w", src, dst, err)
	}

	cleanup := func() {
		_ = out.Close()
		_ = os.Remove(dst)
	}

	if _, err := io.Copy(out, in); err != nil {
		_ = in.Close()
		cleanup()
		return fmt.Errorf("utils: move %q to %q: copy: %w", src, dst, err)
	}
	// 在删除源文件前关闭读句柄，避免 Windows 上的共享冲突。
	if err := in.Close(); err != nil {
		cleanup()
		return fmt.Errorf("utils: move %q to %q: close source: %w", src, dst, err)
	}
	if err := out.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("utils: move %q to %q: sync: %w", src, dst, err)
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(dst)
		return fmt.Errorf("utils: move %q to %q: close: %w", src, dst, err)
	}
	if err := os.Chmod(dst, info.Mode().Perm()); err != nil {
		_ = os.Remove(dst)
		return fmt.Errorf("utils: move %q to %q: chmod: %w", src, dst, err)
	}
	if err := os.Chtimes(dst, time.Now(), info.ModTime()); err != nil {
		_ = os.Remove(dst)
		return fmt.Errorf("utils: move %q to %q: chtimes: %w", src, dst, err)
	}
	if err := os.Remove(src); err != nil {
		return fmt.Errorf("%w: %q -> %q: %w", ErrPartialMove, src, dst, err)
	}
	return nil
}

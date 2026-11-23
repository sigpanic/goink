//go:build !windows

package utils

import (
	"errors"
	"syscall"
)

// isCrossDevice 判断 rename 是否因跨文件系统失败。
func isCrossDevice(err error) bool {
	return errors.Is(err, syscall.EXDEV)
}

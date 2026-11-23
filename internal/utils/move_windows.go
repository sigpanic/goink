//go:build windows

package utils

import (
	"errors"
	"syscall"
)

// errorNotSameDevice 是 Windows 的 ERROR_NOT_SAME_DEVICE。
// 标准库 syscall 未导出该常量，值来自 WinError.h。
const errorNotSameDevice syscall.Errno = 17

// isCrossDevice 判断 rename 是否因跨文件系统失败。
func isCrossDevice(err error) bool {
	return errors.Is(err, errorNotSameDevice)
}

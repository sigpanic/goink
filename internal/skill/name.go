package skill

import (
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidName 表示技能名称无法安全地映射为 <name>.md 文件名。
var ErrInvalidName = errors.New("skill: invalid name")

// ValidateName 校验技能名称。
// 名称将作为 <name>.md 的文件名使用，因此拒绝路径分隔符、.md 后缀、
// Windows 非法字符以及空名称/首尾空白。
func ValidateName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("%w: 名称不能为空", ErrInvalidName)
	}
	if strings.TrimSpace(name) != name {
		return fmt.Errorf("%w: 名称首尾不能有空白", ErrInvalidName)
	}
	if name == "." || name == ".." {
		return fmt.Errorf("%w: 名称不能是 %q", ErrInvalidName, name)
	}
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("%w: 名称不能包含路径分隔符", ErrInvalidName)
	}
	if strings.ContainsAny(name, `:*?"<>|`) {
		return fmt.Errorf("%w: 名称不能包含 : * ? \" < > |", ErrInvalidName)
	}
	if strings.HasSuffix(strings.ToLower(name), ".md") {
		return fmt.Errorf("%w: 名称不能以 .md 结尾", ErrInvalidName)
	}
	return nil
}

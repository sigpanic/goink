package skill

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sigpanic/goink/internal/utils"
)

// ErrInvalidContent 表示技能文件无法解析，或 frontmatter.name 与文件名不一致。
var ErrInvalidContent = errors.New("skill: invalid content")

// Move 将 name 对应的技能文件从 srcPath 移动到 dstPath。
// srcPath 必须存在且 frontmatter.name 与 name 一致；dstPath 已存在时返回 fs.ErrExist。
// 目标父目录不存在时会自动创建。
func Move(srcPath, dstPath, name string) (*Skill, error) {
	if err := ValidateName(name); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(srcPath)
	if err != nil {
		return nil, fmt.Errorf("skill: 读取 %s 失败: %w", srcPath, err)
	}
	sk, err := ParseBytes(data, "")
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidContent, srcPath, err)
	}
	if sk.Name != name {
		return nil, fmt.Errorf("%w: frontmatter name %q 与文件名 %q 不一致", ErrInvalidContent, sk.Name, name)
	}

	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return nil, fmt.Errorf("skill: 创建目标目录失败: %w", err)
	}
	if err := utils.MoveFile(srcPath, dstPath); err != nil {
		return nil, fmt.Errorf("skill: 移动 %s -> %s: %w", srcPath, dstPath, err)
	}
	return sk, nil
}

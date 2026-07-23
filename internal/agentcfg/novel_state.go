package agentcfg

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/git"
	"github.com/sigpanic/goink/internal/novel"
)

// NovelState 构建小说上下文快照（原 System3），每轮对话开头注入。
// 只包含基本信息 + 故事状态。具体数据（角色、时间线等）由 MCP 工具按需提供。
func NovelState(db *gorm.DB, novelID int64) (string, error) {
	var n novel.Novel
	if err := db.First(&n, novelID).Error; err != nil {
		return "", fmt.Errorf("agentcfg: load novel %d: %w", novelID, err)
	}

	var b []byte
	b = append(b, "【小说基础信息】\n"...)
	b = append(b, fmt.Sprintf("书名：%s\n", n.Title)...)
	if n.Genre != "" {
		b = append(b, fmt.Sprintf("类型：%s\n", n.Genre)...)
	}
	if n.Description != "" {
		b = append(b, fmt.Sprintf("简介：%s\n", n.Description)...)
	}

	state, err := git.ReadFile(novelID, git.GoinkPath())
	if err == nil && state != "" {
		b = append(b, "\n【故事状态文档】\n"...)
		b = append(b, state...)
	}

	return string(b), nil
}

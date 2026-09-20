//go:build cgo

package mcp_tools

import "testing"

// TestValidPath edit/read 支持的全部路径形态。
func TestValidPath(t *testing.T) {
	valid := []string{
		"chapters/id_1.md",
		"chapters/3/id_1.md",
		"chapters/3/new.md",
		"outlines/id_1.md",
		"outlines/3/new.md",
		"volumes/3.md",
		"goink.md",
		"skills/my-skill.md",
		"~/.goink/skills/my-skill.md",
	}
	for _, p := range valid {
		if !validPath(p) {
			t.Errorf("validPath(%q) = false, want true", p)
		}
	}

	invalid := []string{
		"chapters/001.md",
		"outlines/001.md",
		"chapters/new",        // 缺扩展名
		"chapters/id_1.md/",   // 尾部斜杠
		"plans/weekly.md",     // 不支持的目录
		"volumes/0.md",        // 卷 ID 必须为正整数
		"volumes/01.md",       // 卷 ID 必须用规范整数表示
		"volumes/1.txt",       // 扩展名错误
		"skills/",             // 缺文件名
		"skills/a/b.md",       // 多级技能名
		"/etc/passwd",         // 绝对路径（builtin 前缀之外）
		"~/.goink/other/x.md", // 用户目录下非 skills 路径
	}
	for _, p := range invalid {
		if validPath(p) {
			t.Errorf("validPath(%q) = true, want false", p)
		}
	}
}

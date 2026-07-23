package agentcfg

import (
	"strings"

	"github.com/sigpanic/goink/internal/skill"
)

// BuildSkillCatalog 将 auto 模式的 skill 元数据格式化为 LLM 可用的技能目录。
// manual 和 always 模式的 skill 不出现在目录中。
// 按 novel > user > builtin 分组输出，每组只列出 name 和 description。
func BuildSkillCatalog(meta []skill.SkillMeta) string {
	if len(meta) == 0 {
		return ""
	}

	novel := filterBySource(meta, "novel")
	user := filterBySource(meta, "user")
	builtin := filterBySource(meta, "builtin")

	var b strings.Builder
	b.WriteString("<available_skills>\n")

	if len(novel) > 0 {
		b.WriteString("## 小说专属技能\n")
		for _, s := range novel {
			b.WriteString("- ")
			b.WriteString(s.Name)
			if s.Description != "" {
				b.WriteString(": ")
				b.WriteString(s.Description)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	if len(user) > 0 {
		b.WriteString("## 用户技能\n")
		for _, s := range user {
			b.WriteString("- ")
			b.WriteString(s.Name)
			if s.Description != "" {
				b.WriteString(": ")
				b.WriteString(s.Description)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	if len(builtin) > 0 {
		b.WriteString("## 内置技能（只读）\n")
		for _, s := range builtin {
			b.WriteString("- ")
			b.WriteString(s.Name)
			if s.Description != "" {
				b.WriteString(": ")
				b.WriteString(s.Description)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	b.WriteString("---\n")
	b.WriteString("使用 read 工具加载技能完整内容：\n")
	b.WriteString("- skills/<name>.md（小说技能）\n")
	b.WriteString("- ~/.goink/skills/<name>.md（用户技能）\n")
	b.WriteString("- /builtin/skills/<name>.md（内置技能，只读）\n")
	b.WriteString("使用 edit 工具创建或修改技能。内置技能不可编辑。\n")
	b.WriteString("</available_skills>")

	return b.String()
}

// BuildAlwaysSkillsContent 拼合所有 always 模式的 skill 正文，作为常驻系统消息注入。
// 不包含 YAML frontmatter，只拼接 body 内容。
func BuildAlwaysSkillsContent(meta []skill.SkillMeta, store *skill.Store, novelID int64) string {
	always := filterByMode(meta, skill.ModeAlways)
	if len(always) == 0 || store == nil {
		return ""
	}

	var b strings.Builder
	b.WriteString("【常驻技能】\n")
	b.WriteString("以下技能在本次对话中始终生效：\n\n")

	for _, m := range always {
		sk, ok := store.Get(novelID, m.Name)
		if !ok {
			continue
		}
		b.WriteString("--- ")
		b.WriteString(sk.Name)
		b.WriteString(" ---\n")
		b.WriteString(sk.Content)
		b.WriteString("\n\n")
	}

	return strings.TrimSpace(b.String())
}

func filterBySource(meta []skill.SkillMeta, source string) []skill.SkillMeta {
	var result []skill.SkillMeta
	for _, s := range meta {
		if s.Source == source {
			result = append(result, s)
		}
	}
	return result
}

func filterByMode(meta []skill.SkillMeta, mode string) []skill.SkillMeta {
	var result []skill.SkillMeta
	for _, s := range meta {
		if s.Mode == mode {
			result = append(result, s)
		}
	}
	return result
}

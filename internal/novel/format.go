package novel

import (
	"fmt"
	"strings"
)

// FormatPreferences 格式化偏好列表为可读文本，按全局/小说专属分组，每条带 id 前缀。
// 用于 NovelProfile 注入 system 消息，agent 看到后可通过 id 调 upsert_preference 更新。
// 空列表返回空字符串，调用方决定是否注入。
func FormatPreferences(items []PreferenceItem) string {
	if len(items) == 0 {
		return ""
	}

	var globalB, novelB strings.Builder
	for _, item := range items {
		if item.IsGlobal {
			fmt.Fprintf(&globalB, "- [#%d | %s] %s\n", item.ID, item.Category, item.Content)
		} else {
			fmt.Fprintf(&novelB, "- [#%d | %s] %s\n", item.ID, item.Category, item.Content)
		}
	}

	var b strings.Builder
	b.WriteString("【偏好】\n")
	if globalB.Len() > 0 {
		b.WriteString("\n#### 全局偏好（所有小说生效）\n")
		b.WriteString(globalB.String())
	}
	if novelB.Len() > 0 {
		b.WriteString("\n#### 本小说专属偏好\n")
		b.WriteString(novelB.String())
	}

	return b.String()
}

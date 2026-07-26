package setting

import (
	"fmt"
	"strings"

	"github.com/sigpanic/goink/internal/llm"
)

// SettingsTokenBudget 是注入 system 消息时设定的软上限（token）。
// 设计依据：8k 占 200k 上下文窗口 4%，中文 8k 约等于 4-6k 字，能装较完整的世界观/力量体系。
// 超预算时按 created_at DESC 保留最新，末尾追加截断提示（让 LLM 知道有内容被截断、应合并而非新建）。
const SettingsTokenBudget = 8000

// FormatSettings 格式化设定列表为可读文本，按全局/小说专属分组，每条带 id 前缀。
// 用于 NovelProfile 注入 system 消息，agent 看到后可通过 id 调 upsert_setting 更新。
// 空 items 返回空字符串。
//
// 排序：调用方（NovelProfile）SQL 已按 is_global DESC, created_at DESC 排序——
// 全局组在前、组内最新在前。FormatSettings 信任调用方顺序，不重复排序。
// 超预算时丢弃"最旧"=列表末尾的条目，保留最近创建/更新的。
//
// 超预算处理：逐条累加 token，超 SettingsTokenBudget 时停止拼接并丢弃剩余条目，
// 末尾追加截断提示。返回 (text, truncatedCount)。
func FormatSettings(items []SettingItem) (string, int) {
	if len(items) == 0 {
		return "", 0
	}

	// 调用方 SQL 已排序，这里只按 IsGlobal 分组（保持原相对顺序）。
	global, novelSets := splitByGlobal(items)

	var b strings.Builder
	b.WriteString("【设定】\n")
	used, _ := llm.CountTokens(b.String())

	totalWritten := 0
	budgetExceeded := false

	for _, group := range []struct {
		name  string
		items []SettingItem
	}{
		{"全局设定（所有小说生效）", global},
		{"本小说专属设定", novelSets},
	} {
		if len(group.items) == 0 || budgetExceeded {
			continue
		}
		header := fmt.Sprintf("\n#### %s\n", group.name)
		headerTokens, _ := llm.CountTokens(header)
		if used+headerTokens > SettingsTokenBudget {
			budgetExceeded = true
			continue
		}
		b.WriteString(header)
		used += headerTokens
		written := 0
		for _, item := range group.items {
			line := fmt.Sprintf("- [setting_id:%d | %s] %s\n", item.ID, item.Category, item.Content)
			lineTokens, _ := llm.CountTokens(line)
			if used+lineTokens > SettingsTokenBudget {
				budgetExceeded = true
				break
			}
			b.WriteString(line)
			used += lineTokens
			written++
		}
		totalWritten += written
	}

	if truncated := len(items) - totalWritten; truncated > 0 {
		fmt.Fprintf(&b, "\n⚠️ 设定总量超过 %d token 软上限，已保留最新 %d 条，被截断 %d 条。请优先用 upsert_setting 合并已有条目（基于 id 更新 content，把多条设定合并为一条）而非新建，避免预算持续膨胀。\n",
			SettingsTokenBudget, totalWritten, truncated)
		return b.String(), truncated
	}

	return b.String(), 0
}

// CountSettingsTokens 估算设定列表全量 token 数（不截断），用于 App 层报告超预算状态。
// 与 FormatSettings 输出格式一致，但不做截断，返回完整文本的 token 数。
// 用于 GetNovelSetting 返回 token_count + over_budget，前端可据此显示但不阻塞。
func CountSettingsTokens(items []SettingItem) (int, error) {
	if len(items) == 0 {
		return 0, nil
	}
	var b strings.Builder
	b.WriteString("【设定】\n")
	global, novelSets := splitByGlobal(items)
	if len(global) > 0 {
		b.WriteString("\n#### 全局设定（所有小说生效）\n")
		for _, item := range global {
			fmt.Fprintf(&b, "- [setting_id:%d | %s] %s\n", item.ID, item.Category, item.Content)
		}
	}
	if len(novelSets) > 0 {
		b.WriteString("\n#### 本小说专属设定\n")
		for _, item := range novelSets {
			fmt.Fprintf(&b, "- [setting_id:%d | %s] %s\n", item.ID, item.Category, item.Content)
		}
	}
	n, err := llm.CountTokens(b.String())
	if err != nil {
		return 0, fmt.Errorf("count setting tokens: %w", err)
	}
	return n, nil
}

// splitByGlobal 把 items 按 IsGlobal 拆成（全局、小说专属）两组，保持原相对顺序。
func splitByGlobal(items []SettingItem) (global, novel []SettingItem) {
	for _, item := range items {
		if item.IsGlobal {
			global = append(global, item)
		} else {
			novel = append(novel, item)
		}
	}
	return
}

package preference

import (
	"fmt"
	"strings"

	"github.com/sigpanic/goink/internal/llm"
)

// PreferencesTokenBudget 是注入 system 消息时偏好的软上限（token）。
// 设计依据：4k 占 200k 上下文窗口 2%，中文 4k 约等于 2-3k 字，能装 30-80 条偏好。
// 超预算时按 created_at DESC 保留最新，末尾追加截断提示（让 LLM 知道有内容被截断、应合并而非新建）。
const PreferencesTokenBudget = 4000

// FormatPreferences 格式化偏好列表为可读文本，按全局/小说专属分组，每条带 id 前缀。
// 用于 NovelProfile 注入 system 消息，agent 看到后可通过 id 调 upsert_preference 更新。
// 空 items 返回空字符串。
//
// 排序：调用方（NovelProfile）SQL 已按 is_global DESC, created_at DESC 排序——
// 全局组在前、组内最新在前。FormatPreferences 信任调用方顺序，不重复排序。
// 超预算时丢弃"最旧"=列表末尾的条目，保留用户最近创建/更新的。
//
// 超预算处理：逐条累加 token，超 PreferencesTokenBudget 时停止拼接并丢弃剩余条目，
// 末尾追加截断提示。返回 (text, truncatedCount)。
func FormatPreferences(items []PreferenceItem) (string, int) {
	if len(items) == 0 {
		return "", 0
	}

	// 调用方 SQL 已排序，这里只按 IsGlobal 分组（保持原相对顺序）。
	global, novelPrefs := splitByGlobal(items)

	var b strings.Builder
	b.WriteString("【偏好】\n")
	used, _ := llm.CountTokens(b.String())

	totalWritten := 0
	budgetExceeded := false

	for _, group := range []struct {
		name  string
		items []PreferenceItem
	}{
		{"全局偏好（所有小说生效）", global},
		{"本小说专属偏好", novelPrefs},
	} {
		if len(group.items) == 0 || budgetExceeded {
			continue
		}
		header := fmt.Sprintf("\n#### %s\n", group.name)
		headerTokens, _ := llm.CountTokens(header)
		if used+headerTokens > PreferencesTokenBudget {
			budgetExceeded = true
			continue
		}
		b.WriteString(header)
		used += headerTokens
		written := 0
		for _, item := range group.items {
			line := fmt.Sprintf("- [preference_id:%d | %s] %s\n", item.ID, item.Category, item.Content)
			lineTokens, _ := llm.CountTokens(line)
			if used+lineTokens > PreferencesTokenBudget {
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
		fmt.Fprintf(&b, "\n⚠️ 偏好总量超过 %d token 软上限，已保留最新 %d 条，被截断 %d 条。请优先用 upsert_preference 合并已有条目（基于 id 更新 content，把多条偏好合并为一条）而非新建，避免预算持续膨胀。\n",
			PreferencesTokenBudget, totalWritten, truncated)
		return b.String(), truncated
	}

	return b.String(), 0
}

// CountPreferencesTokens 估算偏好列表全量 token 数（不截断），用于 App 层报告超预算状态。
// 与 FormatPreferences 输出格式一致，但不做截断，返回完整文本的 token 数。
// 用于 GetPreferences 返回 token_count + over_budget，前端可据此显示但不阻塞。
func CountPreferencesTokens(items []PreferenceItem) (int, error) {
	if len(items) == 0 {
		return 0, nil
	}
	var b strings.Builder
	b.WriteString("【偏好】\n")
	global, novelPrefs := splitByGlobal(items)
	if len(global) > 0 {
		b.WriteString("\n#### 全局偏好（所有小说生效）\n")
		for _, item := range global {
			fmt.Fprintf(&b, "- [preference_id:%d | %s] %s\n", item.ID, item.Category, item.Content)
		}
	}
	if len(novelPrefs) > 0 {
		b.WriteString("\n#### 本小说专属偏好\n")
		for _, item := range novelPrefs {
			fmt.Fprintf(&b, "- [preference_id:%d | %s] %s\n", item.ID, item.Category, item.Content)
		}
	}
	n, err := llm.CountTokens(b.String())
	if err != nil {
		return 0, fmt.Errorf("count preference tokens: %w", err)
	}
	return n, nil
}

// splitByGlobal 把 items 按 IsGlobal 拆成（全局、小说专属）两组，保持原相对顺序。
func splitByGlobal(items []PreferenceItem) (global, novel []PreferenceItem) {
	for _, item := range items {
		if item.IsGlobal {
			global = append(global, item)
		} else {
			novel = append(novel, item)
		}
	}
	return
}

package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"novel/internal/llm"
)

// truncateErrMsg 按 rune 截断字符串到 maxLen，用于防止过长内容（含潜在注入载体）进入 LLM 上下文。
// rune 截断避免破坏 UTF-8 多字节字符。
func truncateErrMsg(s string, maxLen int) string {
	r := []rune(s)
	if len(r) > maxLen {
		return string(r[:maxLen]) + "..."
	}
	return s
}

// FriendlyError 将 LLM 错误转换为用户友好的消息。
// 原始 error 应由调用方另行记录日志。
func FriendlyError(err error) string {
	if errors.Is(err, context.Canceled) {
		return ""
	}
	var apiErr *llm.APIError
	if errors.As(err, &apiErr) {
		// 网络错误/首字节超时：StatusCode=0，无 HTTP 状态码
		// base 用"网络错误"区分网络场景与其他对话错误
		if apiErr.StatusCode == 0 {
			const base = "网络错误"
			if msg := strings.TrimSpace(apiErr.Message); msg != "" {
				return fmt.Sprintf("%s：%s", base, msg)
			}
			return base
		}
		var base string
		switch apiErr.StatusCode {
		case 401:
			base = "API Key 无效，请在设置中检查"
		case 403:
			base = "API Key 无权限"
		case 429:
			base = "请求过于频繁，请稍后重试"
		default:
			if apiErr.StatusCode >= 500 {
				base = "AI 服务暂时不可用，请稍后重试"
			} else {
				base = "对话出错，请重试"
			}
		}
		if msg := strings.TrimSpace(apiErr.Message); msg != "" {
			return fmt.Sprintf("%s（HTTP %d：%s）", base, apiErr.StatusCode, msg)
		}
		return base
	}
	return "对话出错，请重试"
}

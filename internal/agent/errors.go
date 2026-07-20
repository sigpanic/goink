package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"novel/internal/llm"
)

// FriendlyError 将 LLM 错误转换为用户友好的消息。
// 原始 error 应由调用方另行记录日志。
func FriendlyError(err error) string {
	if errors.Is(err, context.Canceled) {
		return ""
	}
	var apiErr *llm.APIError
	if errors.As(err, &apiErr) {
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

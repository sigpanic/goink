package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sigpanic/goink/internal/llm"
)

func TestFriendlyErrorPreservesFailureAndInterruptReasons(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"ordinary error", errors.New("加载 API 历史失败"), "对话出错：加载 API 历史失败"},
		{"missing terminal", fmt.Errorf("gemini stream ended without a terminal interaction: %w", io.ErrUnexpectedEOF), "对话出错：gemini stream ended without a terminal interaction: unexpected EOF"},
		{"deadline", fmt.Errorf("request failed: %w", context.DeadlineExceeded), "对话出错：request failed: context deadline exceeded"},
		{"HTTP error", &llm.APIError{StatusCode: 429, Message: "quota exceeded"}, "请求过于频繁，请稍后重试（HTTP 429：quota exceeded）"},
		{"protocol error", &llm.APIError{Kind: "invalid_response", Message: "missing interaction status"}, "对话出错：missing interaction status"},
		{"tool arguments", &llm.APIError{Kind: "tool_args_invalid", Message: "invalid arguments JSON"}, "对话出错：invalid arguments JSON"},
		{"empty API message", &llm.APIError{Kind: "invalid_response"}, "对话出错"},
		{"loop", &LoopInterrupt{Reason: "检测到重复调用循环，已中断对话"}, "检测到重复调用循环，已中断对话"},
		{"max turns", &MaxTurnsInterrupt{Reason: "已达最大轮数，对话结束"}, "已达最大轮数，对话结束"},
		{"tool failures", &ToolFailureInterrupt{Reason: "工具 read 连续系统错误 5 次，已中断对话"}, "工具 read 连续系统错误 5 次，已中断对话"},
		{"cancelled", fmt.Errorf("request failed: %w", context.Canceled), ""},
		{"cancelled API error", &llm.APIError{Message: "request failed", Cause: context.Canceled}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, FriendlyError(tc.err))
		})
	}
}

func TestFriendlyErrorIncludesNetworkCauseWithoutDuplication(t *testing.T) {
	cause := &net.DNSError{Err: "no such host", Name: "provider.invalid", IsNotFound: true}
	message := FriendlyError(&llm.APIError{Message: "网络连接被拒绝或域名无法解析，请检查网络配置", Cause: cause})
	require.Contains(t, message, "网络错误：网络连接被拒绝或域名无法解析")
	require.Contains(t, message, cause.Error())

	message = FriendlyError(&llm.APIError{Message: "SSE stream read error: unexpected EOF", Cause: io.ErrUnexpectedEOF})
	require.Equal(t, "网络错误：SSE stream read error: unexpected EOF", message)
	require.Equal(t, 1, strings.Count(message, io.ErrUnexpectedEOF.Error()))
}

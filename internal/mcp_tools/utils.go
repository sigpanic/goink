package mcp_tools

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/sigpanic/goink/internal/chapter"
	"github.com/sigpanic/goink/internal/web"
)

func webToolFailure(operation string, err error) (*ToolResult, error) {
	var apiErr *web.APIError
	if errors.As(err, &apiErr) {
		var message string
		switch apiErr.StatusCode {
		case http.StatusBadRequest:
			message = "远程服务拒绝当前请求，请调整参数"
		case http.StatusUnauthorized:
			message = "DeepSeek API Key 无效，请检查配置"
		case http.StatusPaymentRequired:
			message = "DeepSeek 余额不足，请充值后重试"
		case http.StatusForbidden:
			message = "远程服务的安全策略拒绝了当前请求"
		case http.StatusNotFound:
			message = "远程服务未获取到可用内容，请更换来源"
		case http.StatusRequestEntityTooLarge:
			message = "请求内容超过远程服务的大小限制"
		}
		if message != "" {
			return &ToolResult{Error: operation + "失败：" + message}, nil
		}
	}
	return nil, fmt.Errorf("%s失败: %w", operation, err)
}

// ensureChapterIDsInNovel 批量校验章节存在且属于当前小说。
// 仅用于工具写入具体章节引用；未来计划的 reading number 不适用。
func ensureChapterIDsInNovel(ctx context.Context, tc ToolContext, ids []int64) (*ToolResult, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	missingIDs, err := chapter.NewStore(tc.DB, tc.LoggerOrDefault()).MissingIDsByNovel(ctx, nil, tc.NovelID, ids)
	if err != nil {
		return nil, fmt.Errorf("query chapters: %w", err)
	}
	if len(missingIDs) > 0 {
		missing := make([]string, len(missingIDs))
		for i, id := range missingIDs {
			missing[i] = fmt.Sprint(id)
		}
		return &ToolResult{Success: false, Error: "章节 " + strings.Join(missing, "、") + " 不存在或不属于当前小说"}, nil
	}
	return nil, nil
}

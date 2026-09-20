package mcp_tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/sigpanic/goink/internal/chapter"
)

// ensureChapterIDsInNovel 批量校验章节存在且属于当前小说。
// 仅用于工具写入具体章节引用；未来计划的 reading number 不适用。
func ensureChapterIDsInNovel(ctx context.Context, tc ToolContext, ids []int64) (*ToolResult, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	missingIDs, err := chapter.NewStore(tc.DB, tc.LoggerOrDefault()).MissingIDsByNovel(ctx, tc.NovelID, ids)
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

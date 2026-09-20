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

	unique := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}

	var foundIDs []int64
	if err := tc.DB.WithContext(ctx).Model(&chapter.Chapter{}).
		Where("novel_id = ? AND id IN ?", tc.NovelID, unique).
		Pluck("id", &foundIDs).Error; err != nil {
		return nil, fmt.Errorf("query chapters: %w", err)
	}
	found := make(map[int64]struct{}, len(foundIDs))
	for _, id := range foundIDs {
		found[id] = struct{}{}
	}

	missing := make([]string, 0, len(unique))
	for _, id := range unique {
		if _, ok := found[id]; !ok {
			missing = append(missing, fmt.Sprint(id))
		}
	}
	if len(missing) > 0 {
		return &ToolResult{Success: false, Error: "章节 " + strings.Join(missing, "、") + " 不存在或不属于当前小说"}, nil
	}
	return nil, nil
}

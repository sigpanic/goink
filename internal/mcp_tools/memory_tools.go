//go:build cgo

package mcp_tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sigpanic/goink/internal/chapter"
	"github.com/sigpanic/goink/internal/rag"
)

// ── search_story_memory ──────────────────────────────────

// SearchStoryMemoryArgs 是 search_story_memory 的参数。
type SearchStoryMemoryArgs struct {
	Query          string   `json:"query" jsonschema:"required,description=语义搜索查询。用自然语言描述你想找的内容" validate:"required"`
	TopK           int      `json:"top_k" jsonschema:"description=返回结果数量,default=5,minimum=1,maximum=20" validate:"omitempty,min=1,max=20"`
	MinRelevance   float64  `json:"min_relevance" jsonschema:"description=相关度阈值 0-1,default=0.5" validate:"omitempty,min=0,max=1"`
	ChapterNumbers []int    `json:"chapter_numbers" jsonschema:"description=限定章节号范围，空表示不限制"`
	ChunkTypes     []string `json:"chunk_types" jsonschema:"description=限定块类型：summary(章节摘要) / chapter_brief(章节概要) / content(正文内容)，空表示全部"`
}

// SearchStoryMemoryTool 语义检索小说记忆。
type SearchStoryMemoryTool struct{}

func (t *SearchStoryMemoryTool) Name() string           { return "search_story_memory" }
func (t *SearchStoryMemoryTool) Description() string    { return searchStoryMemoryDescription }
func (t *SearchStoryMemoryTool) Category() ToolCategory { return CategoryMemoryRetrieval }

func (t *SearchStoryMemoryTool) JSONSchema() json.RawMessage {
	return SchemaOf(SearchStoryMemoryArgs{})
}
func (t *SearchStoryMemoryTool) ExposeToLLM() bool { return true }
func (t *SearchStoryMemoryTool) NewArgs() any      { return &SearchStoryMemoryArgs{} }

func (t *SearchStoryMemoryTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*SearchStoryMemoryArgs)

	if a.TopK == 0 {
		a.TopK = 5
	}
	if a.MinRelevance == 0 {
		a.MinRelevance = 0.5
	}

	vs := rag.GetVectorStore()
	if vs == nil {
		return &ToolResult{Success: false, Error: "向量检索不可用，ONNX 模型未安装，请先下载模型"}, nil
	}

	// 1. 过量获取，提高召回
	fetchK := min(a.TopK*2, 40)

	var filter *rag.SearchFilter
	if len(a.ChapterNumbers) > 0 || len(a.ChunkTypes) > 0 {
		filter = &rag.SearchFilter{
			ChapterNumbers: a.ChapterNumbers,
			ChunkTypes:     a.ChunkTypes,
		}
	}

	results, err := vs.Search(ctx, tc.NovelID, a.Query, fetchK, filter)
	if err != nil {
		return nil, fmt.Errorf("向量检索失败: %w", err)
	}

	// 2. 相关度过滤
	filtered := make([]rag.SearchResult, 0, len(results))
	for _, r := range results {
		if r.Relevance >= a.MinRelevance {
			filtered = append(filtered, r)
		}
	}

	if len(filtered) == 0 {
		return &ToolResult{
			Success: true,
			Data: map[string]any{
				"query":   a.Query,
				"total":   0,
				"message": "未找到相关记忆，可以尝试更换查询词或降低相关度阈值",
			},
		}, nil
	}

	// 3. MMR 重排序
	reranked := rag.MMRRerank(a.Query, filtered, a.TopK, 0.7)

	// 4. 查询章节元数据
	chapterNums := make([]int, 0, len(reranked))
	seen := make(map[int]bool, len(reranked))
	for _, r := range reranked {
		if r.ChapterNumber > 0 && !seen[r.ChapterNumber] {
			chapterNums = append(chapterNums, r.ChapterNumber)
			seen[r.ChapterNumber] = true
		}
	}

	var chapters []chapter.Chapter
	if len(chapterNums) > 0 {
		if err := tc.DB.WithContext(ctx).
			Where("novel_id = ? AND chapter_number IN ?", tc.NovelID, chapterNums).
			Find(&chapters).Error; err != nil {
			return nil, fmt.Errorf("查询章节元数据失败: %w", err)
		}
	}
	chapMap := make(map[int]chapter.Chapter, len(chapters))
	for _, ch := range chapters {
		chapMap[ch.ChapterNumber] = ch
	}

	// 5. 格式化 Markdown 输出
	var sb strings.Builder
	sb.WriteString("## 语义搜索结果\n\n")
	fmt.Fprintf(&sb, "**查询：** %s  \n", a.Query)
	fmt.Fprintf(&sb, "**结果数：** %d", len(reranked))

	maxRelevance := 0.0
	for i, r := range reranked {
		if r.Relevance > maxRelevance {
			maxRelevance = r.Relevance
		}

		ch, ok := chapMap[r.ChapterNumber]
		sourceLabel := chunkTypeLabel(r.SourceType)
		fmt.Fprintf(&sb, "\n\n### %d. ", i+1)
		if ok && ch.ChapterNumber > 0 {
			fmt.Fprintf(&sb, "第%d章 %s", ch.ChapterNumber, ch.Title)
		} else {
			sb.WriteString("未知章节")
		}
		fmt.Fprintf(&sb, " — %s（相关度：%.2f）\n\n", sourceLabel, r.Relevance)
		sb.WriteString(r.Content)
	}

	fmt.Fprintf(&sb, "\n\n> 最高相关度：%.2f | 查询：%s", maxRelevance, a.Query)

	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"query":         a.Query,
			"total":         len(reranked),
			"max_relevance": fmt.Sprintf("%.2f", maxRelevance),
			"content":       sb.String(),
		},
	}, nil
}

// chunkTypeLabel 将块类型转为中文标签。
func chunkTypeLabel(t string) string {
	switch t {
	case "summary":
		return "章节摘要"
	case "chapter_brief":
		return "章节概要"
	case "content":
		return "正文内容"
	default:
		return t
	}
}

const searchStoryMemoryDescription = `语义检索小说记忆，在已索引的章节内容中查找与查询最相关的文本片段。

支持的块类型（chunk_types 过滤）：
- summary：章节摘要（AI 生成的高密度剧情总结）
- chapter_brief：章节概要（标题 + 摘要 + 正文开头）
- content：正文内容块（420 token 的文本窗口）

返回每个结果的来源章节、相关度分数和内容文本。相关度分数 0-1，越高越匹配。
当需要查找特定情节、对话、场景或细节时使用此工具，而非逐个读取章节文件。`

// ── 注册 ──────────────────────────────────────────────────

// RegisterMemoryTools 注册记忆检索工具。
func RegisterMemoryTools(r *Registry) {
	r.Register(&SearchStoryMemoryTool{})
}

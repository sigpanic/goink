package mcp_tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"gorm.io/gorm"

	"novel/internal/chapter"
	"novel/internal/novel"
	"novel/internal/storage"
)

// ── get_chapter_list ─────────────────────────────────

// GetChapterListArgs 是 get_chapter_list 的参数。
type GetChapterListArgs struct {
	PageArgs // 嵌入分页参数
}

// GetChapterListTool 获取章节列表，按章节号降序。
type GetChapterListTool struct{}

func (t *GetChapterListTool) Name() string { return "get_chapter_list" }
func (t *GetChapterListTool) Description() string {
	return "获取小说的章节列表，支持分页。按章节号降序排列（最新的在前）。返回每章的 id、章节号、标题、字数、摘要。"
}
func (t *GetChapterListTool) Category() ToolCategory { return CategoryNovelManagement }

func (t *GetChapterListTool) JSONSchema() json.RawMessage {
	return SchemaOf(GetChapterListArgs{})
}

func (t *GetChapterListTool) ExposeToLLM() bool { return true }
func (t *GetChapterListTool) NewArgs() any      { return &GetChapterListArgs{} }

func (t *GetChapterListTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*GetChapterListArgs)
	a.NormalizePage()

	chStore := chapter.NewStore(tc.DB, slog.Default())
	result, err := chStore.ListByNovel(ctx, tc.NovelID, chapter.ListByNovelOptions{
		PageParams: storage.PageParams{Page: a.Page, Size: a.Size},
		Order:      "desc",
	})
	if err != nil {
		return nil, fmt.Errorf("list chapters: %w", err)
	}

	items := make([]map[string]any, len(result.Items))
	for i, ch := range result.Items {
		items[i] = map[string]any{
			"id":             ch.ID,
			"chapter_number": ch.ChapterNumber,
			"title":          ch.Title,
			"word_count":     ch.WordCount,
			"summary":        ch.Summary,
			"created_at":     ch.CreatedAt,
			"updated_at":     ch.UpdatedAt,
		}
	}

	data := PageMeta(result)
	data["items"] = items

	return &ToolResult{
		Success: true,
		Data:    data,
	}, nil
}

// ── get_preferences ─────────────────────────────────

// GetPreferencesArgs 是 get_preferences 的参数（无参数，直接返回全部偏好）。
type GetPreferencesArgs struct{}

// GetPreferencesTool 获取创作偏好，包括全局偏好和当前小说的专属偏好。
type GetPreferencesTool struct{}

func (t *GetPreferencesTool) Name() string { return "get_preferences" }
func (t *GetPreferencesTool) Description() string {
	return "获取所有创作偏好，包括全局偏好（所有小说生效）和当前小说的专属偏好。返回格式化文本，按全局→小说专属分组展示。当需要确认长期创作规则、风格约束、用户指令时调用。"
}
func (t *GetPreferencesTool) Category() ToolCategory { return CategoryMemoryRetrieval }

func (t *GetPreferencesTool) JSONSchema() json.RawMessage { return SchemaOf(GetPreferencesArgs{}) }
func (t *GetPreferencesTool) ExposeToLLM() bool           { return true }
func (t *GetPreferencesTool) NewArgs() any                { return &GetPreferencesArgs{} }

func (t *GetPreferencesTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	store := novel.NewStore(tc.DB, slog.Default())
	items, err := store.ListPreferences(ctx, tc.NovelID)
	if err != nil {
		return nil, fmt.Errorf("list preferences: %w", err)
	}

	formatted := formatPreferences(items)
	return &ToolResult{
		Success: true,
		Data:    map[string]any{"content": formatted},
	}, nil
}

// ── create_preference ────────────────────────────────

// CreatePreferenceItem 是 create_preference 的单条参数。
type CreatePreferenceItem struct {
	IsGlobal bool   `json:"is_global" jsonschema:"description=是否为全局偏好（所有小说生效），默认false"`
	Category string `json:"category" jsonschema:"required,description=偏好分类(自由文本标签)" validate:"required"`
	Content  string `json:"content" jsonschema:"required,description=偏好内容" validate:"required"`
}

// CreatePreferenceArgs 是 create_preference 的参数。
type CreatePreferenceArgs struct {
	Preferences []CreatePreferenceItem `json:"preferences" jsonschema:"required,description=要创建的偏好列表（1-5个）" validate:"required,min=1,max=5,dive"`
}

// CreatePreferenceTool 创建创作偏好条目。
type CreatePreferenceTool struct{}

func (t *CreatePreferenceTool) Name() string { return "create_preference" }
func (t *CreatePreferenceTool) Description() string {
	return "批量创建创作偏好（1-5个）。保证原子性，失败时返回具体条目原因。" +
		"偏好按自由文本 Category 归类，同 Category 即为同类条目。" +
		"如果已存在相似分类的偏好，应优先调用 update_preference 对已有条目做增量合并（在原文基础上追加），而非创建重复条目。"
}
func (t *CreatePreferenceTool) Category() ToolCategory { return CategoryWritingAssistant }

func (t *CreatePreferenceTool) JSONSchema() json.RawMessage { return SchemaOf(CreatePreferenceArgs{}) }
func (t *CreatePreferenceTool) ExposeToLLM() bool           { return true }
func (t *CreatePreferenceTool) NewArgs() any                { return &CreatePreferenceArgs{} }

func (t *CreatePreferenceTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*CreatePreferenceArgs)

	var ids []int64
	var failedName string
	var failedErr error
	err := tc.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, item := range a.Preferences {
			pref := novel.PreferenceItem{
				NovelID:  tc.NovelID,
				IsGlobal: item.IsGlobal,
				Category: item.Category,
				Content:  item.Content,
			}
			if err := tx.Create(&pref).Error; err != nil {
				failedName = item.Category
				failedErr = err
				return err
			}
			ids = append(ids, pref.ID)
		}
		return nil
	})
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("创建偏好 [%s] 失败: %s", failedName, failedErr),
		}, nil
	}

	return &ToolResult{
		Success: true,
		Data:    map[string]any{"ids": ids, "count": len(ids)},
	}, nil
}

// ── update_preference ────────────────────────────────

// UpdatePreferenceArgs 是 update_preference 的参数。
type UpdatePreferenceArgs struct {
	PreferenceID int64  `json:"preference_id" jsonschema:"required,description=偏好条目ID" validate:"required,min=1"`
	Category     string `json:"category" jsonschema:"description=新的分类标签"`
	Content      string `json:"content" jsonschema:"description=新的偏好内容（增量合并时传入合并后的完整内容）"`
	IsGlobal     bool   `json:"is_global" jsonschema:"description=是否改为全局偏好"`
}

// UpdatePreferenceTool 更新创作偏好条目（PATCH 语义）。
type UpdatePreferenceTool struct{}

func (t *UpdatePreferenceTool) Name() string { return "update_preference" }
func (t *UpdatePreferenceTool) Description() string {
	return "更新已有的创作偏好条目。只需传入要修改的字段（PATCH 语义），未传入的字段保持原值。" +
		"增量合并时，先读取现有内容，再拼接新内容后传入 content 字段。"
}
func (t *UpdatePreferenceTool) Category() ToolCategory { return CategoryWritingAssistant }

func (t *UpdatePreferenceTool) JSONSchema() json.RawMessage { return SchemaOf(UpdatePreferenceArgs{}) }
func (t *UpdatePreferenceTool) ExposeToLLM() bool           { return true }
func (t *UpdatePreferenceTool) NewArgs() any                { return &UpdatePreferenceArgs{} }

func (t *UpdatePreferenceTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*UpdatePreferenceArgs)

	var item novel.PreferenceItem
	if err := tc.DB.WithContext(ctx).First(&item, a.PreferenceID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &ToolResult{Success: false, Error: fmt.Sprintf("偏好条目 %d 不存在", a.PreferenceID)}, nil
		}
		return nil, fmt.Errorf("query preference: %w", err)
	}

	// 归属校验：只能修改全局偏好或当前小说的偏好
	if !item.IsGlobal && item.NovelID != tc.NovelID {
		return &ToolResult{Success: false, Error: fmt.Sprintf("偏好条目 %d 不属于当前小说", a.PreferenceID)}, nil
	}

	if err := json.Unmarshal(tc.RawArgs, &item); err != nil {
		return &ToolResult{Success: false, Error: "参数格式不正确: " + err.Error()}, nil
	}

	if err := tc.DB.WithContext(ctx).Save(&item).Error; err != nil {
		return nil, fmt.Errorf("save preference: %w", err)
	}

	return &ToolResult{
		Success: true,
		Data:    map[string]any{"preference_id": item.ID},
	}, nil
}

// ── 格式化 ──────────────────────────────────────────────

func formatPreferences(items []novel.PreferenceItem) string {
	if len(items) == 0 {
		return "暂无创作偏好。"
	}

	var globalLines, novelLines []string
	for _, item := range items {
		line := fmt.Sprintf("- 【%s】%s [preference_id:%d]", item.Category, item.Content, item.ID)
		if item.IsGlobal {
			globalLines = append(globalLines, line)
		} else {
			novelLines = append(novelLines, line)
		}
	}

	var parts []string
	parts = append(parts, "### 创作偏好")

	if len(globalLines) > 0 {
		parts = append(parts, "\n#### 全局偏好（所有小说生效）")
		parts = append(parts, globalLines...)
	}
	if len(novelLines) > 0 {
		parts = append(parts, "\n#### 本小说专属偏好")
		parts = append(parts, novelLines...)
	}

	return strings.Join(parts, "\n")
}

// ── 注册 ──────────────────────────────────────────────

// RegisterNovelTools 注册小说管理类工具。
func RegisterNovelTools(r *Registry) {
	r.Register(&GetChapterListTool{})
	r.Register(&GetPreferencesTool{})
	r.Register(&CreatePreferenceTool{})
	r.Register(&UpdatePreferenceTool{})
}

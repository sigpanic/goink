package mcp_tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/storage"
	"github.com/sigpanic/goink/internal/timeline"
)

// ── get_timeline ────────────────────────────────────────

// GetTimelineArgs 是 get_timeline 的参数。
type GetTimelineArgs struct {
	CurrentChapter int    `json:"current_chapter" jsonschema:"description=当前章节号。传入时自动收集附近条目并检测异常。写新章时必填" validate:"omitempty,min=1"`
	Category       string `json:"category" jsonschema:"description=按分类筛选,enum=foreshadowing,enum=user_directive" validate:"omitempty,oneof=foreshadowing user_directive"`
	Status         string `json:"status" jsonschema:"description=按状态筛选,enum=pending,enum=resolved,enum=abandoned" validate:"omitempty,oneof=pending resolved abandoned"`
	Search         string `json:"search" jsonschema:"description=按标题或内容模糊搜索（仅不传 current_chapter 时生效）"`
	PageArgs              // 嵌入分页参数（仅不传 current_chapter 时生效）
}

// GetTimelineTool 获取章节计划 + 伏笔/用户指令总览。
type GetTimelineTool struct{}

func (t *GetTimelineTool) Name() string { return "get_timeline" }
func (t *GetTimelineTool) Description() string {
	return "获取故事时间线总览：章节计划（next/near/far）+ 伏笔和用户指令。两种用法：\n" +
		"- 传入 current_chapter：自动收集附近条目（近期历史+未来+异常标记），不要传分页/过滤 参数\n" +
		"- 不传 current_chapter：分页浏览条目（不含计划），可用 category/status 过滤，需要传分页/过滤 参数"
}
func (t *GetTimelineTool) Category() ToolCategory { return CategoryMemoryRetrieval }

func (t *GetTimelineTool) JSONSchema() json.RawMessage { return SchemaOf(GetTimelineArgs{}) }
func (t *GetTimelineTool) ExposeToLLM() bool           { return true }
func (t *GetTimelineTool) NewArgs() any                { return &GetTimelineArgs{} }

func (t *GetTimelineTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*GetTimelineArgs)
	a.NormalizePage()

	store := timeline.NewStore(tc.DB, tc.LoggerOrDefault())

	if a.CurrentChapter > 0 {
		plans, err := store.GetPlans(ctx, tc.NovelID)
		if err != nil {
			return nil, fmt.Errorf("query plans: %w", err)
		}
		return t.executeContext(ctx, a, tc, store, plans)
	}
	return t.executeFull(ctx, a, tc, store)
}

func (t *GetTimelineTool) executeContext(ctx context.Context, a *GetTimelineArgs, tc ToolContext, store *timeline.Store, plans []timeline.ChapterPlan) (*ToolResult, error) {
	// 近期历史（target_chapter < current，最近 10 条）
	history, err := store.ListBefore(ctx, tc.NovelID, a.CurrentChapter, 10)
	if err != nil {
		return nil, fmt.Errorf("query history: %w", err)
	}

	// 异常：前面的章还有 pending（该回收没回收）
	pendingBefore, err := store.ListPendingBefore(ctx, tc.NovelID, a.CurrentChapter)
	if err != nil {
		return nil, fmt.Errorf("query pending before: %w", err)
	}
	anomalies := pendingBefore // target_chapter < current && status=pending

	// 未来条目
	future, err := store.ListAfter(ctx, tc.NovelID, a.CurrentChapter)
	if err != nil {
		return nil, fmt.Errorf("query future: %w", err)
	}

	// 异常补充：未来条目中已 resolved 的（不应提前回收）
	for _, e := range future {
		if e.Status == "resolved" {
			anomalies = append(anomalies, e)
		}
	}

	formatted := formatTimelineContext(plans, history, anomalies, future, a.CurrentChapter)

	return &ToolResult{
		Success: true,
		Data:    map[string]any{"content": formatted},
	}, nil
}

func (t *GetTimelineTool) executeFull(ctx context.Context, a *GetTimelineArgs, tc ToolContext, store *timeline.Store) (*ToolResult, error) {
	result, err := store.ListByNovel(ctx, tc.NovelID, timeline.ListByNovelOptions{
		PageParams: storage.PageParams{Page: a.Page, Size: a.Size},
		Search:     a.Search,
		Category:   a.Category,
		Status:     a.Status,
		Order:      "target_chapter ASC, importance DESC",
	})
	if err != nil {
		return nil, fmt.Errorf("list timeline: %w", err)
	}

	formatted := formatTimelineFull(result.Items)

	data := PageMeta(result)
	data["content"] = formatted

	return &ToolResult{Success: true, Data: data}, nil
}

// ── create_timeline_entry ───────────────────────────────

// CreateTimelineEntryItem 是单条伏笔/用户指令的创建参数。
type CreateTimelineEntryItem struct {
	Category      string `json:"category" jsonschema:"required,description=条目类型,enum=foreshadowing,enum=user_directive" validate:"required,oneof=foreshadowing user_directive"`
	Title         string `json:"title" jsonschema:"required,description=简短标题"                                   validate:"required"`
	Content       string `json:"content" jsonschema:"description=详细描述"`
	DetailJSON    string `json:"detail_json" jsonschema:"description=字符串形式的JSON，结构化数据"`
	TargetChapter int    `json:"target_chapter" jsonschema:"required,description=预计回收章节号（不准确不要紧，后续可调整）"        validate:"required,min=1"`
	Importance    int    `json:"importance" jsonschema:"description=重要度1-5,default=3,minimum=1,maximum=5"         validate:"omitempty,min=1,max=5"`
	SourceChapter int    `json:"source_chapter" jsonschema:"description=在哪章创建/埋下的"`
	Source        string `json:"source" jsonschema:"description=来源,default=ai"`
}

// CreateTimelineEntryArgs 是 create_timeline_entry 的参数。
type CreateTimelineEntryArgs struct {
	Entries []CreateTimelineEntryItem `json:"entries" jsonschema:"required,description=要添加的条目列表（1-6个）" validate:"required,min=1,max=6,dive"`
}

// CreateTimelineEntryTool 批量创建伏笔和用户指令。
type CreateTimelineEntryTool struct{}

func (t *CreateTimelineEntryTool) Name() string { return "create_timeline_entry" }
func (t *CreateTimelineEntryTool) Description() string {
	return "批量创建伏笔或用户指令（1-6条）。保证原子性，失败时返回具体条目原因。" +
		"每章写完后发现新埋的伏笔或用户指令时调用。" +
		"category 为 foreshadowing（伏笔）或 user_directive（用户创作指令）。"
}
func (t *CreateTimelineEntryTool) Category() ToolCategory { return CategoryWritingAssistant }

func (t *CreateTimelineEntryTool) JSONSchema() json.RawMessage {
	return SchemaOf(CreateTimelineEntryArgs{})
}
func (t *CreateTimelineEntryTool) ExposeToLLM() bool { return true }
func (t *CreateTimelineEntryTool) NewArgs() any      { return &CreateTimelineEntryArgs{} }

func (t *CreateTimelineEntryTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*CreateTimelineEntryArgs)

	var ids []int64
	var failedName string
	var failedErr error
	err := tc.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, item := range a.Entries {
			source := item.Source
			if source == "" {
				source = "ai"
			}
			importance := item.Importance
			if importance == 0 {
				importance = 3
			}
			entry := timeline.TimelineEntry{
				NovelID:       tc.NovelID,
				Category:      item.Category,
				Title:         item.Title,
				Content:       item.Content,
				DetailJSON:    item.DetailJSON,
				TargetChapter: item.TargetChapter,
				Importance:    importance,
				SourceChapter: item.SourceChapter,
				Source:        source,
				Status:        "pending",
			}
			if err := tx.Create(&entry).Error; err != nil {
				failedName = item.Title
				failedErr = err
				return err
			}
			ids = append(ids, entry.ID)
		}
		return nil
	})
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("创建时间线条目 [%s] 失败: %s", failedName, failedErr),
		}, nil
	}

	return &ToolResult{
		Success: true,
		Data:    map[string]any{"ids": ids, "count": len(ids)},
	}, nil
}

// ── update_timeline_entry ───────────────────────────────

// UpdateTimelineEntryArgs 是 update_timeline_entry 的参数。
type UpdateTimelineEntryArgs struct {
	EntryID         int64  `json:"entry_id" jsonschema:"required,description=条目ID"              validate:"required,min=1"`
	Title           string `json:"title" jsonschema:"description=新的标题"`
	Content         string `json:"content" jsonschema:"description=新的描述"`
	DetailJSON      string `json:"detail_json" jsonschema:"description=新的结构化数据（完全替换旧的）"`
	TargetChapter   int    `json:"target_chapter" jsonschema:"description=新的目标章节号,minimum=1" validate:"omitempty,min=1"`
	Importance      int    `json:"importance" jsonschema:"description=新的重要度1-5,minimum=1,maximum=5"`
	Status          string `json:"status" jsonschema:"description=新状态,enum=pending,enum=resolved,enum=abandoned" validate:"omitempty,oneof=pending resolved abandoned"`
	ResolvedChapter int    `json:"resolved_chapter" jsonschema:"description=在哪章回收（标记resolved时填入）"`
}

// UpdateTimelineEntryTool 更新伏笔或用户指令。
type UpdateTimelineEntryTool struct{}

func (t *UpdateTimelineEntryTool) Name() string { return "update_timeline_entry" }
func (t *UpdateTimelineEntryTool) Description() string {
	return "更新已有的伏笔或用户指令。只需传入要修改的字段。" +
		"常见用途：回收伏笔（status=resolved + resolved_chapter）、调整 target_chapter、修改内容。" +
		"category 和 source_chapter 创建后不可变。"
}
func (t *UpdateTimelineEntryTool) Category() ToolCategory { return CategoryWritingAssistant }

func (t *UpdateTimelineEntryTool) JSONSchema() json.RawMessage {
	return SchemaOf(UpdateTimelineEntryArgs{})
}
func (t *UpdateTimelineEntryTool) ExposeToLLM() bool { return true }
func (t *UpdateTimelineEntryTool) NewArgs() any      { return &UpdateTimelineEntryArgs{} }

func (t *UpdateTimelineEntryTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*UpdateTimelineEntryArgs)

	if a.Title == "" && a.Content == "" && a.DetailJSON == "" && a.TargetChapter == 0 && a.Importance == 0 && a.Status == "" && a.ResolvedChapter == 0 {
		return &ToolResult{Success: false, Error: "至少需要提供一个要修改的字段"}, nil
	}

	var entry timeline.TimelineEntry
	if err := tc.DB.WithContext(ctx).
		Where("id = ? AND novel_id = ?", a.EntryID, tc.NovelID).
		First(&entry).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &ToolResult{Success: false, Error: fmt.Sprintf("条目 %d 不存在", a.EntryID)}, nil
		}
		return nil, fmt.Errorf("query timeline entry: %w", err)
	}

	if err := json.Unmarshal(tc.RawArgs, &entry); err != nil {
		return &ToolResult{Success: false, Error: "参数格式不正确: " + err.Error()}, nil
	}

	if err := tc.DB.WithContext(ctx).Save(&entry).Error; err != nil {
		return nil, fmt.Errorf("save timeline entry: %w", err)
	}

	return &ToolResult{
		Success: true,
		Data:    map[string]any{"id": entry.ID},
	}, nil
}

// ── update_chapter_plan ─────────────────────────────────

// UpdateChapterPlanArgs 是 update_chapter_plan 的参数。
type UpdateChapterPlanArgs struct {
	Scope   string `json:"scope" jsonschema:"required,description=计划范围,enum=next,enum=near,enum=far" validate:"required,oneof=next near far"`
	Content string `json:"content" jsonschema:"required,description=计划内容，自然语言描述"                   validate:"required"`
}

// UpdateChapterPlanTool 更新章节创作计划。
type UpdateChapterPlanTool struct{}

func (t *UpdateChapterPlanTool) Name() string { return "update_chapter_plan" }
func (t *UpdateChapterPlanTool) Description() string {
	return "更新章节创作计划。三个槽位：\n" +
		"- next：下一章的写作计划\n" +
		"- near：近期的写作计划\n" +
		"- far：远期的写作方向\n" +
		"同一 scope 重复调用会覆盖旧值。写新章前应更新计划以反映最新进展。"
}
func (t *UpdateChapterPlanTool) Category() ToolCategory { return CategoryWritingAssistant }

func (t *UpdateChapterPlanTool) JSONSchema() json.RawMessage {
	return SchemaOf(UpdateChapterPlanArgs{})
}
func (t *UpdateChapterPlanTool) ExposeToLLM() bool { return true }
func (t *UpdateChapterPlanTool) NewArgs() any      { return &UpdateChapterPlanArgs{} }

func (t *UpdateChapterPlanTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*UpdateChapterPlanArgs)

	plan := timeline.ChapterPlan{
		NovelID: tc.NovelID,
		Scope:   a.Scope,
		Content: a.Content,
	}

	if err := timeline.NewStore(tc.DB, tc.LoggerOrDefault()).SavePlan(ctx, &plan); err != nil {
		return nil, fmt.Errorf("save plan: %w", err)
	}

	return &ToolResult{
		Success: true,
		Data: map[string]any{
			"scope": plan.Scope,
		},
	}, nil
}

// ── 格式化 ──────────────────────────────────────────────

func formatTimelineContext(plans []timeline.ChapterPlan, history, anomalies, future []timeline.TimelineEntry, currentChapter int) string {
	var parts []string

	// 章节计划
	parts = append(parts, "### 章节计划")
	planMap := map[string]string{"next": "暂无", "near": "暂无", "far": "暂无"}
	for _, p := range plans {
		if p.Content != "" {
			planMap[p.Scope] = p.Content
		}
	}
	parts = append(parts, fmt.Sprintf("- **next**：%s", planMap["next"]))
	parts = append(parts, fmt.Sprintf("- **near**：%s", planMap["near"]))
	parts = append(parts, fmt.Sprintf("- **far**：%s", planMap["far"]))

	// 近期历史（排除已在异常中的条目，避免重复）
	anomalyIDs := make(map[int64]bool, len(anomalies))
	for _, e := range anomalies {
		anomalyIDs[e.ID] = true
	}
	if len(history) > 0 {
		lines := []string{fmt.Sprintf("\n### 近期历史（最近%d条，截至第%d章）", len(history), currentChapter)}
		for _, e := range history {
			if anomalyIDs[e.ID] {
				continue
			}
			cat := catLabel(e.Category)
			st := statusLabel(e.Status)
			line := fmt.Sprintf("- %s %s [entry_id:%d] — 目标第%d章 — %s", cat, e.Title, e.ID, e.TargetChapter, st)
			lines = append(lines, line)
		}
		parts = append(parts, strings.Join(lines, "\n"))
	}

	// 异常
	if len(anomalies) > 0 {
		lines := []string{"\n### ⚠️ 状态异常"}
		lines = append(lines, "以下条目需要关注和修正：")
		for _, e := range anomalies {
			cat := catLabel(e.Category)
			if e.Status == "pending" && e.TargetChapter < currentChapter {
				lines = append(lines, fmt.Sprintf("- %s %s [entry_id:%d] — 目标第%d章但仍为pending，应在第%d章前回收",
					cat, e.Title, e.ID, e.TargetChapter, currentChapter))
			} else if e.Status == "resolved" && e.TargetChapter >= currentChapter {
				lines = append(lines, fmt.Sprintf("- %s %s [entry_id:%d] — 目标第%d章但已标记resolved，可能提前回收",
					cat, e.Title, e.ID, e.TargetChapter))
			}
		}
		parts = append(parts, strings.Join(lines, "\n"))
	}

	// 未来条目
	if len(future) > 0 {
		lines := []string{fmt.Sprintf("\n### 未来条目（%d条，按目标章节排序）", len(future))}
		for _, e := range future {
			if e.Status == "resolved" {
				continue // 已在异常区展示
			}
			line := fmt.Sprintf("- %s %s [entry_id:%d] 第%d章", catLabel(e.Category), e.Title, e.ID, e.TargetChapter)
			if e.Importance > 0 {
				line += fmt.Sprintf(" [重要度:%d]", e.Importance)
			}
			if e.Status == "abandoned" {
				line += " — 已废弃"
			}
			lines = append(lines, line)
		}
		parts = append(parts, strings.Join(lines, "\n"))
	}

	if len(history) == 0 && len(anomalies) == 0 && len(future) == 0 {
		parts = append(parts, "\n暂无伏笔或用户指令。")
	}

	return strings.Join(parts, "\n")
}

func formatTimelineFull(entries []timeline.TimelineEntry) string {
	var parts []string

	if len(entries) > 0 {
		lines := []string{fmt.Sprintf("### 伏笔与用户指令（%d条）", len(entries))}
		for _, e := range entries {
			line := fmt.Sprintf("- %s %s [entry_id:%d] 第%d章 — %s",
				catLabel(e.Category), e.Title, e.ID, e.TargetChapter, statusLabel(e.Status))
			if e.Importance > 0 {
				line += fmt.Sprintf(" [重要度:%d]", e.Importance)
			}
			lines = append(lines, line)
		}
		parts = append(parts, strings.Join(lines, "\n"))
	} else {
		parts = append(parts, "暂无伏笔或用户指令。")
	}

	return strings.Join(parts, "\n")
}

func catLabel(category string) string {
	switch category {
	case "foreshadowing":
		return "【伏笔】"
	case "user_directive":
		return "【用户指令】"
	}
	return "【" + category + "】"
}

func statusLabel(status string) string {
	switch status {
	case "pending":
		return "pending"
	case "resolved":
		return "已回收 ✓"
	case "abandoned":
		return "已废弃 ✗"
	}
	return status
}

// ── 注册 ──────────────────────────────────────────────

// RegisterTimelineTools 注册时间线管理类工具。
func RegisterTimelineTools(r *Registry) {
	r.Register(&GetTimelineTool{})
	r.Register(&CreateTimelineEntryTool{})
	r.Register(&UpdateTimelineEntryTool{})
	r.Register(&UpdateChapterPlanTool{})
}

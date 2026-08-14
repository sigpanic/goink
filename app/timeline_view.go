package app

import (
	"fmt"

	"github.com/sigpanic/goink/internal/storage"
	"github.com/sigpanic/goink/internal/timeline"
)

// GetChapterPlans 返回指定小说的章节计划（next/near/far 三个槽位）。
func (a *App) GetChapterPlans(novelID int64) ([]timeline.ChapterPlan, error) {
	plans, err := a.timeline.GetPlans(a.ctx, novelID)
	if err != nil {
		return nil, err
	}
	if plans == nil {
		return []timeline.ChapterPlan{}, nil
	}
	return plans, nil
}

// GetTimelineEntries 返回指定小说的全部伏笔/用户指令，供前端列表渲染。
// 4b: 改调 ListByNovel(Size=-1) 全量（废弃 ListByChapterRange，前端 useTimelineEntries
// 传 0,0 全量 + 内存切窗口，行为等价）。显式传 Order 保持原排序。
func (a *App) GetTimelineEntries(novelID int64) ([]timeline.TimelineEntry, error) {
	result, err := a.timeline.ListByNovel(a.ctx, novelID, timeline.ListByNovelOptions{
		PageParams: storage.PageParams{Size: -1},
		Order:      "target_chapter ASC, importance DESC",
	})
	if err != nil {
		return nil, err
	}
	if result.Items == nil {
		return []timeline.TimelineEntry{}, nil
	}
	return result.Items, nil
}

// ── Chapter Plan CRUD ──────────────────────────────────

// UpdateChapterPlanInput 采用 PUT 语义：前端全量传，后端全量替换文件内容。
type UpdateChapterPlanInput struct {
	Scope   string `json:"scope"`   // "next" | "near" | "far"
	Content string `json:"content"` // 计划内容
}

// UpdateChapterPlan 更新章节计划（全量替换文件内容）。
func (a *App) UpdateChapterPlan(novelID int64, input UpdateChapterPlanInput) error {
	plan := &timeline.ChapterPlan{
		NovelID: novelID,
		Scope:   input.Scope,
		Content: input.Content,
	}
	if err := a.timeline.SavePlan(a.ctx, plan); err != nil {
		return fmt.Errorf("update chapter plan: %w", err)
	}
	return nil
}

// ── Timeline Entry CRUD ────────────────────────────────

// CreateTimelineEntryInput 是 CreateTimelineEntry 的参数。
type CreateTimelineEntryInput struct {
	Category      string `json:"category"`                 // "foreshadowing" | "user_directive"，必填
	Title         string `json:"title"`                    // 简短标题，必填
	Content       string `json:"content,omitempty"`        // 详细描述
	DetailJSON    string `json:"detail_json,omitempty"`    // JSON 字符串
	TargetChapter int    `json:"target_chapter"`           // 预计回收章节号，必填
	Importance    int    `json:"importance,omitempty"`     // 重要度 1-5
	SourceChapter int    `json:"source_chapter,omitempty"` // 在哪章创建
	Source        string `json:"source,omitempty"`         // "ai" | "user"
}

// CreateTimelineEntry 创建一条伏笔或用户指令。
func (a *App) CreateTimelineEntry(novelID int64, input CreateTimelineEntryInput) (*timeline.TimelineEntry, error) {
	if input.Category == "" || input.Title == "" || input.TargetChapter == 0 {
		return nil, fmt.Errorf("标题、类型、目标章节不能为空")
	}
	entry := timeline.TimelineEntry{
		NovelID:       novelID,
		Category:      input.Category,
		Title:         input.Title,
		Content:       input.Content,
		DetailJSON:    input.DetailJSON,
		TargetChapter: input.TargetChapter,
		Importance:    input.Importance,
		SourceChapter: input.SourceChapter,
		Source:        input.Source,
		Status:        "pending",
	}
	if entry.Source == "" {
		entry.Source = "user"
	}
	if entry.Importance == 0 {
		entry.Importance = 3
	}
	if err := a.timeline.DB.WithContext(a.ctx).Create(&entry).Error; err != nil {
		return nil, fmt.Errorf("create timeline entry: %w", err)
	}
	return &entry, nil
}

// UpdateTimelineEntryInput 采用 PUT 语义：前端全量传，后端全量覆盖。
// DetailJSON 是 AI 写入字段，不在 input 里，后端 First 加载原值保留。
type UpdateTimelineEntryInput struct {
	Title           string `json:"title"`
	Content         string `json:"content"`
	TargetChapter   int    `json:"target_chapter"`
	Importance      int    `json:"importance"`
	Status          string `json:"status"`           // "pending" | "resolved" | "abandoned"
	ResolvedChapter int    `json:"resolved_chapter"` // 标记 resolved 时填入
}

// UpdateTimelineEntry 更新伏笔或用户指令。PUT 全量覆盖用户可编辑字段。
func (a *App) UpdateTimelineEntry(novelID int64, entryID int64, input UpdateTimelineEntryInput) error {
	var entry timeline.TimelineEntry
	if err := a.timeline.DB.WithContext(a.ctx).
		Where("id = ? AND novel_id = ?", entryID, novelID).
		First(&entry).Error; err != nil {
		return fmt.Errorf("update timeline entry: %w", err)
	}
	// PUT 全量覆盖用户可编辑字段。DetailJSON 是 AI 写入字段，不在 input 里，
	// 保留 First 加载的原值，避免前端编辑保存覆盖 AI 写入的新值（lost update）。
	entry.Title = input.Title
	entry.Content = input.Content
	entry.TargetChapter = input.TargetChapter
	entry.Importance = input.Importance
	entry.Status = input.Status
	entry.ResolvedChapter = input.ResolvedChapter
	if err := a.timeline.DB.WithContext(a.ctx).Save(&entry).Error; err != nil {
		return fmt.Errorf("update timeline entry: %w", err)
	}
	return nil
}

// DeleteTimelineEntry 删除一条伏笔或用户指令。
func (a *App) DeleteTimelineEntry(novelID int64, entryID int64) error {
	if err := a.timeline.DB.WithContext(a.ctx).
		Where("id = ? AND novel_id = ?", entryID, novelID).
		Delete(&timeline.TimelineEntry{}).Error; err != nil {
		return fmt.Errorf("delete timeline entry: %w", err)
	}
	return nil
}

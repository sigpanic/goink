package mcp_tools

import (
	"context"
	"encoding/json"
	"fmt"

	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/novel"
)

// ── upsert_preference ─────────────────────────────────

// UpsertPreferenceItem 是 upsert_preference 的单条参数。
type UpsertPreferenceItem struct {
	ID       *int64 `json:"id,omitempty" jsonschema:"description=已存在偏好ID；不传=新建"`
	Category string `json:"category" jsonschema:"required,description=偏好分类(自由文本标签)" validate:"required"`
	Content  string `json:"content" jsonschema:"required,description=偏好内容" validate:"required"`
	IsGlobal *bool  `json:"is_global,omitempty" jsonschema:"description=是否全局偏好(对所有小说生效)；不传=新建默认false，更新保持原值"`
}

// UpsertPreferenceArgs 是 upsert_preference 的参数。
type UpsertPreferenceArgs struct {
	Preferences []UpsertPreferenceItem `json:"preferences" jsonschema:"required,description=要upsert的偏好列表(1-5个，传id=更新，不传=创建)" validate:"required,min=1,max=5,dive"`
}

// UpsertPreferenceTool 批量 upsert 创作偏好（创建或更新，单事务原子性）。
type UpsertPreferenceTool struct{}

func (t *UpsertPreferenceTool) Name() string { return "upsert_preference" }
func (t *UpsertPreferenceTool) Description() string {
	return "批量创建或更新创作偏好（1-5个，单事务原子性，失败全部回滚）。" +
		"传 id=更新该条（PATCH 语义，只覆盖传入字段），不传 id=新建。" +
		"开局已全量注入偏好到上下文(带 [#id | 分类] 前缀)，更新时从注入里取 id 传入。" +
		"新增相似分类的偏好前，应优先更新已有条目(在原文基础上合并)而非创建重复条目。"
}
func (t *UpsertPreferenceTool) Category() ToolCategory { return CategoryWritingAssistant }

func (t *UpsertPreferenceTool) JSONSchema() json.RawMessage { return SchemaOf(UpsertPreferenceArgs{}) }
func (t *UpsertPreferenceTool) ExposeToLLM() bool           { return true }
func (t *UpsertPreferenceTool) NewArgs() any                { return &UpsertPreferenceArgs{} }

func (t *UpsertPreferenceTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*UpsertPreferenceArgs)

	type failure struct {
		index    int
		category string
		err      error
	}
	var failed *failure
	var ids []int64

	err := tc.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for i, item := range a.Preferences {
			id, err := upsertOnePreference(tx, tc.NovelID, item)
			if err != nil {
				failed = &failure{index: i, category: item.Category, err: err}
				return err
			}
			ids = append(ids, id)
		}
		return nil
	})
	if err != nil {
		if failed == nil {
			return nil, fmt.Errorf("upsert preference: %w", err)
		}
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("第 %d 条偏好 [%s] 失败: %s", failed.index, failed.category, failed.err),
			Data:    map[string]any{"failed_index": failed.index, "failed_category": failed.category},
		}, nil
	}

	return &ToolResult{
		Success: true,
		Data:    map[string]any{"preference_ids": ids, "count": len(ids)},
	}, nil
}

// upsertOnePreference 在事务内 upsert 单条偏好，返回偏好 ID。
func upsertOnePreference(tx *gorm.DB, novelID int64, item UpsertPreferenceItem) (int64, error) {
	// 更新分支
	if item.ID != nil {
		var existing novel.PreferenceItem
		if err := tx.First(&existing, *item.ID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return 0, fmt.Errorf("偏好条目 %d 不存在", *item.ID)
			}
			return 0, fmt.Errorf("query preference: %w", err)
		}
		// 归属校验：只能改全局偏好或当前小说的偏好
		if !existing.IsGlobal && existing.NovelID != novelID {
			return 0, fmt.Errorf("偏好条目 %d 不属于当前小说", *item.ID)
		}
		// PATCH 覆盖
		if item.Category != "" {
			existing.Category = item.Category
		}
		if item.Content != "" {
			existing.Content = item.Content
		}
		if item.IsGlobal != nil {
			existing.IsGlobal = *item.IsGlobal
			// is_global 切换时 NovelID 相应调整
			if *item.IsGlobal {
				existing.NovelID = 0
			} else {
				existing.NovelID = novelID
			}
		}
		if err := tx.Save(&existing).Error; err != nil {
			return 0, fmt.Errorf("save preference: %w", err)
		}
		return existing.ID, nil
	}

	// 创建分支
	pref := novel.PreferenceItem{
		Category: item.Category,
		Content:  item.Content,
		NovelID:  novelID,
	}
	if item.IsGlobal != nil && *item.IsGlobal {
		pref.IsGlobal = true
		pref.NovelID = 0
	}
	if err := tx.Create(&pref).Error; err != nil {
		return 0, fmt.Errorf("create preference: %w", err)
	}
	return pref.ID, nil
}

// ── 注册 ──────────────────────────────────────────────

// RegisterPreferenceTools 注册偏好工具。
func RegisterPreferenceTools(r *Registry) {
	r.Register(&UpsertPreferenceTool{})
}

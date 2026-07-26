package mcp_tools

import (
	"context"
	"encoding/json"
	"fmt"

	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/setting"
)

// ── upsert_setting ──────────────────────────────────

// UpsertSettingItem 是 upsert_setting 的单条参数。
type UpsertSettingItem struct {
	SettingID *int64 `json:"setting_id,omitempty" jsonschema:"description=已存在设定ID；不传=新建"`
	Category  string `json:"category" jsonschema:"required,description=设定分类(自由文本，如世界观/力量体系/角色/地理/历史/物品)" validate:"required"`
	Content   string `json:"content" jsonschema:"required,description=设定内容" validate:"required"`
	IsGlobal  *bool  `json:"is_global,omitempty" jsonschema:"description=是否全局设定(对所有小说生效)；不传=新建默认false，更新保持原值"`
}

// UpsertSettingArgs 是 upsert_setting 的参数。
type UpsertSettingArgs struct {
	Settings []UpsertSettingItem `json:"settings" jsonschema:"required,description=要upsert的设定列表(1-5个，传setting_id=更新，不传=创建)" validate:"required,min=1,max=5,dive"`
}

// UpsertSettingTool 批量 upsert 世界观设定（创建或更新，单事务原子性）。
type UpsertSettingTool struct{}

func (t *UpsertSettingTool) Name() string { return "upsert_setting" }
func (t *UpsertSettingTool) Description() string {
	return "批量创建或更新小说设定（1-5个，单事务原子性，失败全部回滚）。" +
		"传 setting_id=更新该条（PATCH 语义，只覆盖传入字段），不传=新建。" +
		"开局已全量注入设定到上下文(带 [setting_id:N | 分类] 前缀)，更新时从注入里取 setting_id 传入。" +
		"新增相似分类的设定前，应优先更新已有条目(在原文基础上合并)而非创建重复条目。"
}
func (t *UpsertSettingTool) Category() ToolCategory { return CategoryWritingAssistant }

func (t *UpsertSettingTool) JSONSchema() json.RawMessage { return SchemaOf(UpsertSettingArgs{}) }
func (t *UpsertSettingTool) ExposeToLLM() bool           { return true }
func (t *UpsertSettingTool) NewArgs() any                { return &UpsertSettingArgs{} }

func (t *UpsertSettingTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*UpsertSettingArgs)

	type failure struct {
		index    int
		category string
		err      error
	}
	var failed *failure
	var ids []int64

	err := tc.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for i, item := range a.Settings {
			id, err := upsertOneSetting(tx, tc.NovelID, item)
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
			return nil, fmt.Errorf("upsert setting: %w", err)
		}
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("第 %d 条设定 [%s] 失败: %s", failed.index, failed.category, failed.err),
			Data:    map[string]any{"failed_index": failed.index, "failed_category": failed.category},
		}, nil
	}

	return &ToolResult{
		Success: true,
		Data:    map[string]any{"setting_ids": ids, "count": len(ids)},
	}, nil
}

// upsertOneSetting 在事务内 upsert 单条设定，返回设定 ID。
func upsertOneSetting(tx *gorm.DB, novelID int64, item UpsertSettingItem) (int64, error) {
	// 更新分支
	if item.SettingID != nil {
		var existing setting.SettingItem
		if err := tx.First(&existing, *item.SettingID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return 0, fmt.Errorf("设定条目 %d 不存在", *item.SettingID)
			}
			return 0, fmt.Errorf("query setting: %w", err)
		}
		// 归属校验：只能改全局设定或当前小说的设定
		if !existing.IsGlobal && existing.NovelID != novelID {
			return 0, fmt.Errorf("设定条目 %d 不属于当前小说", *item.SettingID)
		}
		// PATCH 覆盖（指针字段判断"传没传"，值字段 required 总传值）
		if item.Category != "" {
			existing.Category = item.Category
		}
		if item.Content != "" {
			existing.Content = item.Content
		}
		if item.IsGlobal != nil {
			existing.IsGlobal = *item.IsGlobal
			// is_global 切换时 NovelID 联动调整（rawArgs patch 表达不了的联动，手动补）
			if *item.IsGlobal {
				existing.NovelID = 0
			} else {
				existing.NovelID = novelID
			}
		}
		if err := tx.Save(&existing).Error; err != nil {
			return 0, fmt.Errorf("save setting: %w", err)
		}
		return existing.ID, nil
	}

	// 创建分支
	rec := setting.SettingItem{
		Category: item.Category,
		Content:  item.Content,
		NovelID:  novelID,
	}
	if item.IsGlobal != nil && *item.IsGlobal {
		rec.IsGlobal = true
		rec.NovelID = 0
	}
	if err := tx.Create(&rec).Error; err != nil {
		return 0, fmt.Errorf("create setting: %w", err)
	}
	return rec.ID, nil
}

// ── 注册 ──────────────────────────────────────────────

// RegisterSettingTools 注册设定工具。
func RegisterSettingTools(r *Registry) {
	r.Register(&UpsertSettingTool{})
}

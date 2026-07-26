package app

import (
	"fmt"

	"github.com/sigpanic/goink/internal/setting"
)

// ── 小说设定 ──────────────────────────────────────────────

// SettingResult 是 GetNovelSettings 的返回结构。
// v2 取消全局设定概念后，设定全部归属当前小说，不再分 Global/Novel 两组。
// 保留 Items 字段（取代原 Global/Novel），前端可直接遍历。
type SettingResult struct {
	Items      []setting.SettingItem `json:"items"`
	TokenCount int                   `json:"token_count"` // 全量设定 token 数（不截断），前端可显示是否超预算
	OverBudget bool                  `json:"over_budget"` // token_count > SettingsTokenBudget
}

// GetNovelSettings 返回当前小说的全部设定，附带全量 token 数与超预算标记。
// token_count 用于前端显示是否超 8k 软上限，不阻塞、不强制精简（用户自行决定）。
// 注入给 agent 的 NovelState 会按 created_at DESC 截断到 8k 内，与此处 token_count 是两套口径：
// 这里是"全量实际多少"，agent 注入是"截断后剩多少"。
func (a *App) GetNovelSettings(novelID int64) (*SettingResult, error) {
	items, err := a.setting.ListSettings(a.ctx, novelID)
	if err != nil {
		return nil, err
	}
	tokenCount, _ := setting.CountSettingsTokens(items)
	return &SettingResult{
		Items:      items,
		TokenCount: tokenCount,
		OverBudget: tokenCount > setting.SettingsTokenBudget,
	}, nil
}

// CreateNovelSettingInput 是创建设定的入参。
// v2 取消 is_global 字段，设定全部归属当前小说。
type CreateNovelSettingInput struct {
	Category string `json:"category"`
	Content  string `json:"content"`
}

// CreateNovelSetting 创建一条小说设定。
func (a *App) CreateNovelSetting(novelID int64, input CreateNovelSettingInput) (*setting.SettingItem, error) {
	item := setting.SettingItem{
		NovelID:  novelID,
		Category: input.Category,
		Content:  input.Content,
	}
	if err := a.setting.DB.WithContext(a.ctx).Create(&item).Error; err != nil {
		return nil, fmt.Errorf("create setting: %w", err)
	}
	return &item, nil
}

// UpdateNovelSettingInput 是更新设定的入参。
// 采用 PUT 语义：前端全量传，后端全量覆盖。
// v2 取消 is_global 后不再需要 is_global 切换的 NovelID 联动，可直接用 PatchAndSave，
// 但为了与 UpdatePreference 风格一致（db.Save 走 GORM 回调），保留手动 First + Save 模式。
type UpdateNovelSettingInput struct {
	Category string `json:"category"`
	Content  string `json:"content"`
}

// UpdateNovelSetting 更新一条小说设定。
// PUT 语义：前端全量传 category/content，后端全量覆盖 + db.Save（走 GORM 回调）。
// 归属校验：只能改当前小说的设定。
func (a *App) UpdateNovelSetting(novelID int64, id int64, input UpdateNovelSettingInput) (*setting.SettingItem, error) {
	var item setting.SettingItem
	if err := a.setting.DB.WithContext(a.ctx).First(&item, id).Error; err != nil {
		return nil, fmt.Errorf("update setting: %w", err)
	}
	// 归属校验：只能改当前小说的设定
	if item.NovelID != novelID {
		return nil, fmt.Errorf("update setting: 无权修改其他小说的设定")
	}
	// PUT 全量覆盖
	item.Category = input.Category
	item.Content = input.Content
	if err := a.setting.DB.WithContext(a.ctx).Save(&item).Error; err != nil {
		return nil, fmt.Errorf("update setting: %w", err)
	}
	return &item, nil
}

// DeleteNovelSetting 删除一条小说设定。
func (a *App) DeleteNovelSetting(id int64) error {
	if err := a.setting.DB.WithContext(a.ctx).Delete(&setting.SettingItem{}, id).Error; err != nil {
		return fmt.Errorf("delete setting: %w", err)
	}
	return nil
}

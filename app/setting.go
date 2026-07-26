package app

import (
	"fmt"

	"github.com/sigpanic/goink/internal/setting"
)

// ── 小说设定 ──────────────────────────────────────────────

// SettingResult 是 GetNovelSettings 的返回结构。
type SettingResult struct {
	Global     []setting.SettingItem `json:"global"`
	Novel      []setting.SettingItem `json:"novel"`
	TokenCount int                   `json:"token_count"` // 全量设定 token 数（不截断），前端可显示是否超预算
	OverBudget bool                  `json:"over_budget"` // token_count > SettingsTokenBudget
}

// GetNovelSettings 返回全局设定和当前小说的专属设定，附带全量 token 数与超预算标记。
// token_count 用于前端显示是否超 8k 软上限，不阻塞、不强制精简（用户自行决定）。
// 注入给 agent 的 NovelProfile 会按 created_at DESC 截断到 8k 内，与此处 token_count 是两套口径：
// 这里是"全量实际多少"，agent 注入是"截断后剩多少"。
func (a *App) GetNovelSettings(novelID int64) (*SettingResult, error) {
	global, err := a.setting.ListGlobalSettings(a.ctx)
	if err != nil {
		return nil, err
	}
	novelSettings, err := a.setting.ListNovelSettings(a.ctx, novelID)
	if err != nil {
		return nil, err
	}
	all := make([]setting.SettingItem, 0, len(global)+len(novelSettings))
	all = append(all, global...)
	all = append(all, novelSettings...)
	tokenCount, _ := setting.CountSettingsTokens(all)
	return &SettingResult{
		Global:     global,
		Novel:      novelSettings,
		TokenCount: tokenCount,
		OverBudget: tokenCount > setting.SettingsTokenBudget,
	}, nil
}

// CreateNovelSettingInput 是创建设定的入参。
type CreateNovelSettingInput struct {
	IsGlobal bool   `json:"is_global"`
	Category string `json:"category"`
	Content  string `json:"content"`
}

// CreateNovelSetting 创建一条小说设定。
func (a *App) CreateNovelSetting(novelID int64, input CreateNovelSettingInput) (*setting.SettingItem, error) {
	item := setting.SettingItem{
		IsGlobal: input.IsGlobal,
		Category: input.Category,
		Content:  input.Content,
	}
	// is_global=true 时 NovelID 置 0（全局设定不归属任何小说），否则归属当前小说
	if input.IsGlobal {
		item.NovelID = 0
	} else {
		item.NovelID = novelID
	}
	if err := a.setting.DB.WithContext(a.ctx).Create(&item).Error; err != nil {
		return nil, fmt.Errorf("create setting: %w", err)
	}
	return &item, nil
}

// UpdateNovelSettingInput 是更新设定的入参。
// 采用 PUT 语义：前端全量传，后端全量覆盖。
// 不走 PatchAndSave（设定可能全局 NovelID=0 + is_global 切换需特殊处理 NovelID）。
type UpdateNovelSettingInput struct {
	Category string `json:"category"`
	Content  string `json:"content"`
	IsGlobal bool   `json:"is_global"`
}

// UpdateNovelSetting 更新一条小说设定。
// novelID 由前端显式传入（当前工作区小说），用于 is_global 切换时填 NovelID。
// PUT 语义：前端全量传 category/content/is_global，后端全量覆盖 + db.Save（走 GORM 回调）。
func (a *App) UpdateNovelSetting(novelID int64, id int64, input UpdateNovelSettingInput) (*setting.SettingItem, error) {
	var item setting.SettingItem
	if err := a.setting.DB.WithContext(a.ctx).First(&item, id).Error; err != nil {
		return nil, fmt.Errorf("update setting: %w", err)
	}
	// 归属校验：只能改全局设定或当前小说的设定
	if !item.IsGlobal && item.NovelID != novelID {
		return nil, fmt.Errorf("update setting: 无权修改其他小说的设定")
	}
	// PUT 全量覆盖
	item.Category = input.Category
	item.Content = input.Content
	item.IsGlobal = input.IsGlobal
	if input.IsGlobal {
		item.NovelID = 0
	} else {
		item.NovelID = novelID
	}
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

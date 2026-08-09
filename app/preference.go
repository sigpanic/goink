package app

import (
	"fmt"

	"github.com/sigpanic/goink/internal/preference"
	"github.com/sigpanic/goink/internal/storage"
)

// ── 创作偏好 ──────────────────────────────────────────────

// PreferenceResult 是 GetPreferences 的返回结构。
type PreferenceResult struct {
	Global     []preference.PreferenceItem `json:"global"`
	Novel      []preference.PreferenceItem `json:"novel"`
	TokenCount int                         `json:"token_count"` // 全量偏好 token 数（不截断），前端可显示是否超预算
	OverBudget bool                        `json:"over_budget"` // token_count > PreferencesTokenBudget
}

// GetPreferences 返回全局偏好和当前小说的专属偏好，附带全量 token 数与超预算标记。
// token_count 用于前端显示是否超 4k 软上限，不阻塞、不强制精简（用户自行决定）。
// 注入给 agent 的 NovelProfile 会按 created_at DESC 截断到 4k 内，与此处 token_count 是两套口径：
// 这里是"全量实际多少"，agent 注入是"截断后剩多少"。
func (a *App) GetPreferences(novelID int64) (*PreferenceResult, error) {
	// 4b: 改调 ListGlobal/ListNovel 显式传 Order 保持原 created_at ASC 排序，Size=-1 全量拉取。
	globalResult, err := a.preference.ListGlobalPreferences(a.ctx, preference.ListOptions{
		PageParams: storage.PageParams{Size: -1},
		Order:      "created_at ASC",
	})
	if err != nil {
		return nil, err
	}
	novelResult, err := a.preference.ListNovelPreferences(a.ctx, novelID, preference.ListOptions{
		PageParams: storage.PageParams{Size: -1},
		Order:      "created_at ASC",
	})
	if err != nil {
		return nil, err
	}
	global := globalResult.Items
	novelPrefs := novelResult.Items
	all := make([]preference.PreferenceItem, 0, len(global)+len(novelPrefs))
	all = append(all, global...)
	all = append(all, novelPrefs...)
	tokenCount, _ := preference.CountPreferencesTokens(all)
	return &PreferenceResult{
		Global:     global,
		Novel:      novelPrefs,
		TokenCount: tokenCount,
		OverBudget: tokenCount > preference.PreferencesTokenBudget,
	}, nil
}

// CreatePreferenceInput 是创建偏好的入参。
type CreatePreferenceInput struct {
	IsGlobal bool   `json:"is_global"`
	Category string `json:"category"`
	Content  string `json:"content"`
}

// CreatePreference 创建一条创作偏好。
func (a *App) CreatePreference(novelID int64, input CreatePreferenceInput) (*preference.PreferenceItem, error) {
	item := preference.PreferenceItem{
		IsGlobal: input.IsGlobal,
		Category: input.Category,
		Content:  input.Content,
	}
	// is_global=true 时 NovelID 置 0（全局偏好不归属任何小说），否则归属当前小说
	if input.IsGlobal {
		item.NovelID = 0
	} else {
		item.NovelID = novelID
	}
	if err := a.preference.DB.WithContext(a.ctx).Create(&item).Error; err != nil {
		return nil, fmt.Errorf("create preference: %w", err)
	}
	return &item, nil
}

// UpdatePreferenceInput 是更新偏好的入参。
// 采用 PUT 语义：前端全量传，后端全量覆盖。
// 不走 PatchAndSave（偏好可能全局 NovelID=0 + is_global 切换需特殊处理 NovelID）。
type UpdatePreferenceInput struct {
	Category string `json:"category"`
	Content  string `json:"content"`
	IsGlobal bool   `json:"is_global"`
}

// UpdatePreference 更新一条创作偏好。
// novelID 由前端显式传入（当前工作区小说），用于 is_global 切换时填 NovelID。
// PUT 语义：前端全量传 category/content/is_global，后端全量覆盖 + db.Save（走 GORM 回调）。
func (a *App) UpdatePreference(novelID int64, id int64, input UpdatePreferenceInput) (*preference.PreferenceItem, error) {
	var item preference.PreferenceItem
	if err := a.preference.DB.WithContext(a.ctx).First(&item, id).Error; err != nil {
		return nil, fmt.Errorf("update preference: %w", err)
	}
	// 归属校验：只能改全局偏好或当前小说的偏好
	if !item.IsGlobal && item.NovelID != novelID {
		return nil, fmt.Errorf("update preference: 无权修改其他小说的偏好")
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
	if err := a.preference.DB.WithContext(a.ctx).Save(&item).Error; err != nil {
		return nil, fmt.Errorf("update preference: %w", err)
	}
	return &item, nil
}

// DeletePreference 删除一条创作偏好。
func (a *App) DeletePreference(id int64) error {
	if err := a.preference.DB.WithContext(a.ctx).Delete(&preference.PreferenceItem{}, id).Error; err != nil {
		return fmt.Errorf("delete preference: %w", err)
	}
	return nil
}

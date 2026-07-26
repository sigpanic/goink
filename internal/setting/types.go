package setting

import "time"

// SettingItem 是小说世界观/设定条目（in-fiction 事实）。
// 设定本质是小说级（v2 取消全局设定概念）：跨小说复用的"通用力量体系"等不合理，
// 用户想要通用规则应放偏好（procedural）。
// 与 PreferenceItem 区分：Setting 装小说世界内事实（修仙等级、主角武器），Preference 装创作规则（用短句）。
type SettingItem struct {
	ID        int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	NovelID   int64     `gorm:"column:novel_id;not null;index"     json:"novel_id"` // 必填，归属小说
	Category  string    `gorm:"column:category"                   json:"category"`  // 自由文本：世界观/力量体系/角色/地理/历史/物品
	Content   string    `gorm:"column:content;not null"           json:"content"`   // 自由文本，不强行结构化
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"  json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime"  json:"updated_at"`
}

// TableName 指定 GORM 表名。
func (SettingItem) TableName() string { return "setting_items" }

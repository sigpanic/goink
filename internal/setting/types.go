package setting

import "time"

// SettingItem 是小说世界观/设定条目（in-fiction 事实）。
// IsGlobal=true 表示全局设定（对所有小说生效，如通用力量体系），IsGlobal=false 表示特定小说的设定。
// 与 PreferenceItem 区分：Setting 装小说世界内事实（修仙等级、主角武器），Preference 装创作规则（用短句）。
type SettingItem struct {
	ID        int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	NovelID   int64     `gorm:"column:novel_id;index"             json:"novel_id"`  // IsGlobal=true 时为 0
	IsGlobal  bool      `gorm:"column:is_global;not null;index"   json:"is_global"` // true=全局，false=特定小说
	Category  string    `gorm:"column:category"                   json:"category"`  // 自由文本：世界观/力量体系/角色/地理/历史/物品
	Content   string    `gorm:"column:content;not null"           json:"content"`   // 自由文本，不强行结构化
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"  json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime"  json:"updated_at"`
}

// TableName 指定 GORM 表名。
func (SettingItem) TableName() string { return "setting_items" }

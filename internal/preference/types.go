package preference

import "time"

// PreferenceItem 是创作偏好条目。
// IsGlobal=true 表示用户级偏好（对所有小说生效），IsGlobal=false 表示特定小说的偏好。
type PreferenceItem struct {
	ID        int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	NovelID   int64     `gorm:"column:novel_id;index"             json:"novel_id"`  // IsGlobal=true 时无意义
	IsGlobal  bool      `gorm:"column:is_global;not null;index"   json:"is_global"` // true=用户全局，false=特定小说
	Category  string    `gorm:"column:category"                   json:"category"`  // LLM 自行归类，自由文本
	Content   string    `gorm:"column:content;not null"           json:"content"`   // 偏好内容
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"  json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime"  json:"updated_at"` // 最近活动时间，用于排序（注入截断保留活跃）
}

// TableName 指定 GORM 表名。
func (PreferenceItem) TableName() string { return "preference_items" }

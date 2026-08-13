package config

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

// AppSettings 是全局 app_config 表的单行配置。
// 适合存放更新频率低，重要的，其他可前端localstorage。
// 增删配置项直接在此 struct 加减字段即可，GORM 自动迁移。
type AppSettings struct {
	ID                uint      `gorm:"column:id;primaryKey;default:1"`
	LastNovelID       int64     `gorm:"column:last_novel_id;default:0"       json:"last_novel_id"`
	SelectedModelKey  string    `gorm:"column:selected_model_key;default:''"  json:"selected_model_key"`
	ReasoningEffort   string    `gorm:"column:reasoning_effort;default:''"    json:"reasoning_effort"`
	ApprovalMode      string    `gorm:"column:approval_mode;default:manual"   json:"approval_mode"`
	LastSessionID     string    `gorm:"column:last_session_id;default:''"     json:"last_session_id"`
	UserName          string    `gorm:"column:user_name;default:''"            json:"user_name"`
	GitName           string    `gorm:"column:git_name;default:'Goink'"         json:"git_name"`
	GitEmail          string    `gorm:"column:git_email;default:'goink@local'"  json:"git_email"`
	DismissedVersion  string    `gorm:"column:dismissed_version;default:''"     json:"dismissed_version"`     // 用户已忽略的更新版本号
	LastUpdateCheckAt time.Time `gorm:"column:last_update_check_at"              json:"last_update_check_at"` // 上次自动更新检查时间，用于 12h 节流；零值表示从未检查
}

func (AppSettings) TableName() string { return "app_config" }

// LoadSettings 从全局库读取配置（单行）。首次使用时自动创建空行。
func LoadSettings(db *gorm.DB) (*AppSettings, error) {
	// 确保有且只有一行（id=1）
	if err := db.FirstOrCreate(&AppSettings{}, AppSettings{ID: 1}).Error; err != nil {
		return nil, fmt.Errorf("读取应用配置失败: %w", err)
	}

	var s AppSettings
	if err := db.First(&s, 1).Error; err != nil {
		return nil, fmt.Errorf("读取应用配置失败: %w", err)
	}
	return &s, nil
}

// SaveSettings 将配置写回全局库。
func SaveSettings(db *gorm.DB, s *AppSettings) error {
	s.ID = 1 // 确保只更新同一行
	if err := db.Save(s).Error; err != nil {
		return fmt.Errorf("保存应用配置失败: %w", err)
	}
	return nil
}

package app

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/location"
	"github.com/sigpanic/goink/internal/storage"
)

// GetLocations 返回指定小说的全部地点，供前端侧边栏嵌套树和关系图节点渲染。
// 4b: 改调 ListByNovel(Size=-1) 全量（废弃 ListAllByNovel），传 Order="name ASC"
// 保持原 name 升序行为。
func (a *App) GetLocations(novelID int64) ([]location.Location, error) {
	result, err := a.location.ListByNovel(a.ctx, novelID, location.ListByNovelOptions{
		PageParams: storage.PageParams{Size: -1},
		Order:      "name ASC",
	})
	if err != nil {
		return nil, err
	}
	return result.Items, nil
}

// GetLocationRelations 返回指定小说的全部空间关系（无向边），供前端关系图渲染。
func (a *App) GetLocationRelations(novelID int64) ([]location.LocationRelation, error) {
	return a.location.ListRelationsByNovel(a.ctx, novelID)
}

// ── Location CRUD ───────────────────────────────────────

// CreateLocationInput 是 CreateLocation 的参数。
type CreateLocationInput struct {
	Name             string `json:"name"`                         // 地点名称，必填
	LocationType     string `json:"location_type,omitempty"`      // 自由文本类型
	Description      string `json:"description,omitempty"`        // 自然语言描述
	DetailJSON       string `json:"detail_json,omitempty"`        // JSON 自由格式
	ParentLocationID *int64 `json:"parent_location_id,omitempty"` // 父级地点 ID
	Tags             string `json:"tags,omitempty"`               // JSON 数组标签
}

// CreateLocation 创建一个地点。
func (a *App) CreateLocation(novelID int64, input CreateLocationInput) (*location.Location, error) {
	if input.Name == "" {
		return nil, fmt.Errorf("地点名称不能为空")
	}
	loc := location.Location{
		NovelID:          novelID,
		Name:             input.Name,
		LocationType:     input.LocationType,
		Description:      input.Description,
		DetailJSON:       input.DetailJSON,
		ParentLocationID: input.ParentLocationID,
		Tags:             input.Tags,
	}
	if err := a.location.DB.WithContext(a.ctx).Create(&loc).Error; err != nil {
		return nil, fmt.Errorf("create location: %w", err)
	}
	return &loc, nil
}

// UpdateLocationInput 是 UpdateLocation 的参数。
// 所有字段均为 optional，PATCH 只传要改的字段即可；传完整对象也行。
type UpdateLocationInput struct {
	Name             string `json:"name,omitempty"`
	LocationType     string `json:"location_type,omitempty"`
	Description      string `json:"description,omitempty"`
	DetailJSON       string `json:"detail_json,omitempty"`
	ParentLocationID *int64 `json:"parent_location_id,omitempty"`
	Tags             string `json:"tags,omitempty"`
	ClearParent      bool   `json:"clear_parent,omitempty" gorm:"-"`
}

// UpdateLocation 更新地点。只更新非零值字段。
// 用手动 First+if+Save 而非 PatchAndSave，因为 ClearParent 需要特殊处理。
func (a *App) UpdateLocation(novelID int64, locID int64, input UpdateLocationInput) error {
	var loc location.Location
	if err := a.location.DB.WithContext(a.ctx).
		Where("id = ? AND novel_id = ?", locID, novelID).First(&loc).Error; err != nil {
		return fmt.Errorf("update location: %w", err)
	}
	if input.Name != "" {
		loc.Name = input.Name
	}
	if input.LocationType != "" {
		loc.LocationType = input.LocationType
	}
	if input.Description != "" {
		loc.Description = input.Description
	}
	if input.DetailJSON != "" {
		loc.DetailJSON = input.DetailJSON
	}
	if input.Tags != "" {
		loc.Tags = input.Tags
	}
	if input.ClearParent {
		loc.ParentLocationID = nil
	} else if input.ParentLocationID != nil {
		loc.ParentLocationID = input.ParentLocationID
	}
	if err := a.location.DB.WithContext(a.ctx).Save(&loc).Error; err != nil {
		return fmt.Errorf("update location: %w", err)
	}
	return nil
}

// DeleteLocation 删除地点（子地点父级置空，级联删除空间关系）。
func (a *App) DeleteLocation(novelID int64, locID int64) error {
	return a.location.DB.WithContext(a.ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&location.Location{}).
			Where("parent_location_id = ? AND novel_id = ?", locID, novelID).
			Update("parent_location_id", nil).Error; err != nil {
			return fmt.Errorf("reparent children: %w", err)
		}
		if err := tx.Where("(location_a = ? OR location_b = ?) AND novel_id = ?", locID, locID, novelID).
			Delete(&location.LocationRelation{}).Error; err != nil {
			return fmt.Errorf("delete location relations: %w", err)
		}
		if err := tx.Where("id = ? AND novel_id = ?", locID, novelID).
			Delete(&location.Location{}).Error; err != nil {
			return fmt.Errorf("delete location: %w", err)
		}
		return nil
	})
}

package app

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/activity"
	"github.com/sigpanic/goink/internal/git"
	"github.com/sigpanic/goink/internal/volume"
)

// PlaceVolume 创建或移动卷，并将其置于锚点卷前或小说末尾。
type PlaceVolumeInput = volume.PlaceInput

func (a *App) PlaceVolume(input PlaceVolumeInput) (*volume.Volume, error) {
	v, err := a.volume.Place(a.ctx, nil, input)
	if err != nil {
		return nil, fmt.Errorf("place volume: %w", err)
	}
	if input.SourceVolumeID == nil && a.activity != nil {
		a.activity.AddActivity(a.ctx, activity.ActivityDelta{VolumesCreated: 1})
	}
	return v, nil
}

// UpdateVolume 重命名指定小说中的卷。
func (a *App) UpdateVolume(novelID, volumeID int64, name string) error {
	if err := a.volume.Update(a.ctx, nil, novelID, volumeID, name); err != nil {
		return fmt.Errorf("update volume: %w", err)
	}
	return nil
}

// DeleteVolume 删除指定小说中的空卷。
func (a *App) DeleteVolume(novelID, volumeID int64) error {
	return a.db.WithContext(a.ctx).Transaction(func(tx *gorm.DB) error {
		if err := a.volume.Delete(a.ctx, tx, novelID, volumeID); err != nil {
			return fmt.Errorf("delete volume: %w", err)
		}
		if err := git.RemoveFile(novelID, git.VolumePath(volumeID)); err != nil {
			return fmt.Errorf("卷已删除，但清理卷纲失败: %w", err)
		}
		return nil
	})
}

// GetVolumes 返回指定小说的全部卷，按阅读顺序排列。
func (a *App) GetVolumes(novelID int64) ([]volume.Volume, error) {
	volumes, err := a.volume.ListByNovel(a.ctx, nil, novelID)
	if err != nil {
		return nil, fmt.Errorf("list volumes: %w", err)
	}
	return volumes, nil
}

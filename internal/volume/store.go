package volume

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/git"
)

// ErrNotFound 卷不存在，或不属于该小说。
var ErrNotFound = errors.New("卷不存在（或不属于当前小说）")

// ErrNameTaken 同一小说内卷名重复。
var ErrNameTaken = errors.New("卷名已存在")

// ErrHasChapters 卷下仍有章节，不可删除。
var ErrHasChapters = errors.New("卷下仍有章节，请先移动或删除这些章节")

// chapterTable 是章节表名。用字面量而非 import chapter 包，
// 避免 volume → chapter 依赖成环（chapter 侧将来要调用本包分配 sort_order）。
const chapterTable = "chapters"

// Store 管理 Volume 持久化。
//
// 事务约定：每个方法都接受一个可选的 tx 参数——
//   - 事务外调用传 nil，方法用 Store 自己的连接
//   - 事务内调用必须传 tx，否则会去抢本应用唯一的连接（SetMaxOpenConns(1)）而死锁
type Store struct {
	DB     *gorm.DB
	logger *slog.Logger
}

// NewStore 创建 volume 存储。
func NewStore(db *gorm.DB, logger *slog.Logger) *Store {
	return &Store{DB: db, logger: logger}
}

// pick 返回本次操作用的连接：tx 非 nil 用 tx，否则用 Store 自己的 db。
func (s *Store) pick(tx *gorm.DB) *gorm.DB {
	if tx != nil {
		return tx
	}
	return s.DB
}

// Place 创建或移动卷，并将其放到锚点卷前或小说末尾。
// 最终顺序会在同一事务内以两阶段写入落库，避免卷顺序唯一索引的中间态冲突。
func (s *Store) Place(ctx context.Context, tx *gorm.DB, input PlaceInput) (*Volume, error) {
	if input.SourceVolumeID == nil && input.Name == nil {
		return nil, fmt.Errorf("创建卷时必须提供名称")
	}
	if input.SourceVolumeID != nil && input.Name != nil {
		return nil, fmt.Errorf("移动卷时不能提供名称")
	}

	db := s.pick(tx)
	var placed Volume
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		volumes, err := s.ListByNovel(ctx, tx, input.NovelID)
		if err != nil {
			return err
		}

		var source *Volume
		if input.SourceVolumeID != nil {
			for i := range volumes {
				if volumes[i].ID == *input.SourceVolumeID {
					source = &volumes[i]
					break
				}
			}
			if source == nil {
				return fmt.Errorf("%w: %d", ErrNotFound, *input.SourceVolumeID)
			}
		} else {
			taken, err := s.nameTaken(ctx, tx, input.NovelID, *input.Name, 0)
			if err != nil {
				return err
			}
			if taken {
				return ErrNameTaken
			}

			maxSortOrder := 0
			for _, v := range volumes {
				if v.SortOrder > maxSortOrder {
					maxSortOrder = v.SortOrder
				}
			}
			created := Volume{NovelID: input.NovelID, Name: *input.Name, SortOrder: maxSortOrder + 1}
			if err := tx.WithContext(ctx).Create(&created).Error; err != nil {
				return fmt.Errorf("volume store: create: %w", err)
			}
			volumes = append(volumes, created)
			source = &volumes[len(volumes)-1]
		}

		if input.BeforeVolumeID != nil && source.ID == *input.BeforeVolumeID {
			placed = *source
			return nil
		}

		orderedIDs := make([]int64, 0, len(volumes))
		anchorFound := input.BeforeVolumeID == nil
		for _, v := range volumes {
			if v.ID == source.ID {
				continue
			}
			if input.BeforeVolumeID != nil && v.ID == *input.BeforeVolumeID {
				orderedIDs = append(orderedIDs, source.ID)
				anchorFound = true
			}
			orderedIDs = append(orderedIDs, v.ID)
		}
		if !anchorFound {
			return fmt.Errorf("%w: %d", ErrNotFound, *input.BeforeVolumeID)
		}
		if input.BeforeVolumeID == nil {
			orderedIDs = append(orderedIDs, source.ID)
		}
		if err := s.writeOrder(ctx, tx, orderedIDs); err != nil {
			return err
		}
		for i, id := range orderedIDs {
			if id == source.ID {
				placed = *source
				placed.SortOrder = i + 1
				break
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	placed.OutlineFilePath = git.VolumePath(placed.ID)
	return &placed, nil
}

// Update 重命名卷。名称未变时视为无操作。
func (s *Store) Update(ctx context.Context, tx *gorm.DB, novelID, volumeID int64, name string) error {
	db := s.pick(tx)
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		v, err := s.GetByID(ctx, tx, novelID, volumeID)
		if err != nil {
			return err
		}
		if v.Name == name {
			return nil
		}
		taken, err := s.nameTaken(ctx, tx, novelID, name, volumeID)
		if err != nil {
			return err
		}
		if taken {
			return ErrNameTaken
		}
		if err := tx.WithContext(ctx).Model(&Volume{}).
			Where("id = ?", volumeID).Update("name", name).Error; err != nil {
			return fmt.Errorf("volume store: update: %w", err)
		}
		return nil
	})
}

// Delete 删除卷。卷下仍有章节时返回 ErrHasChapters。
func (s *Store) Delete(ctx context.Context, tx *gorm.DB, novelID, volumeID int64) error {
	db := s.pick(tx)
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := s.GetByID(ctx, tx, novelID, volumeID); err != nil {
			return err
		}

		var chapters int64
		if err := tx.WithContext(ctx).Table(chapterTable).
			Where("volume_id = ?", volumeID).Count(&chapters).Error; err != nil {
			return fmt.Errorf("volume store: count chapters: %w", err)
		}
		if chapters > 0 {
			return ErrHasChapters
		}

		if err := tx.WithContext(ctx).Delete(&Volume{}, volumeID).Error; err != nil {
			return fmt.Errorf("volume store: delete: %w", err)
		}
		return nil
	})
}

// GetByID 按 id 取卷，并校验归属该小说。不存在返回 ErrNotFound。
func (s *Store) GetByID(ctx context.Context, tx *gorm.DB, novelID, volumeID int64) (*Volume, error) {
	var v Volume
	err := s.pick(tx).WithContext(ctx).
		Where("id = ? AND novel_id = ?", volumeID, novelID).
		First(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("%w: %d", ErrNotFound, volumeID)
	}
	if err != nil {
		return nil, fmt.Errorf("volume store: get: %w", err)
	}
	v.OutlineFilePath = git.VolumePath(v.ID)
	return &v, nil
}

// ListByNovel 返回该小说全部卷，按 sort_order 升序。
func (s *Store) ListByNovel(ctx context.Context, tx *gorm.DB, novelID int64) ([]Volume, error) {
	var volumes []Volume
	if err := s.pick(tx).WithContext(ctx).
		Where("novel_id = ?", novelID).
		Order("sort_order ASC").
		Find(&volumes).Error; err != nil {
		return nil, fmt.Errorf("volume store: list: %w", err)
	}
	for i := range volumes {
		volumes[i].OutlineFilePath = git.VolumePath(volumes[i].ID)
	}
	return volumes, nil
}

// LastByNovel 返回 sort_order 最大的卷（最后一卷），无卷返回 (nil, nil)。
func (s *Store) LastByNovel(ctx context.Context, tx *gorm.DB, novelID int64) (*Volume, error) {
	var v Volume
	err := s.pick(tx).WithContext(ctx).
		Where("novel_id = ?", novelID).
		Order("sort_order DESC").
		First(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("volume store: last: %w", err)
	}
	v.OutlineFilePath = git.VolumePath(v.ID)
	return &v, nil
}

// writeOrder 按传入的完整卷顺序更新 sort_order（1-based）。
// 调用方负责在当前事务中验证全部 ID 的归属与唯一性。
// 两阶段写入：先把全部目标卷挪到负数区，再写目标值，避免唯一索引冲突。
func (s *Store) writeOrder(ctx context.Context, tx *gorm.DB, volumeIDs []int64) error {
	for i, id := range volumeIDs {
		if err := tx.WithContext(ctx).Model(&Volume{}).
			Where("id = ?", id).
			Update("sort_order", -(i + 1)).Error; err != nil {
			return fmt.Errorf("volume store: reorder stage1: %w", err)
		}
	}
	for i, id := range volumeIDs {
		if err := tx.WithContext(ctx).Model(&Volume{}).
			Where("id = ?", id).
			Update("sort_order", i+1).Error; err != nil {
			return fmt.Errorf("volume store: reorder stage2: %w", err)
		}
	}
	return nil
}

// AllocateChapterSortOrder 为「新建章节」分配所属分组内的 sort_order，返回末尾位置。
//
// 分组规则：volumeID 非 nil 时为对应卷；nil 时为未分卷组。
// 新建章节总是追加到目标分组末尾，不影响其他卷或未分卷组的 sort_order。
// 事务内必须传 tx。卷不存在时返回 ErrNotFound。
func (s *Store) AllocateChapterSortOrder(ctx context.Context, tx *gorm.DB, novelID int64, volumeID *int64) (int, error) {
	db := s.pick(tx)
	q := db.WithContext(ctx).Table(chapterTable).Where("novel_id = ?", novelID)
	if volumeID != nil {
		if _, err := s.GetByID(ctx, tx, novelID, *volumeID); err != nil {
			return 0, err
		}
		q = q.Where("volume_id = ?", *volumeID)
	} else {
		q = q.Where("volume_id IS NULL")
	}

	var maxSort int
	if err := q.Select("COALESCE(MAX(sort_order), 0)").Scan(&maxSort).Error; err != nil {
		return 0, fmt.Errorf("volume store: max sort_order in group: %w", err)
	}
	return maxSort + 1, nil
}

// nameTaken 判断该小说内卷名是否已被占用（excludeID 用于重命名时排除自身）。
func (s *Store) nameTaken(ctx context.Context, tx *gorm.DB, novelID int64, name string, excludeID int64) (bool, error) {
	q := tx.WithContext(ctx).Model(&Volume{}).Where("novel_id = ? AND name = ?", novelID, name)
	if excludeID != 0 {
		q = q.Where("id <> ?", excludeID)
	}
	var n int64
	if err := q.Count(&n).Error; err != nil {
		return false, fmt.Errorf("volume store: check name: %w", err)
	}
	return n > 0, nil
}

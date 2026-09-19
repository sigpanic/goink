package volume

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"gorm.io/gorm"
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

// Create 新建卷，sort_order 取该小说当前最大值 +1（追加到末尾）。
func (s *Store) Create(ctx context.Context, tx *gorm.DB, novelID int64, name string) (*Volume, error) {
	db := s.pick(tx)
	var created *Volume
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		taken, err := s.nameTaken(ctx, tx, novelID, name, 0)
		if err != nil {
			return err
		}
		if taken {
			return ErrNameTaken
		}

		var maxSort int
		if err := tx.WithContext(ctx).Model(&Volume{}).
			Select("COALESCE(MAX(sort_order), 0)").
			Where("novel_id = ?", novelID).
			Scan(&maxSort).Error; err != nil {
			return fmt.Errorf("volume store: max sort_order: %w", err)
		}

		v := &Volume{NovelID: novelID, Name: name, SortOrder: maxSort + 1}
		if err := tx.WithContext(ctx).Create(v).Error; err != nil {
			return fmt.Errorf("volume store: create: %w", err)
		}
		created = v
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
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
	return &v, nil
}

// Reorder 按传入顺序重排全部卷的 sort_order（1-based）。
//
// volumeIDs 必须是该小说全部卷的 id 集合（全量重排），缺漏或混入他卷都报错。
// 两阶段写入：先把全部目标卷挪到负数区，再写目标值——
// 否则逐个改写会与 (novel_id, sort_order) 唯一索引冲突。
func (s *Store) Reorder(ctx context.Context, tx *gorm.DB, novelID int64, volumeIDs []int64) error {
	db := s.pick(tx)
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		existing, err := s.ListByNovel(ctx, tx, novelID)
		if err != nil {
			return err
		}
		if len(existing) != len(volumeIDs) {
			return fmt.Errorf("volume store: reorder 需传入全部卷（现有 %d，传入 %d）",
				len(existing), len(volumeIDs))
		}
		valid := make(map[int64]bool, len(existing))
		for _, v := range existing {
			valid[v.ID] = true
		}
		for _, id := range volumeIDs {
			if !valid[id] {
				return fmt.Errorf("volume store: reorder 传入的卷 %d 不属于该小说", id)
			}
		}

		// 阶段 1：全部挪到负数区，腾空正数区
		for i, id := range volumeIDs {
			if err := tx.WithContext(ctx).Model(&Volume{}).
				Where("id = ?", id).
				Update("sort_order", -(i + 1)).Error; err != nil {
				return fmt.Errorf("volume store: reorder stage1: %w", err)
			}
		}
		// 阶段 2：写入目标顺序
		for i, id := range volumeIDs {
			if err := tx.WithContext(ctx).Model(&Volume{}).
				Where("id = ?", id).
				Update("sort_order", i+1).Error; err != nil {
				return fmt.Errorf("volume store: reorder stage2: %w", err)
			}
		}
		return nil
	})
}

// AllocateChapterSortOrder 为「新建章节」分配 sort_order，返回插入位置 pos。
//
// 分配规则（全局阅读序 = ORDER BY sort_order）：
//   - 目标卷有章节：卷内 max(sort_order)+1，并把 pos 及之后的章节批量 +1 腾位
//   - 目标卷为空：取目标卷之前各卷（按 volumes.sort_order）章节的 max+1
//   - 上面仍为 0（全书还没有卷章节）：退化为全局 max+1，追加到末尾
//   - volumeID 为 0（未分卷）：全局 max+1，追加到全书末尾
//
// 事务内必须传 tx。卷不存在时返回 ErrNotFound。
func (s *Store) AllocateChapterSortOrder(ctx context.Context, tx *gorm.DB, novelID, volumeID int64) (int, error) {
	db := s.pick(tx)
	anchor := 0
	if volumeID != 0 {
		v, err := s.GetByID(ctx, tx, novelID, volumeID)
		if err != nil {
			return 0, err
		}
		if err := db.WithContext(ctx).Table(chapterTable).
			Select("COALESCE(MAX(sort_order), 0)").
			Where("novel_id = ? AND volume_id = ?", novelID, v.ID).
			Scan(&anchor).Error; err != nil {
			return 0, fmt.Errorf("volume store: max sort_order in volume: %w", err)
		}
		if anchor == 0 {
			// 空卷：取该卷之前各卷章节的最大 sort_order
			if err := db.WithContext(ctx).Raw(
				`SELECT COALESCE(MAX(c.sort_order), 0) FROM chapters c
				 JOIN volumes v ON v.id = c.volume_id
				 WHERE c.novel_id = ? AND v.novel_id = ? AND v.sort_order < ?`,
				novelID, novelID, v.SortOrder).
				Scan(&anchor).Error; err != nil {
				return 0, fmt.Errorf("volume store: max sort_order before volume: %w", err)
			}
		}
	}
	if anchor == 0 {
		// 未分卷新建，或前面没有卷章节：追加到全书末尾
		if err := db.WithContext(ctx).Table(chapterTable).
			Select("COALESCE(MAX(sort_order), 0)").
			Where("novel_id = ?", novelID).
			Scan(&anchor).Error; err != nil {
			return 0, fmt.Errorf("volume store: max sort_order: %w", err)
		}
	}

	pos := anchor + 1
	// 腾位：插入点及之后的章节整体 +1（追加到全书末尾时影响 0 行）
	if err := db.WithContext(ctx).Table(chapterTable).
		Where("novel_id = ? AND sort_order >= ?", novelID, pos).
		Update("sort_order", gorm.Expr("sort_order + 1")).Error; err != nil {
		return 0, fmt.Errorf("volume store: shift sort_order: %w", err)
	}
	return pos, nil
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

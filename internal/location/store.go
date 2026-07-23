package location

import (
	"context"
	"fmt"
	"log/slog"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/sigpanic/goink/internal/storage"
)

// Store 管理 Location 和 LocationRelation 持久化。DB 导出供调用方做简单 CRUD。
type Store struct {
	DB     *gorm.DB
	logger *slog.Logger
}

// NewStore 创建 location 存储。
func NewStore(db *gorm.DB, logger *slog.Logger) *Store {
	return &Store{DB: db, logger: logger}
}

// ── Location ─────────────────────────────────────────

// ListByNovelOptions 是 ListByNovel 的可选参数。
type ListByNovelOptions struct {
	PageParams   storage.PageParams
	LocationType string // 空字符串=不过滤
	Search       string // 空字符串=不过滤，按 name LIKE 模糊匹配
}

// ListByNovel 分页列出某小说的地点，支持类型过滤和名称搜索。
func (s *Store) ListByNovel(ctx context.Context, novelID int64, opts ListByNovelOptions) (*storage.PageResult[Location], error) {
	pp := opts.PageParams
	pp.Normalize()

	q := s.DB.WithContext(ctx).Model(&Location{}).Where("novel_id = ?", novelID)

	if opts.LocationType != "" {
		q = q.Where("location_type = ?", opts.LocationType)
	}
	if opts.Search != "" {
		q = q.Where("name LIKE ?", "%"+opts.Search+"%")
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("location store: count: %w", err)
	}

	var locs []Location
	offset := (pp.Page - 1) * pp.Size
	if err := q.Order("name ASC").Offset(offset).Limit(pp.Size).Find(&locs).Error; err != nil {
		return nil, fmt.Errorf("location store: list: %w", err)
	}

	s.logger.Debug("location store: listed", "novel_id", novelID, "total", total, "page", pp.Page)
	return storage.NewPageResult(locs, total, pp.Page, pp.Size), nil
}

// ListAllByNovel 返回某小说的全部地点（不分页），供前端侧边栏嵌套树和关系图渲染。
func (s *Store) ListAllByNovel(ctx context.Context, novelID int64) ([]Location, error) {
	var locs []Location
	if err := s.DB.WithContext(ctx).Where("novel_id = ?", novelID).Order("name ASC").Find(&locs).Error; err != nil {
		return nil, fmt.Errorf("location store: list all: %w", err)
	}
	s.logger.Debug("location store: list all", "novel_id", novelID, "count", len(locs))
	return locs, nil
}

// GetChildren 返回某地点的直接子地点。
func (s *Store) GetChildren(ctx context.Context, parentID int64) ([]Location, error) {
	var children []Location
	if err := s.DB.WithContext(ctx).
		Where("parent_location_id = ?", parentID).
		Order("name ASC").
		Find(&children).Error; err != nil {
		return nil, fmt.Errorf("location store: children: %w", err)
	}
	return children, nil
}

// GetByIDs 批量按 ID 取地点，用于图查询时解析名称。
func (s *Store) GetByIDs(ctx context.Context, ids []int64) ([]Location, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var locs []Location
	if err := s.DB.WithContext(ctx).Where("id IN ?", ids).Find(&locs).Error; err != nil {
		return nil, fmt.Errorf("location store: get by ids: %w", err)
	}
	return locs, nil
}

// ── LocationRelation ──────────────────────────────────

// ListRelationsByNovel 返回某小说全部空间关系边。
func (s *Store) ListRelationsByNovel(ctx context.Context, novelID int64) ([]LocationRelation, error) {
	var rels []LocationRelation
	if err := s.DB.WithContext(ctx).
		Where("novel_id = ?", novelID).
		Order("relation_type ASC").
		Find(&rels).Error; err != nil {
		return nil, fmt.Errorf("location store: list relations: %w", err)
	}
	return rels, nil
}

// ListRelationsByLocation 返回涉及某地点的所有无向边（location_a=X 或 location_b=X）。
func (s *Store) ListRelationsByLocation(ctx context.Context, locID int64) ([]LocationRelation, error) {
	var rels []LocationRelation
	if err := s.DB.WithContext(ctx).
		Where("location_a = ? OR location_b = ?", locID, locID).
		Order("relation_type ASC").
		Find(&rels).Error; err != nil {
		return nil, fmt.Errorf("location store: list relations by location: %w", err)
	}
	return rels, nil
}

// ListRelationsInvolving 返回涉及任意给定地点 ID 的全部边。
func (s *Store) ListRelationsInvolving(ctx context.Context, locIDs []int64) ([]LocationRelation, error) {
	if len(locIDs) == 0 {
		return nil, nil
	}
	var rels []LocationRelation
	if err := s.DB.WithContext(ctx).
		Where("location_a IN ? OR location_b IN ?", locIDs, locIDs).
		Order("relation_type ASC").
		Find(&rels).Error; err != nil {
		return nil, fmt.Errorf("location store: list relations involving: %w", err)
	}
	return rels, nil
}

// UpsertRelation 插入或更新空间关系边。(location_a, location_b) 唯一约束下 ON CONFLICT DO UPDATE。
func (s *Store) UpsertRelation(ctx context.Context, rel *LocationRelation) error {
	if err := s.DB.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "location_a"}, {Name: "location_b"}},
			DoUpdates: clause.AssignmentColumns([]string{"relation_type", "description", "updated_at"}),
		}).
		Create(rel).Error; err != nil {
		return fmt.Errorf("location store: upsert relation: %w", err)
	}
	return nil
}

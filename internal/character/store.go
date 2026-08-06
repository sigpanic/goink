package character

import (
	"context"
	"fmt"
	"log/slog"

	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/storage"
)

// Store 管理 Character 和 CharacterRelation 持久化。DB 导出供调用方做简单 CRUD。
type Store struct {
	DB     *gorm.DB
	logger *slog.Logger
}

// NewStore 创建 character 存储。
func NewStore(db *gorm.DB, logger *slog.Logger) *Store {
	return &Store{DB: db, logger: logger}
}

// ── Character ────────────────────────────────────────

// ListByNovelOptions 是 ListByNovel 的可选参数。
type ListByNovelOptions struct {
	PageParams storage.PageParams
	Search     string // 空字符串=不过滤，按 name LIKE 模糊匹配
}

// ListByNovel 分页列出某小说的角色，支持 name 搜索。
func (s *Store) ListByNovel(ctx context.Context, novelID int64, opts ListByNovelOptions) (*storage.PageResult[Character], error) {
	pp := opts.PageParams
	pp.Normalize()

	q := s.DB.WithContext(ctx).Model(&Character{}).Where("novel_id = ?", novelID)

	if opts.Search != "" {
		q = q.Where("name LIKE ?", "%"+opts.Search+"%")
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("character store: count: %w", err)
	}

	var chars []Character
	if err := q.Order("updated_at DESC").Offset(pp.Offset()).Limit(pp.Size).Find(&chars).Error; err != nil {
		return nil, fmt.Errorf("character store: list: %w", err)
	}

	s.logger.Debug("character store: listed", "novel_id", novelID, "total", total, "page", pp.Page)
	return storage.NewPageResult(chars, total, pp.Page, pp.Size), nil
}

// ListAllByNovel 返回某小说的全部角色（不分页），供前端侧边栏和关系图渲染。
func (s *Store) ListAllByNovel(ctx context.Context, novelID int64) ([]Character, error) {
	var chars []Character
	if err := s.DB.WithContext(ctx).Where("novel_id = ?", novelID).Order("name ASC").Find(&chars).Error; err != nil {
		return nil, fmt.Errorf("character store: list all: %w", err)
	}
	s.logger.Debug("character store: list all", "novel_id", novelID, "count", len(chars))
	return chars, nil
}

// GetByIDs 批量按 ID 取角色，用于关系查询时解析角色名。
func (s *Store) GetByIDs(ctx context.Context, ids []int64) ([]Character, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var chars []Character
	if err := s.DB.WithContext(ctx).Where("id IN ?", ids).Find(&chars).Error; err != nil {
		return nil, fmt.Errorf("character store: get by ids: %w", err)
	}
	return chars, nil
}

// ── CharacterRelation ─────────────────────────────────

// ListCurrentByNovel 返回某小说全部当前有效关系（is_current=true）。
// 前端关系图渲染用，数据量大时不建议直接给 LLM。
func (s *Store) ListCurrentByNovel(ctx context.Context, novelID int64) ([]CharacterRelation, error) {
	var rels []CharacterRelation
	if err := s.DB.WithContext(ctx).
		Where("novel_id = ? AND is_current = ?", novelID, true).
		Find(&rels).Error; err != nil {
		return nil, fmt.Errorf("character store: list current relations: %w", err)
	}
	return rels, nil
}

// ListByCharacter 返回某角色所有当前关系（is_current=true），不限方向。
func (s *Store) ListByCharacter(ctx context.Context, characterID int64) ([]CharacterRelation, error) {
	var rels []CharacterRelation
	if err := s.DB.WithContext(ctx).
		Where("(source_character_id = ? OR target_character_id = ?) AND is_current = ?", characterID, characterID, true).
		Find(&rels).Error; err != nil {
		return nil, fmt.Errorf("character store: list by character: %w", err)
	}
	return rels, nil
}

// ListByCharacters 批量按角色 ID 取当前关系，用于 LLM 按需查询指定角色群的关系网。
func (s *Store) ListByCharacters(ctx context.Context, characterIDs []int64) ([]CharacterRelation, error) {
	if len(characterIDs) == 0 {
		return nil, nil
	}
	var rels []CharacterRelation
	if err := s.DB.WithContext(ctx).
		Where("is_current = ? AND (source_character_id IN ? OR target_character_id IN ?)", true, characterIDs, characterIDs).
		Find(&rels).Error; err != nil {
		return nil, fmt.Errorf("character store: list by characters: %w", err)
	}
	return rels, nil
}

// GetHistory 返回两角色间全部关系记录（含历史），按时间升序。
// 不考虑方向——A→B 或 B→A 都算。
func (s *Store) GetHistory(ctx context.Context, charA, charB int64) ([]CharacterRelation, error) {
	var rels []CharacterRelation
	if err := s.DB.WithContext(ctx).
		Where("(source_character_id = ? AND target_character_id = ?) OR (source_character_id = ? AND target_character_id = ?)",
			charA, charB, charB, charA).
		Order("created_at ASC").
		Find(&rels).Error; err != nil {
		return nil, fmt.Errorf("character store: get history: %w", err)
	}
	return rels, nil
}

// ListBetweenCharacters 返回给定角色集合内部的当前关系边（两端都在集合内）。
func (s *Store) ListBetweenCharacters(ctx context.Context, characterIDs []int64) ([]CharacterRelation, error) {
	if len(characterIDs) == 0 {
		return nil, nil
	}
	var rels []CharacterRelation
	if err := s.DB.WithContext(ctx).
		Where("is_current = ? AND source_character_id IN ? AND target_character_id IN ?", true, characterIDs, characterIDs).
		Find(&rels).Error; err != nil {
		return nil, fmt.Errorf("character store: list between characters: %w", err)
	}
	return rels, nil
}

// Deactivate 将一条关系标记为非当前（is_current=false）。
func (s *Store) Deactivate(ctx context.Context, relationID int64) error {
	res := s.DB.WithContext(ctx).
		Model(&CharacterRelation{}).
		Where("id = ?", relationID).
		Update("is_current", false)
	if res.Error != nil {
		return fmt.Errorf("character store: deactivate: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("character store: deactivate: %w", gorm.ErrRecordNotFound)
	}
	return nil
}

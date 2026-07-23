package app

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/character"
	"github.com/sigpanic/goink/internal/storage"
)

// GetCharacters 返回指定小说的全部角色，供前端侧边栏列表和关系图节点渲染。
func (a *App) GetCharacters(novelID int64) ([]character.Character, error) {
	return a.character.ListAllByNovel(a.ctx, novelID)
}

// GetCharacterRelations 返回指定小说的全部当前角色关系（有向边），供前端关系图渲染。
func (a *App) GetCharacterRelations(novelID int64) ([]character.CharacterRelation, error) {
	return a.character.ListCurrentByNovel(a.ctx, novelID)
}

// ── Character CRUD ──────────────────────────────────────

// CreateCharacterInput 是 CreateCharacter 的参数。
type CreateCharacterInput struct {
	Name        string `json:"name"`                  // 角色名称，必填
	Description string `json:"description,omitempty"` // 自然语言描述
	Personality string `json:"personality,omitempty"` // JSON 自由格式，如 {"traits":["勇敢","冲动"]}
	Abilities   string `json:"abilities,omitempty"`   // JSON 数组，如 ["剑术","隐身"]
}

// CreateCharacter 创建一个角色。
func (a *App) CreateCharacter(novelID int64, input CreateCharacterInput) (*character.Character, error) {
	if input.Name == "" {
		return nil, fmt.Errorf("角色名称不能为空")
	}
	char := character.Character{
		NovelID:     novelID,
		Name:        input.Name,
		Description: input.Description,
		Personality: input.Personality,
		Abilities:   input.Abilities,
	}
	if err := a.character.DB.WithContext(a.ctx).Create(&char).Error; err != nil {
		return nil, fmt.Errorf("create character: %w", err)
	}
	return &char, nil
}

// UpdateCharacterInput 是 UpdateCharacter 的参数。
// 所有字段均为 optional，PATCH 只传要改的字段即可；传完整对象也行。
type UpdateCharacterInput struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Personality string `json:"personality,omitempty"`
	Abilities   string `json:"abilities,omitempty"`
}

// UpdateCharacter 更新角色。只更新非零值字段。
func (a *App) UpdateCharacter(novelID int64, charID int64, input UpdateCharacterInput) error {
	var ch character.Character
	if err := storage.PatchAndSave(a.character.DB.WithContext(a.ctx), charID, novelID, &input, &ch); err != nil {
		return fmt.Errorf("update character: %w", err)
	}
	return nil
}

// DeleteCharacter 删除角色（级联删除关联的关系记录）。
func (a *App) DeleteCharacter(novelID int64, charID int64) error {
	return a.character.DB.WithContext(a.ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("(source_character_id = ? OR target_character_id = ?) AND novel_id = ?", charID, charID, novelID).
			Delete(&character.CharacterRelation{}).Error; err != nil {
			return fmt.Errorf("delete character relations: %w", err)
		}
		if err := tx.Where("id = ? AND novel_id = ?", charID, novelID).
			Delete(&character.Character{}).Error; err != nil {
			return fmt.Errorf("delete character: %w", err)
		}
		return nil
	})
}

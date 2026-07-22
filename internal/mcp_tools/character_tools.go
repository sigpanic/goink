package mcp_tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"gorm.io/gorm"

	"novel/internal/character"
	"novel/internal/storage"
)

// ── get_characters ────────────────────────────────────

// GetCharactersArgs 是 get_characters 的参数。
type GetCharactersArgs struct {
	Search   string `json:"search" jsonschema:"description=角色名模糊搜索"`
	PageArgs        // 嵌入分页参数
}

// GetCharactersTool 获取角色列表，截断 50 条，冷角色靠搜索。
type GetCharactersTool struct{}

func (t *GetCharactersTool) Name() string { return "get_characters" }
func (t *GetCharactersTool) Description() string {
	return "获取当前小说的角色列表。按最近更新降序排列，截断 50 条——完整了解角色阵容后再创作。" +
		"冷门角色（长时间未更新）可能不在列表中，用 search 按名搜索。" +
		"需要了解角色之间的关系时，用 get_character_relations 传入角色 ID 获取子图。"
}
func (t *GetCharactersTool) Category() ToolCategory { return CategoryNovelManagement }

func (t *GetCharactersTool) JSONSchema() json.RawMessage { return SchemaOf(GetCharactersArgs{}) }
func (t *GetCharactersTool) ExposeToLLM() bool           { return true }
func (t *GetCharactersTool) NewArgs() any                { return &GetCharactersArgs{} }

func (t *GetCharactersTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*GetCharactersArgs)
	a.NormalizePage()

	store := character.NewStore(tc.DB, slog.Default())
	result, err := store.ListByNovel(ctx, tc.NovelID, character.ListByNovelOptions{
		PageParams: storage.PageParams{Page: a.Page, Size: a.Size},
		Search:     a.Search,
	})
	if err != nil {
		return nil, fmt.Errorf("list characters: %w", err)
	}

	items := make([]map[string]any, len(result.Items))
	for i, ch := range result.Items {
		items[i] = map[string]any{
			"id":          ch.ID,
			"name":        ch.Name,
			"description": ch.Description,
			"personality": parseJSONField(ch.Personality),
			"abilities":   parseJSONField(ch.Abilities),
		}
	}

	data := PageMeta(result)
	data["characters"] = items

	return &ToolResult{Success: true, Data: data}, nil
}

// ── get_character_relations ───────────────────────────

// GetCharacterRelationsArgs 是 get_character_relations 的参数。
type GetCharacterRelationsArgs struct {
	CharacterIDs []int64 `json:"character_ids" jsonschema:"required,description=角色ID列表，只返回这些角色之间的关系边" validate:"required,min=1"`
}

// GetCharacterRelationsTool 返回给定角色集合内部的子图边。
type GetCharacterRelationsTool struct{}

func (t *GetCharacterRelationsTool) Name() string { return "get_character_relations" }
func (t *GetCharacterRelationsTool) Description() string {
	return "获取指定角色之间的关系边（子图）。只返回两端都在 character_ids 中的关系，不限方向。" +
		"通常先通过 get_characters 获取角色列表，然后传入你关心的角色 ID 查询它们之间的关系。"
}
func (t *GetCharacterRelationsTool) Category() ToolCategory { return CategoryMemoryRetrieval }

func (t *GetCharacterRelationsTool) JSONSchema() json.RawMessage {
	return SchemaOf(GetCharacterRelationsArgs{})
}
func (t *GetCharacterRelationsTool) ExposeToLLM() bool { return true }
func (t *GetCharacterRelationsTool) NewArgs() any      { return &GetCharacterRelationsArgs{} }

func (t *GetCharacterRelationsTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*GetCharacterRelationsArgs)

	store := character.NewStore(tc.DB, slog.Default())

	rels, err := store.ListBetweenCharacters(ctx, a.CharacterIDs)
	if err != nil {
		return nil, fmt.Errorf("get character relations: %w", err)
	}

	// 解析角色名
	chars, err := store.GetByIDs(ctx, a.CharacterIDs)
	if err != nil {
		return nil, fmt.Errorf("get character names: %w", err)
	}
	nameMap := make(map[int64]string, len(chars))
	for _, ch := range chars {
		nameMap[ch.ID] = ch.Name
	}

	formatted := formatRelationEdges(rels, nameMap)

	return &ToolResult{
		Success: true,
		Data:    map[string]any{"content": formatted},
	}, nil
}

func formatRelationEdges(rels []character.CharacterRelation, nameMap map[int64]string) string {
	if len(rels) == 0 {
		return "暂无关系数据。"
	}

	// 按源角色分组为邻接表
	groups := make(map[int64][]character.CharacterRelation)
	var order []int64
	for _, rel := range rels {
		if _, ok := groups[rel.SourceCharacterID]; !ok {
			order = append(order, rel.SourceCharacterID)
		}
		groups[rel.SourceCharacterID] = append(groups[rel.SourceCharacterID], rel)
	}

	lines := []string{"### 角色关系"}
	for _, srcID := range order {
		srcName := nameMap[srcID]
		var edges []string
		for _, rel := range groups[srcID] {
			edge := fmt.Sprintf("→ %s：%s [relation_id:%d]",
				nameMap[rel.TargetCharacterID], rel.RelationDescribe, rel.ID)
			if rel.Description != "" {
				edge += fmt.Sprintf("（%s）", rel.Description)
			}
			edges = append(edges, edge)
		}
		lines = append(lines, fmt.Sprintf("- %s %s", srcName, strings.Join(edges, "、")))
	}
	return strings.Join(lines, "\n")
}

// ── create_character ──────────────────────────────────

// CreateCharacterItem 是 create_character 的单条参数。
type CreateCharacterItem struct {
	Name        string `json:"name"        jsonschema:"required,description=角色名称"               validate:"required"`
	Description string `json:"description"  jsonschema:"description=角色自然语言描述，如外貌、身份、背景故事等"`
	Personality string `json:"personality" jsonschema:"description=自由JSON对象，描述角色性格/定位/背景等，如{\"traits\":[\"勇敢\"]，\"brief\":\"热血青年\"}"`
	Abilities   string `json:"abilities"   jsonschema:"description=JSON数组，角色能力/技能列表，如[\"剑术\"，\"隐身\"]"`
}

// CreateCharacterArgs 是 create_character 的参数。
type CreateCharacterArgs struct {
	Characters []CreateCharacterItem `json:"characters" jsonschema:"required,description=要创建的角色列表（1-10个）" validate:"required,min=1,max=10,dive"`
}

// CreateCharacterTool 创建新角色。
type CreateCharacterTool struct{}

func (t *CreateCharacterTool) Name() string { return "create_character" }
func (t *CreateCharacterTool) Description() string {
	return "批量创建角色（1-10个）。保证原子性，失败时返回具体条目原因。" +
		"name 必填；personality 为自由 JSON，建议包含 role/traits/background/motivation；" +
		"abilities 为 JSON 数组。创建后可用 get_characters 查看。"
}
func (t *CreateCharacterTool) Category() ToolCategory { return CategoryNovelManagement }

func (t *CreateCharacterTool) JSONSchema() json.RawMessage { return SchemaOf(CreateCharacterArgs{}) }
func (t *CreateCharacterTool) ExposeToLLM() bool           { return true }
func (t *CreateCharacterTool) NewArgs() any                { return &CreateCharacterArgs{} }

func (t *CreateCharacterTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*CreateCharacterArgs)

	var ids []int64
	var failedName string
	var failedErr error
	err := tc.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, item := range a.Characters {
			ch := character.Character{
				NovelID:     tc.NovelID,
				Name:        item.Name,
				Description: item.Description,
				Personality: item.Personality,
				Abilities:   item.Abilities,
			}
			if err := tx.Create(&ch).Error; err != nil {
				failedName = item.Name
				failedErr = err
				return err
			}
			ids = append(ids, ch.ID)
		}
		return nil
	})
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("创建角色 [%s] 失败: %s", failedName, failedErr),
		}, nil
	}

	return &ToolResult{
		Success: true,
		Data:    map[string]any{"ids": ids, "count": len(ids)},
	}, nil
}

// ── update_character ──────────────────────────────────

// UpdateCharacterArgs 是 update_character 的参数。
type UpdateCharacterArgs struct {
	CharacterID int64  `json:"character_id" jsonschema:"required,description=角色ID"     validate:"required,min=1"`
	Name        string `json:"name"         jsonschema:"description=新的名称"`
	Description string `json:"description"  jsonschema:"description=新的自然语言描述（完全替换旧的）"`
	Personality string `json:"personality"  jsonschema:"description=新的性格/设定，字符串形式JSON（完全替换旧的）"`
	Abilities   string `json:"abilities"    jsonschema:"description=新的能力列表，字符串形式JSON（完全替换旧的）"`
}

// UpdateCharacterTool 更新角色字段。
type UpdateCharacterTool struct{}

func (t *UpdateCharacterTool) Name() string { return "update_character" }
func (t *UpdateCharacterTool) Description() string {
	return "更新已有角色的设定。只需传入要修改的字段，未传入的保持不变。" +
		"personality 和 abilities 会完全替换旧值，不是合并。"
}
func (t *UpdateCharacterTool) Category() ToolCategory { return CategoryNovelManagement }

func (t *UpdateCharacterTool) JSONSchema() json.RawMessage { return SchemaOf(UpdateCharacterArgs{}) }
func (t *UpdateCharacterTool) ExposeToLLM() bool           { return true }
func (t *UpdateCharacterTool) NewArgs() any                { return &UpdateCharacterArgs{} }

func (t *UpdateCharacterTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*UpdateCharacterArgs)

	if a.Name == "" && a.Description == "" && a.Personality == "" && a.Abilities == "" {
		return &ToolResult{Success: false, Error: "至少需要提供一个要修改的字段"}, nil
	}

	var ch character.Character
	if err := tc.DB.WithContext(ctx).Where("id = ? AND novel_id = ?", a.CharacterID, tc.NovelID).First(&ch).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &ToolResult{Success: false, Error: fmt.Sprintf("角色 %d 不存在", a.CharacterID)}, nil
		}
		return nil, fmt.Errorf("query character: %w", err)
	}

	if err := json.Unmarshal(tc.RawArgs, &ch); err != nil {
		return &ToolResult{Success: false, Error: "参数格式不正确: " + err.Error()}, nil
	}

	if err := tc.DB.WithContext(ctx).Save(&ch).Error; err != nil {
		return nil, fmt.Errorf("save character: %w", err)
	}

	return &ToolResult{
		Success: true,
		Data:    map[string]any{"id": ch.ID},
	}, nil
}

// ── update_character_relationship ─────────────────────

// UpdateCharacterRelationshipArgs 是 update_character_relationship 的参数。
type UpdateCharacterRelationshipArgs struct {
	RelationID        int64  `json:"relation_id"         jsonschema:"description=编辑已有关系时提供此ID，直接修改描述内容"`
	SourceCharacterID int64  `json:"source_character_id" jsonschema:"description=建立新关系时的发出方角色ID。旧关系自动变为历史"`
	TargetCharacterID int64  `json:"target_character_id" jsonschema:"description=建立新关系时的接收方角色ID。旧关系自动变为历史"`
	RelationDescribe  string `json:"relation_describe"   jsonschema:"description=自由文本描述关系，如'师徒、暗中较量'。详细描述而非简单分类词。编辑已有关系时可不传"`
	Description       string `json:"description"         jsonschema:"description=当前关系阶段的详细描述"`
	ChapterNumber     int    `json:"chapter_number"      jsonschema:"description=此关系确立/变化的章节号"`
}

// UpdateCharacterRelationshipTool 创建或更新角色关系。
type UpdateCharacterRelationshipTool struct{}

func (t *UpdateCharacterRelationshipTool) Name() string { return "update_character_relationship" }
func (t *UpdateCharacterRelationshipTool) Description() string {
	return "更新角色关系。两种用法：" +
		"1) 编辑已有关系——提供 relation_id，修改描述措辞；" +
		"2) 关系演变——提供 source_character_id + target_character_id，旧关系自动保留为历史，新关系设为当前。" +
		"relation_describe 用自然语言描述，如'师徒但暗中互相提防'，不要用简单枚举词。"
}
func (t *UpdateCharacterRelationshipTool) Category() ToolCategory { return CategoryWritingAssistant }

func (t *UpdateCharacterRelationshipTool) JSONSchema() json.RawMessage {
	return SchemaOf(UpdateCharacterRelationshipArgs{})
}
func (t *UpdateCharacterRelationshipTool) ExposeToLLM() bool { return true }
func (t *UpdateCharacterRelationshipTool) NewArgs() any      { return &UpdateCharacterRelationshipArgs{} }

func (t *UpdateCharacterRelationshipTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*UpdateCharacterRelationshipArgs)

	switch {
	case a.RelationID > 0 && a.SourceCharacterID == 0 && a.TargetCharacterID == 0:
		return t.editRelation(ctx, a, tc)
	case a.SourceCharacterID > 0 && a.TargetCharacterID > 0 && a.RelationID == 0:
		return t.evolveRelation(ctx, a, tc)
	default:
		return &ToolResult{
			Success: false,
			Error:   "需要提供 relation_id（编辑已有关系）或 source_character_id + target_character_id（建立新关系）",
		}, nil
	}
}

func (t *UpdateCharacterRelationshipTool) editRelation(ctx context.Context, a *UpdateCharacterRelationshipArgs, tc ToolContext) (*ToolResult, error) {
	var rel character.CharacterRelation
	if err := tc.DB.WithContext(ctx).First(&rel, a.RelationID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &ToolResult{Success: false, Error: fmt.Sprintf("关系 %d 不存在", a.RelationID)}, nil
		}
		return nil, fmt.Errorf("query relation: %w", err)
	}
	if rel.NovelID != tc.NovelID {
		return &ToolResult{Success: false, Error: fmt.Sprintf("关系 %d 不属于当前小说", a.RelationID)}, nil
	}

	if err := json.Unmarshal(tc.RawArgs, &rel); err != nil {
		return &ToolResult{Success: false, Error: "参数格式不正确: " + err.Error()}, nil
	}

	if err := tc.DB.WithContext(ctx).Save(&rel).Error; err != nil {
		return nil, fmt.Errorf("save relation: %w", err)
	}

	return &ToolResult{
		Success: true,
		Data:    map[string]any{"id": rel.ID, "action": "edit"},
	}, nil
}

func (t *UpdateCharacterRelationshipTool) evolveRelation(ctx context.Context, a *UpdateCharacterRelationshipArgs, tc ToolContext) (*ToolResult, error) {
	if a.SourceCharacterID == a.TargetCharacterID {
		return &ToolResult{Success: false, Error: "source 和 target 不能是同一个角色"}, nil
	}
	if a.RelationDescribe == "" {
		return &ToolResult{Success: false, Error: "演变关系时 relation_describe 为必填"}, nil
	}

	// 校验两个角色存在且属于当前小说
	store := character.NewStore(tc.DB, slog.Default())
	chars, err := store.GetByIDs(ctx, []int64{a.SourceCharacterID, a.TargetCharacterID})
	if err != nil {
		return nil, fmt.Errorf("query characters: %w", err)
	}
	found := make(map[int64]bool, len(chars))
	for _, ch := range chars {
		found[ch.ID] = true
	}
	if !found[a.SourceCharacterID] {
		return &ToolResult{Success: false, Error: fmt.Sprintf("角色 %d 不存在", a.SourceCharacterID)}, nil
	}
	if !found[a.TargetCharacterID] {
		return &ToolResult{Success: false, Error: fmt.Sprintf("角色 %d 不存在", a.TargetCharacterID)}, nil
	}

	var newRel character.CharacterRelation
	err = tc.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 将同向旧关系标记为非当前（append-only），逐行 Save 以触发操作日志回调
		var oldRels []character.CharacterRelation
		if err := tx.Where("source_character_id = ? AND target_character_id = ? AND is_current = ?",
			a.SourceCharacterID, a.TargetCharacterID, true).
			Find(&oldRels).Error; err != nil {
			return fmt.Errorf("query old relations: %w", err)
		}
		for i := range oldRels {
			oldRels[i].IsCurrent = false
			if err := tx.Save(&oldRels[i]).Error; err != nil {
				return fmt.Errorf("deactivate old relation: %w", err)
			}
		}

		newRel = character.CharacterRelation{
			NovelID:           tc.NovelID,
			SourceCharacterID: a.SourceCharacterID,
			TargetCharacterID: a.TargetCharacterID,
			RelationDescribe:  a.RelationDescribe,
			Description:       a.Description,
			ChapterNumber:     a.ChapterNumber,
			IsCurrent:         true,
		}
		if err := tx.Create(&newRel).Error; err != nil {
			return fmt.Errorf("create relation: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &ToolResult{
		Success: true,
		Data:    map[string]any{"id": newRel.ID, "action": "evolve"},
	}, nil
}

// ── 格式化工具 ──────────────────────────────────────────

func parseJSONField(raw string) any {
	if raw == "" {
		return nil
	}
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return raw
	}
	return v
}

// ── 注册 ──────────────────────────────────────────────

// RegisterCharacterTools 注册角色管理类工具。
func RegisterCharacterTools(r *Registry) {
	r.Register(&GetCharactersTool{})
	r.Register(&GetCharacterRelationsTool{})
	r.Register(&CreateCharacterTool{})
	r.Register(&UpdateCharacterTool{})
	r.Register(&UpdateCharacterRelationshipTool{})
}

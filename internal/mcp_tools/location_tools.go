package mcp_tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"gorm.io/gorm"

	"novel/internal/location"
	"novel/internal/storage"
)

// ── get_locations ──────────────────────────────────────

// GetLocationsArgs 是 get_locations 的参数。
type GetLocationsArgs struct {
	Mode         string `json:"mode" jsonschema:"required,description=查询模式：list=列表 detail=详情 network=全图,enum=list,enum=detail,enum=network,default=list" validate:"required,oneof=list detail network"`
	LocationID   int64  `json:"location_id" jsonschema:"description=地点ID（detail模式必填）"                                                                    validate:"omitempty,min=1"`
	LocationType string `json:"location_type" jsonschema:"description=按类型筛选（list模式可选）"`
	Search       string `json:"search" jsonschema:"description=按名称搜索（list模式可选）"`
	PageArgs            // 嵌入分页参数
}

// GetLocationsTool 获取地点信息，支持三种模式。
type GetLocationsTool struct{}

func (t *GetLocationsTool) Name() string { return "get_locations" }
func (t *GetLocationsTool) Description() string {
	return "获取当前小说的地点信息，支持三种模式：\n" +
		"- list：分页列表，支持按类型和名称搜索\n" +
		"- detail：地点详情，含子地点列表和内部连通关系\n" +
		"- network：大地图——只显示根节点之间的连通关系，用于宏观空间感知"
}
func (t *GetLocationsTool) Category() ToolCategory { return CategoryNovelManagement }

func (t *GetLocationsTool) JSONSchema() json.RawMessage { return SchemaOf(GetLocationsArgs{}) }
func (t *GetLocationsTool) ExposeToLLM() bool           { return true }
func (t *GetLocationsTool) NewArgs() any                { return &GetLocationsArgs{} }

func (t *GetLocationsTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*GetLocationsArgs)
	a.NormalizePage()

	store := location.NewStore(tc.DB, slog.Default())

	switch a.Mode {
	case "detail":
		return t.executeDetail(ctx, a, tc, store)
	case "network":
		return t.executeNetwork(ctx, tc, store)
	default:
		return t.executeList(ctx, a, tc, store)
	}
}

func (t *GetLocationsTool) executeList(ctx context.Context, a *GetLocationsArgs, tc ToolContext, store *location.Store) (*ToolResult, error) {
	result, err := store.ListByNovel(ctx, tc.NovelID, location.ListByNovelOptions{
		PageParams:   storage.PageParams{Page: a.Page, Size: a.Size},
		LocationType: a.LocationType,
		Search:       a.Search,
	})
	if err != nil {
		return nil, fmt.Errorf("list locations: %w", err)
	}

	items := make([]map[string]any, len(result.Items))
	for i, loc := range result.Items {
		desc := loc.Description
		if len([]rune(desc)) > 100 {
			desc = string([]rune(desc)[:100]) + "..."
		}
		items[i] = map[string]any{
			"id":            loc.ID,
			"name":          loc.Name,
			"location_type": loc.LocationType,
			"description":   desc,
			"tags":          parseJSONField(loc.Tags),
		}
	}

	data := PageMeta(result)
	data["locations"] = items

	return &ToolResult{Success: true, Data: data}, nil
}

func (t *GetLocationsTool) executeDetail(ctx context.Context, a *GetLocationsArgs, tc ToolContext, store *location.Store) (*ToolResult, error) {
	if a.LocationID == 0 {
		return &ToolResult{Success: false, Error: "detail 模式需要 location_id"}, nil
	}

	var loc location.Location
	if err := tc.DB.WithContext(ctx).Where("id = ? AND novel_id = ?", a.LocationID, tc.NovelID).First(&loc).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &ToolResult{Success: false, Error: fmt.Sprintf("地点 %d 不存在", a.LocationID)}, nil
		}
		return nil, fmt.Errorf("query location: %w", err)
	}

	// 父地点名
	var parentName string
	if loc.ParentLocationID != nil {
		var parent location.Location
		if err := tc.DB.WithContext(ctx).First(&parent, *loc.ParentLocationID).Error; err == nil {
			parentName = parent.Name
		}
	}

	// 子地点
	children, err := store.GetChildren(ctx, loc.ID)
	if err != nil {
		return nil, fmt.Errorf("query children: %w", err)
	}

	// 连通关系：涉及自身 + 所有子地点的边
	var rels []location.LocationRelation
	ids := []int64{loc.ID}
	for _, ch := range children {
		ids = append(ids, ch.ID)
	}
	rels, err = store.ListRelationsInvolving(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("query relations: %w", err)
	}

	// 解析名称：收集关系中涉及的所有地点 ID
	nameSet := make(map[int64]bool)
	for _, rel := range rels {
		nameSet[rel.LocationA] = true
		nameSet[rel.LocationB] = true
	}
	var relIDs []int64
	for id := range nameSet {
		relIDs = append(relIDs, id)
	}
	nameMap := resolveLocationNames(ctx, tc.DB, relIDs)

	formatted := formatLocationDetail(loc, parentName, children, rels, nameMap)

	return &ToolResult{
		Success: true,
		Data:    map[string]any{"content": formatted},
	}, nil
}

func (t *GetLocationsTool) executeNetwork(ctx context.Context, tc ToolContext, store *location.Store) (*ToolResult, error) {
	// 全部地点
	allResult, err := store.ListByNovel(ctx, tc.NovelID, location.ListByNovelOptions{
		PageParams: storage.PageParams{Page: 1, Size: 10000},
	})
	if err != nil {
		return nil, fmt.Errorf("list locations: %w", err)
	}
	allLocs := allResult.Items

	// 根节点
	var roots []location.Location
	childCounts := make(map[int64]int)
	for _, loc := range allLocs {
		if loc.ParentLocationID == nil {
			roots = append(roots, loc)
		} else {
			childCounts[*loc.ParentLocationID]++
		}
	}

	// 根节点 ID 集合
	rootIDs := make([]int64, len(roots))
	for i, r := range roots {
		rootIDs[i] = r.ID
	}

	// 涉及根节点的边
	allRels, err := store.ListRelationsInvolving(ctx, rootIDs)
	if err != nil {
		return nil, fmt.Errorf("query relations: %w", err)
	}

	// 只保留两端都是根节点的边
	rootSet := make(map[int64]bool, len(rootIDs))
	for _, id := range rootIDs {
		rootSet[id] = true
	}
	var rootRels []location.LocationRelation
	for _, rel := range allRels {
		if rootSet[rel.LocationA] && rootSet[rel.LocationB] {
			rootRels = append(rootRels, rel)
		}
	}

	formatted := formatLocationNetwork(roots, rootRels, childCounts)

	return &ToolResult{
		Success: true,
		Data:    map[string]any{"content": formatted},
	}, nil
}

// ── create_location ────────────────────────────────────

// CreateLocationItem 是 create_location 的单条参数。
type CreateLocationItem struct {
	Name             string `json:"name" jsonschema:"required,description=地点名称"             validate:"required"`
	LocationType     string `json:"location_type" jsonschema:"description=地点类型，自由文本，如'森林'、'城市'、'洞穴'"`
	Description      string `json:"description" jsonschema:"description=环境氛围、特色等描述"`
	DetailJSON       string `json:"detail_json" jsonschema:"description=字符串形式的JSON对象，结构化信息：气候、氛围、历史事件等"`
	Tags             string `json:"tags" jsonschema:"description=字符串形式的JSON数组，自由标签，如[\"危险\"，\"神秘\"]"`
	ParentLocationID *int64 `json:"parent_location_id" jsonschema:"description=父级地点ID，用于构建层级树"`
}

// CreateLocationArgs 是 create_location 的参数。
type CreateLocationArgs struct {
	Locations []CreateLocationItem `json:"locations" jsonschema:"required,description=要创建的地点列表（1-10个）" validate:"required,min=1,max=10,dive"`
}

// CreateLocationTool 创建新地点。
type CreateLocationTool struct{}

func (t *CreateLocationTool) Name() string { return "create_location" }
func (t *CreateLocationTool) Description() string {
	return "批量创建地点（1-10个）。保证原子性，失败时返回具体条目原因。" +
		"name 必填，location_type 自由文本。" +
		"parent_location_id 可接入已有层级树，如创建'大殿'时设为'王宫'的 ID。"
}
func (t *CreateLocationTool) Category() ToolCategory { return CategoryNovelManagement }

func (t *CreateLocationTool) JSONSchema() json.RawMessage { return SchemaOf(CreateLocationArgs{}) }
func (t *CreateLocationTool) ExposeToLLM() bool           { return true }
func (t *CreateLocationTool) NewArgs() any                { return &CreateLocationArgs{} }

func (t *CreateLocationTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*CreateLocationArgs)

	// 预校验：批量验证父地点存在性
	parentIDSet := make(map[int64]bool)
	for _, item := range a.Locations {
		if item.ParentLocationID != nil {
			parentIDSet[*item.ParentLocationID] = true
		}
	}
	if len(parentIDSet) > 0 {
		parentIDs := make([]int64, 0, len(parentIDSet))
		for id := range parentIDSet {
			parentIDs = append(parentIDs, id)
		}
		var count int64
		if err := tc.DB.WithContext(ctx).Model(&location.Location{}).
			Where("id IN ? AND novel_id = ?", parentIDs, tc.NovelID).
			Count(&count).Error; err != nil {
			return nil, fmt.Errorf("verify parent locations: %w", err)
		}
		if int(count) != len(parentIDs) {
			return &ToolResult{Success: false, Error: "父地点不存在或不属于当前小说"}, nil
		}
	}

	var ids []int64
	var failedName string
	var failedErr error
	err := tc.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, item := range a.Locations {
			loc := location.Location{
				NovelID:          tc.NovelID,
				Name:             item.Name,
				LocationType:     item.LocationType,
				Description:      item.Description,
				DetailJSON:       item.DetailJSON,
				Tags:             item.Tags,
				ParentLocationID: item.ParentLocationID,
			}
			if err := tx.Create(&loc).Error; err != nil {
				failedName = item.Name
				failedErr = err
				return err
			}
			ids = append(ids, loc.ID)
		}
		return nil
	})
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("创建地点 [%s] 失败: %s", failedName, failedErr),
		}, nil
	}

	return &ToolResult{
		Success: true,
		Data:    map[string]any{"ids": ids, "count": len(ids)},
	}, nil
}

// ── update_location ────────────────────────────────────

// UpdateLocationArgs 是 update_location 的参数。
type UpdateLocationArgs struct {
	LocationID       int64  `json:"location_id" jsonschema:"required,description=地点ID"               validate:"required,min=1"`
	Name             string `json:"name" jsonschema:"description=新的名称"`
	LocationType     string `json:"location_type" jsonschema:"description=新的类型"`
	Description      string `json:"description" jsonschema:"description=新的描述"`
	DetailJSON       string `json:"detail_json" jsonschema:"description=新的结构化信息，字符串形式JSON（完全替换旧的）"`
	Tags             string `json:"tags" jsonschema:"description=新的标签，字符串形式JSON数组（完全替换旧的）"`
	ParentLocationID *int64 `json:"parent_location_id" jsonschema:"description=新的父级地点ID（不传不变，传null变根节点）"`
}

// UpdateLocationTool 更新地点字段。
type UpdateLocationTool struct{}

func (t *UpdateLocationTool) Name() string { return "update_location" }
func (t *UpdateLocationTool) Description() string {
	return "更新已有地点的设定。只需传入要修改的字段，未传入的保持不变。" +
		"parent_location_id 传入 null 可将地点从树中移除变为根节点。"
}
func (t *UpdateLocationTool) Category() ToolCategory { return CategoryNovelManagement }

func (t *UpdateLocationTool) JSONSchema() json.RawMessage { return SchemaOf(UpdateLocationArgs{}) }
func (t *UpdateLocationTool) ExposeToLLM() bool           { return true }
func (t *UpdateLocationTool) NewArgs() any                { return &UpdateLocationArgs{} }

func (t *UpdateLocationTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*UpdateLocationArgs)

	// ParentLocationID 为 nil 有两种情况：LLM 没传 vs 传了 null。
	// 前者不应视为"有修改"，后者应（清除父节点）。检查 RawArgs 区分。
	var raw map[string]any
	json.Unmarshal(tc.RawArgs, &raw)
	_, hasParent := raw["parent_location_id"]

	if a.Name == "" && a.LocationType == "" && a.Description == "" && a.DetailJSON == "" && a.Tags == "" && !hasParent {
		return &ToolResult{Success: false, Error: "至少需要提供一个要修改的字段"}, nil
	}

	var loc location.Location
	if err := tc.DB.WithContext(ctx).Where("id = ? AND novel_id = ?", a.LocationID, tc.NovelID).First(&loc).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &ToolResult{Success: false, Error: fmt.Sprintf("地点 %d 不存在", a.LocationID)}, nil
		}
		return nil, fmt.Errorf("query location: %w", err)
	}

	// 校验父地点存在性
	if hasParent && a.ParentLocationID != nil {
		var parentCount int64
		if err := tc.DB.WithContext(ctx).Model(&location.Location{}).
			Where("id = ? AND novel_id = ?", *a.ParentLocationID, tc.NovelID).
			Count(&parentCount).Error; err != nil {
			return nil, fmt.Errorf("verify parent location: %w", err)
		}
		if parentCount == 0 {
			return &ToolResult{Success: false, Error: "父地点不存在或不属于当前小说"}, nil
		}
	}

	if err := json.Unmarshal(tc.RawArgs, &loc); err != nil {
		return &ToolResult{Success: false, Error: "参数格式不正确: " + err.Error()}, nil
	}

	if err := tc.DB.WithContext(ctx).Save(&loc).Error; err != nil {
		return nil, fmt.Errorf("save location: %w", err)
	}

	return &ToolResult{
		Success: true,
		Data:    map[string]any{"id": loc.ID},
	}, nil
}

// ── create_location_relation ────────────────────────────

// CreateLocationRelationItem 是 create_location_relation 的单条参数。
type CreateLocationRelationItem struct {
	LocationA    int64  `json:"location_a_id" jsonschema:"required,description=地点A的ID"               validate:"required,min=1"`
	LocationB    int64  `json:"location_b_id" jsonschema:"required,description=地点B的ID"               validate:"required,min=1"`
	RelationType string `json:"relation_type" jsonschema:"required,description=空间关系描述，如'相邻''由山路连通'" validate:"required"`
	Description  string `json:"description" jsonschema:"description=补充细节"`
}

// CreateLocationRelationArgs 是 create_location_relation 的参数。
type CreateLocationRelationArgs struct {
	Relations []CreateLocationRelationItem `json:"relations" jsonschema:"required,description=要创建的关系列表（1-10个）" validate:"required,min=1,max=10,dive"`
}

// CreateLocationRelationTool 创建地点之间的无向空间关系边。
type CreateLocationRelationTool struct{}

func (t *CreateLocationRelationTool) Name() string { return "create_location_relation" }
func (t *CreateLocationRelationTool) Description() string {
	return "批量创建地点间的空间连通关系（1-10个）。保证原子性，失败时返回具体条目原因。" +
		"关系为无向边（A-B 等价 B-A），已存在边时返回错误，需修改已有边请用 update_location_relation。"
}
func (t *CreateLocationRelationTool) Category() ToolCategory { return CategoryWritingAssistant }

func (t *CreateLocationRelationTool) JSONSchema() json.RawMessage {
	return SchemaOf(CreateLocationRelationArgs{})
}
func (t *CreateLocationRelationTool) ExposeToLLM() bool { return true }
func (t *CreateLocationRelationTool) NewArgs() any      { return &CreateLocationRelationArgs{} }

func (t *CreateLocationRelationTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*CreateLocationRelationArgs)

	// 预校验：自环检查 + 归一化
	for i := range a.Relations {
		if a.Relations[i].LocationA == a.Relations[i].LocationB {
			return &ToolResult{Success: false, Error: "不能创建地点与自身的空间关系"}, nil
		}
		if a.Relations[i].LocationA > a.Relations[i].LocationB {
			a.Relations[i].LocationA, a.Relations[i].LocationB = a.Relations[i].LocationB, a.Relations[i].LocationA
		}
	}

	// 预校验：收集所有涉及的地点 ID，批量校验存在性
	idSet := make(map[int64]bool)
	for _, item := range a.Relations {
		idSet[item.LocationA] = true
		idSet[item.LocationB] = true
	}
	allIDs := make([]int64, 0, len(idSet))
	for id := range idSet {
		allIDs = append(allIDs, id)
	}
	var count int64
	if err := tc.DB.WithContext(ctx).Model(&location.Location{}).
		Where("id IN ? AND novel_id = ?", allIDs, tc.NovelID).
		Count(&count).Error; err != nil {
		return nil, fmt.Errorf("verify locations: %w", err)
	}
	if int(count) != len(allIDs) {
		return &ToolResult{Success: false, Error: "部分地点不存在或不属于当前小说"}, nil
	}

	// 预校验：批量检查是否已存在关系边
	seen := make(map[string]bool)
	for _, item := range a.Relations {
		key := fmt.Sprintf("%d-%d", item.LocationA, item.LocationB)
		if seen[key] {
			return &ToolResult{Success: false, Error: fmt.Sprintf("参数中存在重复的关系：地点 %d 和 %d", item.LocationA, item.LocationB)}, nil
		}
		seen[key] = true
	}
	type pair struct{ a, b int64 }
	var pairs []pair
	for key := range seen {
		var aID, bID int64
		_, _ = fmt.Sscanf(key, "%d-%d", &aID, &bID)
		pairs = append(pairs, pair{aID, bID})
	}
	var existing []location.LocationRelation
	for _, p := range pairs {
		var rel location.LocationRelation
		err := tc.DB.WithContext(ctx).
			Where("location_a = ? AND location_b = ? AND novel_id = ?", p.a, p.b, tc.NovelID).
			First(&rel).Error
		if err == nil {
			existing = append(existing, rel)
		} else if err != gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("check existing relation: %w", err)
		}
	}
	if len(existing) > 0 {
		return &ToolResult{Success: false, Error: fmt.Sprintf("地点 %d 和 %d 之间已存在关系边，请使用 update_location_relation 修改", existing[0].LocationA, existing[0].LocationB)}, nil
	}

	var ids []int64
	var failedIdx int
	var failedErr error
	err := tc.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for i, item := range a.Relations {
			rel := location.LocationRelation{
				NovelID:      tc.NovelID,
				LocationA:    item.LocationA,
				LocationB:    item.LocationB,
				RelationType: item.RelationType,
				Description:  item.Description,
			}
			if err := tx.Create(&rel).Error; err != nil {
				failedIdx = i + 1
				failedErr = err
				return err
			}
			ids = append(ids, rel.ID)
		}
		return nil
	})
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("创建关系第 %d 条失败: %s", failedIdx, failedErr),
		}, nil
	}

	return &ToolResult{Success: true, Data: map[string]any{"ids": ids, "count": len(ids)}}, nil
}

// ── update_location_relation ────────────────────────────

// UpdateLocationRelationArgs 是 update_location_relation 的参数。
type UpdateLocationRelationArgs struct {
	RelationID   int64  `json:"relation_id" jsonschema:"required,description=关系边ID" validate:"required,min=1"`
	RelationType string `json:"relation_type" jsonschema:"description=新的空间关系描述"`
	Description  string `json:"description" jsonschema:"description=新的补充细节"`
}

// UpdateLocationRelationTool 更新地点空间关系边（PATCH 语义）。
type UpdateLocationRelationTool struct{}

func (t *UpdateLocationRelationTool) Name() string { return "update_location_relation" }
func (t *UpdateLocationRelationTool) Description() string {
	return "更新已有的地点空间连通关系。只需传入要修改的字段，未传入的保持不变。" +
		"通过 relation_id 定位要修改的边。"
}
func (t *UpdateLocationRelationTool) Category() ToolCategory { return CategoryWritingAssistant }

func (t *UpdateLocationRelationTool) JSONSchema() json.RawMessage {
	return SchemaOf(UpdateLocationRelationArgs{})
}
func (t *UpdateLocationRelationTool) ExposeToLLM() bool { return true }
func (t *UpdateLocationRelationTool) NewArgs() any      { return &UpdateLocationRelationArgs{} }

func (t *UpdateLocationRelationTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*UpdateLocationRelationArgs)

	if a.RelationType == "" && a.Description == "" {
		return &ToolResult{Success: false, Error: "至少需要提供一个要修改的字段"}, nil
	}

	var rel location.LocationRelation
	if err := tc.DB.WithContext(ctx).
		Where("id = ? AND novel_id = ?", a.RelationID, tc.NovelID).
		First(&rel).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &ToolResult{Success: false, Error: fmt.Sprintf("关系边 %d 不存在", a.RelationID)}, nil
		}
		return nil, fmt.Errorf("query relation: %w", err)
	}

	if err := json.Unmarshal(tc.RawArgs, &rel); err != nil {
		return &ToolResult{Success: false, Error: "参数格式不正确: " + err.Error()}, nil
	}

	if err := tc.DB.WithContext(ctx).Save(&rel).Error; err != nil {
		return nil, fmt.Errorf("save relation: %w", err)
	}

	return &ToolResult{Success: true, Data: map[string]any{"id": rel.ID}}, nil
}

// ── 格式化 ──────────────────────────────────────────────

func formatLocationDetail(loc location.Location, parentName string, children []location.Location, rels []location.LocationRelation, nameMap map[int64]string) string {
	var parts []string

	// 标题
	parts = append(parts, fmt.Sprintf("### %s [location_id:%d]", loc.Name, loc.ID))

	// 基本信息
	if loc.LocationType != "" {
		parts = append(parts, fmt.Sprintf("- 类型：%s", loc.LocationType))
	}
	if parentName != "" {
		parts = append(parts, fmt.Sprintf("- 父地点：%s", parentName))
	}
	if loc.Description != "" {
		parts = append(parts, fmt.Sprintf("- 描述：%s", loc.Description))
	}
	if v := parseJSONField(loc.DetailJSON); v != nil {
		parts = append(parts, fmt.Sprintf("- 结构化信息：%v", v))
	}
	if v := parseJSONField(loc.Tags); v != nil {
		parts = append(parts, fmt.Sprintf("- 标签：%v", v))
	}

	// 子地点
	if len(children) > 0 {
		lines := []string{fmt.Sprintf("\n#### 子地点（%d个）", len(children))}
		for _, ch := range children {
			lines = append(lines, fmt.Sprintf("- %s [location_id:%d]", ch.Name, ch.ID))
		}
		parts = append(parts, strings.Join(lines, "\n"))
	}

	// 连通
	if len(rels) > 0 {
		lines := []string{"\n#### 连通"}
		for _, rel := range rels {
			line := fmt.Sprintf("- %s — %s [relation_id:%d]：%s",
				nameMap[rel.LocationA], nameMap[rel.LocationB], rel.ID, rel.RelationType)
			if rel.Description != "" {
				line += fmt.Sprintf("（%s）", rel.Description)
			}
			lines = append(lines, line)
		}
		parts = append(parts, strings.Join(lines, "\n"))
	}

	return strings.Join(parts, "\n")
}

func formatLocationNetwork(roots []location.Location, rootRels []location.LocationRelation, childCounts map[int64]int) string {
	var parts []string
	parts = append(parts, "### 大地图")

	// 摘要
	var rootDescs []string
	for _, r := range roots {
		n := childCounts[r.ID]
		if n > 0 {
			rootDescs = append(rootDescs, fmt.Sprintf("%s [location_id:%d]（含%d个子地点）", r.Name, r.ID, n))
		} else {
			rootDescs = append(rootDescs, fmt.Sprintf("%s [location_id:%d]", r.Name, r.ID))
		}
	}
	parts = append(parts, fmt.Sprintf("\n共 %d 个根节点：%s。", len(roots), strings.Join(rootDescs, "、")))

	// 根节点连通
	if len(rootRels) > 0 {
		nameMap := make(map[int64]string)
		for _, r := range roots {
			nameMap[r.ID] = r.Name
		}
		parts = append(parts, "")
		for _, rel := range rootRels {
			line := fmt.Sprintf("- %s — %s [relation_id:%d]：%s",
				nameMap[rel.LocationA], nameMap[rel.LocationB], rel.ID, rel.RelationType)
			if rel.Description != "" {
				line += fmt.Sprintf("（%s）", rel.Description)
			}
			parts = append(parts, line)
		}
	}

	if len(rootRels) == 0 {
		parts = append(parts, "\n暂无根节点间的连通关系。")
	}

	return strings.Join(parts, "\n")
}

// resolveLocationNames 批量解析地点名称。
func resolveLocationNames(ctx context.Context, db *gorm.DB, ids []int64) map[int64]string {
	if len(ids) == 0 {
		return nil
	}
	var locs []location.Location
	if err := db.WithContext(ctx).Where("id IN ?", ids).Find(&locs).Error; err != nil {
		return nil
	}
	m := make(map[int64]string, len(locs))
	for _, l := range locs {
		m[l.ID] = l.Name
	}
	return m
}

// ── 注册 ──────────────────────────────────────────────

// RegisterLocationTools 注册地点管理类工具。
func RegisterLocationTools(r *Registry) {
	r.Register(&GetLocationsTool{})
	r.Register(&CreateLocationTool{})
	r.Register(&UpdateLocationTool{})
	r.Register(&CreateLocationRelationTool{})
	r.Register(&UpdateLocationRelationTool{})
}

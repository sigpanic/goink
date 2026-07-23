package mcp_tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/storage"
	"github.com/sigpanic/goink/internal/storyarc"
)

// ── get_story_arcs ─────────────────────────────────────

// GetStoryArcsArgs 是 get_story_arcs 的参数。
type GetStoryArcsArgs struct {
	CurrentChapter int    `json:"current_chapter" jsonschema:"description=当前章节号。传入时对活跃弧线做窗口切分并检测异常，暂停弧线显示断点和恢复条件。写新章时必填" validate:"omitempty,min=1"`
	ArcType        string `json:"arc_type" jsonschema:"description=按类型筛选,enum=main,enum=sub,enum=character,enum=background" validate:"omitempty,oneof=main sub character background"`
	Status         string `json:"status" jsonschema:"description=按状态筛选,enum=active,enum=paused,enum=completed,enum=abandoned" validate:"omitempty,oneof=active paused completed abandoned"`
	PageArgs              // 嵌入分页参数（仅不传 current_chapter 时生效）
}

// GetStoryArcsTool 获取叙事弧线及节点链。
type GetStoryArcsTool struct{}

func (t *GetStoryArcsTool) Name() string { return "get_story_arcs" }
func (t *GetStoryArcsTool) Description() string {
	return "获取叙事弧线和节点链。两种用法：\n" +
		"- 传入 current_chapter：活跃弧线做窗口切分（近期/异常/未来），暂停弧线显示断点+恢复条件，已完成/废弃弧线仅显示元数据，不要传分页/过滤 参数\n" +
		"- 不传 current_chapter：分页查看所有弧线含完整节点链，需要传分页/过滤 参数"
}
func (t *GetStoryArcsTool) Category() ToolCategory { return CategoryMemoryRetrieval }

func (t *GetStoryArcsTool) JSONSchema() json.RawMessage { return SchemaOf(GetStoryArcsArgs{}) }
func (t *GetStoryArcsTool) ExposeToLLM() bool           { return true }
func (t *GetStoryArcsTool) NewArgs() any                { return &GetStoryArcsArgs{} }

func (t *GetStoryArcsTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*GetStoryArcsArgs)
	a.NormalizePage()

	store := storyarc.NewStore(tc.DB, slog.Default())

	if a.CurrentChapter > 0 {
		return t.executeContext(ctx, a, tc, store)
	}
	return t.executeFull(ctx, a, tc, store)
}

func (t *GetStoryArcsTool) executeContext(ctx context.Context, a *GetStoryArcsArgs, tc ToolContext, store *storyarc.Store) (*ToolResult, error) {
	arcs, err := store.ListNonArchived(ctx, tc.NovelID)
	if err != nil {
		return nil, fmt.Errorf("query arcs: %w", err)
	}

	var activeArcs []storyarc.StoryArc
	var pausedArcs []storyarc.StoryArc
	var activeIDs []int64

	for _, arc := range arcs {
		switch arc.Status {
		case "active":
			activeArcs = append(activeArcs, arc)
			activeIDs = append(activeIDs, arc.ID)
		case "paused":
			pausedArcs = append(pausedArcs, arc)
		}
	}

	// 每条活跃弧线独占窗口
	beforeByArc, err := store.ListNodesBeforeByArc(ctx, activeIDs, a.CurrentChapter, 10)
	if err != nil {
		return nil, fmt.Errorf("query history: %w", err)
	}
	anomalyByArc, err := store.ListPendingNodesBeforeByArc(ctx, activeIDs, a.CurrentChapter)
	if err != nil {
		return nil, fmt.Errorf("query anomalies: %w", err)
	}
	afterByArc, err := store.ListNodesAfterByArc(ctx, activeIDs, a.CurrentChapter)
	if err != nil {
		return nil, fmt.Errorf("query future: %w", err)
	}

	// 暂停弧线：断点
	pausedBefore := make(map[int64][]storyarc.ArcNode)
	pausedPending := make(map[int64][]storyarc.ArcNode)
	for _, arc := range pausedArcs {
		before, pending, err := store.GetBreakpoint(ctx, arc.ID)
		if err != nil {
			return nil, fmt.Errorf("query breakpoint arc %d: %w", arc.ID, err)
		}
		if before != nil || pending != nil {
			pausedBefore[arc.ID] = before
			pausedPending[arc.ID] = pending
		}
	}

	// 已完成/废弃的弧线（仅元数据）
	var archivedArcs []storyarc.StoryArc
	tc.DB.WithContext(ctx).
		Where("novel_id = ? AND status IN ?", tc.NovelID, []string{"completed", "abandoned"}).
		Order("importance DESC").
		Find(&archivedArcs)

	formatted := formatArcsAll(activeArcs, beforeByArc, anomalyByArc, afterByArc,
		pausedArcs, pausedBefore, pausedPending,
		archivedArcs, a.CurrentChapter)

	return &ToolResult{
		Success: true,
		Data:    map[string]any{"content": formatted},
	}, nil
}

func (t *GetStoryArcsTool) executeFull(ctx context.Context, a *GetStoryArcsArgs, tc ToolContext, store *storyarc.Store) (*ToolResult, error) {
	result, err := store.ListByNovel(ctx, tc.NovelID, storyarc.ListByNovelOptions{
		PageParams: storage.PageParams{Page: a.Page, Size: a.Size},
		ArcType:    a.ArcType,
		Status:     a.Status,
	})
	if err != nil {
		return nil, fmt.Errorf("list arcs: %w", err)
	}

	var arcIDs []int64
	for _, arc := range result.Items {
		arcIDs = append(arcIDs, arc.ID)
	}
	nodes, _ := store.ListByArcs(ctx, arcIDs)

	formatted := formatArcsFull(result.Items, nodes)

	data := PageMeta(result)
	data["content"] = formatted

	return &ToolResult{Success: true, Data: data}, nil
}

// ── create_story_arc ───────────────────────────────────

// CreateStoryArcItem 是 create_story_arc 的单条参数。
type CreateStoryArcItem struct {
	Name        string `json:"name" jsonschema:"required,description=弧线名称，如'复仇之路'"                          validate:"required"`
	ArcType     string `json:"arc_type" jsonschema:"required,description=弧线类型,enum=main,enum=sub,enum=character,enum=background" validate:"required,oneof=main sub character background"`
	Description string `json:"description" jsonschema:"description=弧线整体描述"`
	Importance  int    `json:"importance" jsonschema:"description=重要度1-5,default=1,minimum=1,maximum=5"          validate:"omitempty,min=1,max=5"`
}

// CreateStoryArcArgs 是 create_story_arc 的参数。
type CreateStoryArcArgs struct {
	StoryArcs []CreateStoryArcItem `json:"story_arcs" jsonschema:"required,description=要创建的弧线列表（1-5个）" validate:"required,min=1,max=5,dive"`
}

// CreateStoryArcTool 创建新叙事弧线。
type CreateStoryArcTool struct{}

func (t *CreateStoryArcTool) Name() string { return "create_story_arc" }
func (t *CreateStoryArcTool) Description() string {
	return "批量创建叙事弧线（1-5个）。保证原子性，失败时返回具体条目原因。" +
		"弧线类型：main（主线）/ sub（支线）/ character（角色线）/ background（背景线）。" +
		"弧线是跨越多章节的故事线容器，内部节点通过 create_arc_node 添加。"
}
func (t *CreateStoryArcTool) Category() ToolCategory { return CategoryWritingAssistant }

func (t *CreateStoryArcTool) JSONSchema() json.RawMessage { return SchemaOf(CreateStoryArcArgs{}) }
func (t *CreateStoryArcTool) ExposeToLLM() bool           { return true }
func (t *CreateStoryArcTool) NewArgs() any                { return &CreateStoryArcArgs{} }

func (t *CreateStoryArcTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*CreateStoryArcArgs)

	var ids []int64
	var failedName string
	var failedErr error
	err := tc.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, item := range a.StoryArcs {
			importance := item.Importance
			if importance == 0 {
				importance = 1
			}
			arc := storyarc.StoryArc{
				NovelID:     tc.NovelID,
				Name:        item.Name,
				ArcType:     item.ArcType,
				Description: item.Description,
				Importance:  importance,
				Status:      "active",
			}
			if err := tx.Create(&arc).Error; err != nil {
				failedName = item.Name
				failedErr = err
				return err
			}
			ids = append(ids, arc.ID)
		}
		return nil
	})
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("创建弧线 [%s] 失败: %s", failedName, failedErr),
		}, nil
	}

	return &ToolResult{
		Success: true,
		Data:    map[string]any{"ids": ids, "count": len(ids)},
	}, nil
}

// ── update_story_arc ───────────────────────────────────

// UpdateStoryArcArgs 是 update_story_arc 的参数。
type UpdateStoryArcArgs struct {
	ArcID        int64  `json:"arc_id" jsonschema:"required,description=弧线ID"                         validate:"required,min=1"`
	Name         string `json:"name" jsonschema:"description=新的弧线名称"`
	Description  string `json:"description" jsonschema:"description=新的描述"`
	ArcType      string `json:"arc_type" jsonschema:"description=新的弧线类型,enum=main,enum=sub,enum=character,enum=background" validate:"omitempty,oneof=main sub character background"`
	Importance   int    `json:"importance" jsonschema:"description=新的重要度1-5,minimum=1,maximum=5"`
	Status       string `json:"status" jsonschema:"description=新状态,enum=active,enum=paused,enum=completed,enum=abandoned" validate:"omitempty,oneof=active paused completed abandoned"`
	ReactivateAt string `json:"reactivate_at" jsonschema:"description=暂停弧线的恢复条件，自然语言。状态改为paused时填写"`
}

// UpdateStoryArcTool 更新叙事弧线元数据（PATCH 语义）。
type UpdateStoryArcTool struct{}

func (t *UpdateStoryArcTool) Name() string { return "update_story_arc" }
func (t *UpdateStoryArcTool) Description() string {
	return "更新叙事弧线的元数据。只需传入要修改的字段。" +
		"常用：暂停弧线（status=paused + reactivate_at）、完成弧线（status=completed）、废弃弧线（status=abandoned）。"
}
func (t *UpdateStoryArcTool) Category() ToolCategory { return CategoryWritingAssistant }

func (t *UpdateStoryArcTool) JSONSchema() json.RawMessage { return SchemaOf(UpdateStoryArcArgs{}) }
func (t *UpdateStoryArcTool) ExposeToLLM() bool           { return true }
func (t *UpdateStoryArcTool) NewArgs() any                { return &UpdateStoryArcArgs{} }

func (t *UpdateStoryArcTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*UpdateStoryArcArgs)

	if a.Name == "" && a.Description == "" && a.ArcType == "" && a.Importance == 0 && a.Status == "" && a.ReactivateAt == "" {
		return &ToolResult{Success: false, Error: "至少需要提供一个要修改的字段"}, nil
	}

	var arc storyarc.StoryArc
	if err := tc.DB.WithContext(ctx).
		Where("id = ? AND novel_id = ?", a.ArcID, tc.NovelID).
		First(&arc).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &ToolResult{Success: false, Error: fmt.Sprintf("弧线 %d 不存在", a.ArcID)}, nil
		}
		return nil, fmt.Errorf("query arc: %w", err)
	}

	if err := json.Unmarshal(tc.RawArgs, &arc); err != nil {
		return &ToolResult{Success: false, Error: "参数格式不正确: " + err.Error()}, nil
	}

	if err := tc.DB.WithContext(ctx).Save(&arc).Error; err != nil {
		return nil, fmt.Errorf("save arc: %w", err)
	}

	return &ToolResult{Success: true, Data: map[string]any{"id": arc.ID}}, nil
}

// ── create_arc_node ────────────────────────────────────

// CreateArcNodeItem 是 create_arc_node 的单条参数。
type CreateArcNodeItem struct {
	StoryArcID    int64  `json:"arc_id" jsonschema:"required,description=所属弧线ID"                  validate:"required,min=1"`
	Title         string `json:"title" jsonschema:"required,description=节点标题，如'发现仇人身份'"              validate:"required"`
	Description   string `json:"description" jsonschema:"description=节点详情"`
	TargetChapter int    `json:"target_chapter" jsonschema:"required,description=预计发生章节号（不准确不要紧）"       validate:"required,min=1"`
}

// CreateArcNodeArgs 是 create_arc_node 的参数。
type CreateArcNodeArgs struct {
	ArcNodes []CreateArcNodeItem `json:"arc_nodes" jsonschema:"required,description=要创建的节点列表（1-10个）" validate:"required,min=1,max=10,dive"`
}

// CreateArcNodeTool 新建弧线节点。
type CreateArcNodeTool struct{}

func (t *CreateArcNodeTool) Name() string { return "create_arc_node" }
func (t *CreateArcNodeTool) Description() string {
	return "批量向弧线添加节点（1-10个）。保证原子性，失败时返回具体条目原因。" +
		"target_chapter 为预计发生的章节号（不准确不要紧，后续可通过 update_arc_node 调整）。" +
		"节点按 target_chapter 排序构成弧线演进链。"
}
func (t *CreateArcNodeTool) Category() ToolCategory { return CategoryWritingAssistant }

func (t *CreateArcNodeTool) JSONSchema() json.RawMessage { return SchemaOf(CreateArcNodeArgs{}) }
func (t *CreateArcNodeTool) ExposeToLLM() bool           { return true }
func (t *CreateArcNodeTool) NewArgs() any                { return &CreateArcNodeArgs{} }

func (t *CreateArcNodeTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*CreateArcNodeArgs)

	// 预校验：批量验证所有弧线存在且属于当前小说
	arcIDSet := make(map[int64]bool)
	for _, item := range a.ArcNodes {
		arcIDSet[item.StoryArcID] = true
	}
	arcIDs := make([]int64, 0, len(arcIDSet))
	for id := range arcIDSet {
		arcIDs = append(arcIDs, id)
	}
	var count int64
	if err := tc.DB.WithContext(ctx).Model(&storyarc.StoryArc{}).
		Where("id IN ? AND novel_id = ?", arcIDs, tc.NovelID).
		Count(&count).Error; err != nil {
		return nil, fmt.Errorf("verify arcs: %w", err)
	}
	if int(count) != len(arcIDs) {
		return &ToolResult{Success: false, Error: "部分弧线不存在或不属于当前小说"}, nil
	}

	var ids []int64
	var failedName string
	var failedErr error
	err := tc.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, item := range a.ArcNodes {
			node := storyarc.ArcNode{
				NovelID:       tc.NovelID,
				StoryArcID:    item.StoryArcID,
				Title:         item.Title,
				Description:   item.Description,
				TargetChapter: item.TargetChapter,
				Status:        "pending",
			}
			if err := tx.Create(&node).Error; err != nil {
				failedName = item.Title
				failedErr = err
				return err
			}
			ids = append(ids, node.ID)
		}
		return nil
	})
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("创建节点 [%s] 失败: %s", failedName, failedErr),
		}, nil
	}

	return &ToolResult{
		Success: true,
		Data:    map[string]any{"ids": ids, "count": len(ids)},
	}, nil
}

// ── update_arc_node ────────────────────────────────────

// UpdateArcNodeArgs 是 update_arc_node 的参数。
type UpdateArcNodeArgs struct {
	NodeID        int64  `json:"node_id" jsonschema:"required,description=节点ID"              validate:"required,min=1"`
	Title         string `json:"title" jsonschema:"description=新的标题"`
	Description   string `json:"description" jsonschema:"description=新的描述"`
	TargetChapter int    `json:"target_chapter" jsonschema:"description=新的目标章节号,minimum=1" validate:"omitempty,min=1"`
	ActualChapter int    `json:"actual_chapter" jsonschema:"description=实际发生的章节号（标记完成时填入）"`
	Status        string `json:"status" jsonschema:"description=新状态,enum=pending,enum=completed,enum=abandoned" validate:"omitempty,oneof=pending completed abandoned"`
}

// UpdateArcNodeTool 更新弧线节点（PATCH 语义）。
type UpdateArcNodeTool struct{}

func (t *UpdateArcNodeTool) Name() string { return "update_arc_node" }
func (t *UpdateArcNodeTool) Description() string {
	return "更新已有的弧线节点。只需传入要修改的字段。" +
		"标记完成时填 actual_chapter + status=completed。调整 target_chapter 可改变节点在弧线链中的顺序。"
}
func (t *UpdateArcNodeTool) Category() ToolCategory { return CategoryWritingAssistant }

func (t *UpdateArcNodeTool) JSONSchema() json.RawMessage { return SchemaOf(UpdateArcNodeArgs{}) }
func (t *UpdateArcNodeTool) ExposeToLLM() bool           { return true }
func (t *UpdateArcNodeTool) NewArgs() any                { return &UpdateArcNodeArgs{} }

func (t *UpdateArcNodeTool) Execute(ctx context.Context, args any, tc ToolContext) (*ToolResult, error) {
	a := args.(*UpdateArcNodeArgs)

	if a.Title == "" && a.Description == "" && a.TargetChapter == 0 && a.ActualChapter == 0 && a.Status == "" {
		return &ToolResult{Success: false, Error: "至少需要提供一个要修改的字段"}, nil
	}

	var node storyarc.ArcNode
	if err := tc.DB.WithContext(ctx).
		Where("id = ? AND novel_id = ?", a.NodeID, tc.NovelID).
		First(&node).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &ToolResult{Success: false, Error: fmt.Sprintf("节点 %d 不存在", a.NodeID)}, nil
		}
		return nil, fmt.Errorf("query node: %w", err)
	}

	if err := json.Unmarshal(tc.RawArgs, &node); err != nil {
		return &ToolResult{Success: false, Error: "参数格式不正确: " + err.Error()}, nil
	}

	if err := tc.DB.WithContext(ctx).Save(&node).Error; err != nil {
		return nil, fmt.Errorf("save node: %w", err)
	}

	return &ToolResult{Success: true, Data: map[string]any{"id": node.ID}}, nil
}

// ── 格式化 ──────────────────────────────────────────────

func formatArcsAll(activeArcs []storyarc.StoryArc, beforeByArc, anomalyByArc, afterByArc map[int64][]storyarc.ArcNode, pausedArcs []storyarc.StoryArc, pausedBefore, pausedPending map[int64][]storyarc.ArcNode, archivedArcs []storyarc.StoryArc, currentChapter int) string {
	if len(activeArcs) == 0 && len(pausedArcs) == 0 && len(archivedArcs) == 0 {
		return "暂无叙事弧线。"
	}

	var parts []string
	parts = append(parts, "### 叙事弧线")

	for _, arc := range activeArcs {
		parts = append(parts, formatActiveArc(arc,
			beforeByArc[arc.ID], anomalyByArc[arc.ID], afterByArc[arc.ID], currentChapter))
	}

	for _, arc := range pausedArcs {
		parts = append(parts, formatPausedArc(arc, pausedBefore[arc.ID], pausedPending[arc.ID]))
	}

	for _, arc := range archivedArcs {
		parts = append(parts, formatArchivedArc(arc))
	}

	return strings.Join(parts, "\n")
}

func formatActiveArc(arc storyarc.StoryArc, before, anomalies, after []storyarc.ArcNode, currentChapter int) string {
	var parts []string
	parts = append(parts, fmt.Sprintf("\n#### %s [arc_id:%d] (%s) — %s %s",
		arc.Name, arc.ID, arc.ArcType, arc.Status, importanceStars(arc.Importance)))
	if arc.Description != "" {
		parts = append(parts, arc.Description)
	}

	if len(before) == 0 && len(anomalies) == 0 && len(after) == 0 {
		parts = append(parts, "（暂无节点）")
		return strings.Join(parts, "\n")
	}

	if len(before) > 0 {
		lines := []string{fmt.Sprintf("\n##### 近期（最近%d个节点，截至第%d章）", len(before), currentChapter)}
		for _, n := range before {
			lines = append(lines, formatNode(n, false))
		}
		parts = append(parts, strings.Join(lines, "\n"))
	}

	if len(anomalies) > 0 {
		lines := []string{"\n##### ⚠️ 异常", "以下节点需要关注和修正："}
		for _, n := range anomalies {
			lines = append(lines, fmt.Sprintf("- %s [node_id:%d] — 目标第%d章但仍pending，应在第%d章前完成",
				n.Title, n.ID, n.TargetChapter, currentChapter))
		}
		parts = append(parts, strings.Join(lines, "\n"))
	}

	if len(after) > 0 {
		lines := []string{fmt.Sprintf("\n##### 未来（%d个节点）", len(after))}
		for _, n := range after {
			lines = append(lines, formatNode(n, false))
		}
		parts = append(parts, strings.Join(lines, "\n"))
	}

	return strings.Join(parts, "\n")
}

func formatPausedArc(arc storyarc.StoryArc, before, pending []storyarc.ArcNode) string {
	var parts []string
	parts = append(parts, fmt.Sprintf("\n#### %s [arc_id:%d] (%s) — paused %s",
		arc.Name, arc.ID, arc.ArcType, importanceStars(arc.Importance)))
	if arc.ReactivateAt != "" {
		parts = append(parts, fmt.Sprintf("恢复条件：%s", arc.ReactivateAt))
	}

	if len(before) == 0 && len(pending) == 0 {
		return strings.Join(parts, "\n")
	}

	parts = append(parts, "")
	for _, n := range before {
		ch := n.ActualChapter
		if ch == 0 {
			ch = n.TargetChapter
		}
		parts = append(parts, fmt.Sprintf("- %s [node_id:%d] — 第%d章 ✓", n.Title, n.ID, ch))
	}
	for i, n := range pending {
		if i == 0 {
			parts = append(parts, fmt.Sprintf("- ▶ %s [node_id:%d] — 目标第%d章", n.Title, n.ID, n.TargetChapter))
		} else {
			parts = append(parts, fmt.Sprintf("- %s [node_id:%d] — 目标第%d章", n.Title, n.ID, n.TargetChapter))
		}
	}

	return strings.Join(parts, "\n")
}

func formatArchivedArc(arc storyarc.StoryArc) string {
	return fmt.Sprintf("\n#### %s [arc_id:%d] (%s) — %s %s",
		arc.Name, arc.ID, arc.ArcType, arc.Status, importanceStars(arc.Importance))
}

func formatArcsFull(arcs []storyarc.StoryArc, nodes []storyarc.ArcNode) string {
	if len(arcs) == 0 {
		return "暂无叙事弧线。"
	}

	var parts []string
	parts = append(parts, "### 叙事弧线")

	nodesByArc := groupNodesByArc(nodes)

	for _, arc := range arcs {
		parts = append(parts, fmt.Sprintf("\n#### %s [arc_id:%d] (%s) — %s %s",
			arc.Name, arc.ID, arc.ArcType, arc.Status, importanceStars(arc.Importance)))
		if arc.Description != "" {
			parts = append(parts, arc.Description)
		}
		if arc.Status == "paused" && arc.ReactivateAt != "" {
			parts = append(parts, fmt.Sprintf("恢复条件：%s", arc.ReactivateAt))
		}

		arcNodes := nodesByArc[arc.ID]
		if len(arcNodes) == 0 {
			parts = append(parts, "（暂无节点）")
			continue
		}
		for _, n := range arcNodes {
			parts = append(parts, formatNode(n, true))
		}
	}

	return strings.Join(parts, "\n")
}

func formatNode(n storyarc.ArcNode, showStatus bool) string {
	statusStr := ""
	if n.ActualChapter > 0 {
		statusStr = fmt.Sprintf(" — 第%d章 ✓", n.ActualChapter)
	} else if n.TargetChapter > 0 {
		statusStr = fmt.Sprintf(" — 目标第%d章", n.TargetChapter)
	}
	if showStatus && n.Status == "abandoned" {
		statusStr += " — 已废弃"
	}

	return fmt.Sprintf("- %s [node_id:%d]%s", n.Title, n.ID, statusStr)
}

func groupNodesByArc(nodes []storyarc.ArcNode) map[int64][]storyarc.ArcNode {
	m := make(map[int64][]storyarc.ArcNode)
	for _, n := range nodes {
		m[n.StoryArcID] = append(m[n.StoryArcID], n)
	}
	return m
}

func importanceStars(v int) string {
	if v <= 0 {
		return ""
	}
	s := "⭐"
	for i := 1; i < v && i < 5; i++ {
		s += "⭐"
	}
	return s
}

// ── 注册 ──────────────────────────────────────────────

// RegisterStoryArcTools 注册叙事弧线管理类工具。
func RegisterStoryArcTools(r *Registry) {
	r.Register(&GetStoryArcsTool{})
	r.Register(&CreateStoryArcTool{})
	r.Register(&UpdateStoryArcTool{})
	r.Register(&CreateArcNodeTool{})
	r.Register(&UpdateArcNodeTool{})
}

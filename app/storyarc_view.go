package app

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/storage"
	"github.com/sigpanic/goink/internal/storyarc"
)

// GetStoryArcs 返回指定小说的全部叙事弧线。弧线通常 3-5 条，全量无分页。
// 4b: 显式传 Order 保持原排序行为（importance DESC, created_at ASC）。
func (a *App) GetStoryArcs(novelID int64) ([]storyarc.StoryArc, error) {
	result, err := a.storyarc.ListByNovel(a.ctx, novelID, storyarc.ListByNovelOptions{
		PageParams: storage.PageParams{Size: -1},
		Order:      "importance DESC, created_at ASC",
	})
	if err != nil {
		return nil, err
	}
	if result.Items == nil {
		return []storyarc.StoryArc{}, nil
	}
	return result.Items, nil
}

// GetArcNodes 返回指定小说的全部弧线节点，供前端列表和关系图渲染。
// 4b: 改调 ListNodesByNovel(Size=-1) 全量（废弃 ListNodesByChapterRange）。
// 显式传 Order 补回 story_arc_id 前缀：同一条弧线的节点聚簇连续，避免被跨弧线打散。
func (a *App) GetArcNodes(novelID int64) ([]storyarc.ArcNode, error) {
	result, err := a.storyarc.ListNodesByNovel(a.ctx, novelID, storyarc.ListNodesOptions{
		PageParams: storage.PageParams{Size: -1},
		Order:      "story_arc_id, target_chapter ASC, id ASC",
	})
	if err != nil {
		return nil, err
	}
	if result.Items == nil {
		return []storyarc.ArcNode{}, nil
	}
	return result.Items, nil
}

// ── StoryArc CRUD ──────────────────────────────────────

// CreateStoryArcInput 是 CreateStoryArc 的参数。
type CreateStoryArcInput struct {
	Name        string `json:"name"`                  // 弧线名称，必填
	ArcType     string `json:"arc_type"`              // 弧线类型，必填
	Description string `json:"description,omitempty"` // 弧线整体描述
	Importance  int    `json:"importance,omitempty"`  // 重要度 1-5
}

// CreateStoryArc 创建一条叙事弧线。
func (a *App) CreateStoryArc(novelID int64, input CreateStoryArcInput) (*storyarc.StoryArc, error) {
	if input.Name == "" || input.ArcType == "" {
		return nil, fmt.Errorf("弧线名称和类型不能为空")
	}
	arc := storyarc.StoryArc{
		NovelID:     novelID,
		Name:        input.Name,
		ArcType:     input.ArcType,
		Description: input.Description,
		Importance:  input.Importance,
		Status:      "active",
	}
	if arc.Importance == 0 {
		arc.Importance = 1
	}
	if err := a.storyarc.DB.WithContext(a.ctx).Create(&arc).Error; err != nil {
		return nil, fmt.Errorf("create story arc: %w", err)
	}
	return &arc, nil
}

// UpdateStoryArcInput 采用 PUT 语义：前端全量传，后端全量覆盖。
// ReactivateAt 是 AI 写入字段，不在 input 里，后端 First 加载原值保留。
type UpdateStoryArcInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	ArcType     string `json:"arc_type"`
	Importance  int    `json:"importance"`
	Status      string `json:"status"` // "active" | "paused" | "completed" | "abandoned"
}

// UpdateStoryArc 更新叙事弧线。PUT 全量覆盖用户可编辑字段。
func (a *App) UpdateStoryArc(novelID int64, arcID int64, input UpdateStoryArcInput) error {
	var arc storyarc.StoryArc
	if err := a.storyarc.DB.WithContext(a.ctx).
		Where("id = ? AND novel_id = ?", arcID, novelID).
		First(&arc).Error; err != nil {
		return fmt.Errorf("update story arc: %w", err)
	}
	// PUT 全量覆盖用户可编辑字段。ReactivateAt 是 AI 写入字段，不在 input 里，
	// 保留 First 加载的原值，避免前端编辑保存覆盖 AI 写入的新值（lost update）。
	arc.Name = input.Name
	arc.Description = input.Description
	arc.ArcType = input.ArcType
	arc.Importance = input.Importance
	arc.Status = input.Status
	if err := a.storyarc.DB.WithContext(a.ctx).Save(&arc).Error; err != nil {
		return fmt.Errorf("update story arc: %w", err)
	}
	return nil
}

// DeleteStoryArc 删除一条叙事弧线（级联删除关联节点）。
func (a *App) DeleteStoryArc(novelID int64, arcID int64) error {
	return a.storyarc.DB.WithContext(a.ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("story_arc_id = ? AND novel_id = ?", arcID, novelID).
			Delete(&storyarc.ArcNode{}).Error; err != nil {
			return fmt.Errorf("delete arc nodes: %w", err)
		}
		if err := tx.Where("id = ? AND novel_id = ?", arcID, novelID).
			Delete(&storyarc.StoryArc{}).Error; err != nil {
			return fmt.Errorf("delete story arc: %w", err)
		}
		return nil
	})
}

// ── ArcNode CRUD ───────────────────────────────────────

// CreateArcNodeInput 是 CreateArcNode 的参数。
type CreateArcNodeInput struct {
	StoryArcID    int64  `json:"story_arc_id"`          // 所属弧线 ID，必填
	Title         string `json:"title"`                 // 节点标题，必填
	Description   string `json:"description,omitempty"` // 节点详情
	TargetChapter int    `json:"target_chapter"`        // 预计章节号，必填
}

// CreateArcNode 创建弧线节点。
func (a *App) CreateArcNode(novelID int64, input CreateArcNodeInput) (*storyarc.ArcNode, error) {
	if input.Title == "" || input.StoryArcID == 0 || input.TargetChapter == 0 {
		return nil, fmt.Errorf("节点标题、所属弧线、目标章节不能为空")
	}
	node := storyarc.ArcNode{
		NovelID:       novelID,
		StoryArcID:    input.StoryArcID,
		Title:         input.Title,
		Description:   input.Description,
		TargetChapter: input.TargetChapter,
		Status:        "pending",
	}
	if err := a.storyarc.DB.WithContext(a.ctx).Create(&node).Error; err != nil {
		return nil, fmt.Errorf("create arc node: %w", err)
	}
	return &node, nil
}

// UpdateArcNodeInput 采用 PUT 语义：前端全量传，后端全量覆盖。
type UpdateArcNodeInput struct {
	Title         string `json:"title"`
	Description   string `json:"description"`
	TargetChapter int    `json:"target_chapter"`
	ActualChapter int    `json:"actual_chapter"`
	Status        string `json:"status"` // "pending" | "completed" | "abandoned"
}

// UpdateArcNode 更新弧线节点。PUT 全量覆盖用户可编辑字段。
func (a *App) UpdateArcNode(novelID int64, nodeID int64, input UpdateArcNodeInput) error {
	var node storyarc.ArcNode
	if err := a.storyarc.DB.WithContext(a.ctx).
		Where("id = ? AND novel_id = ?", nodeID, novelID).
		First(&node).Error; err != nil {
		return fmt.Errorf("update arc node: %w", err)
	}
	node.Title = input.Title
	node.Description = input.Description
	node.TargetChapter = input.TargetChapter
	node.ActualChapter = input.ActualChapter
	node.Status = input.Status
	if err := a.storyarc.DB.WithContext(a.ctx).Save(&node).Error; err != nil {
		return fmt.Errorf("update arc node: %w", err)
	}
	return nil
}

// DeleteArcNode 删除弧线节点。
func (a *App) DeleteArcNode(novelID int64, nodeID int64) error {
	if err := a.storyarc.DB.WithContext(a.ctx).
		Where("id = ? AND novel_id = ?", nodeID, novelID).
		Delete(&storyarc.ArcNode{}).Error; err != nil {
		return fmt.Errorf("delete arc node: %w", err)
	}
	return nil
}

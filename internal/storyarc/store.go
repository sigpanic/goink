package storyarc

import (
	"context"
	"fmt"
	"log/slog"

	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/storage"
)

// Store 管理 StoryArc 和 ArcNode 持久化。DB 导出供调用方做简单 CRUD。
//
// 设计要点：
//   - 弧线节点窗口策略跟 TimelineEntry 一致：以章节号为锚点，ListNodesAfter 覆盖未来（>=N），
//     ListNodesBefore 覆盖近期历史（<N 最近 N 条），ListPendingNodesBefore 兜底远古未完成的。
//     三个方法返回原始数据，MCP 工具负责格式化输出。
//   - ArcNode 没有 Upsert——create/update 拆开，create 直接 INSERT，不依赖序列号。
//   - paused 弧线的恢复不在此层判断——MCP 工具将弧线名+断点（下个 pending 节点的 title+target_chapter）
//     +reactivate_at 格式化后呈现给 LLM，LLM 自行判断是否满足恢复条件。
type Store struct {
	DB     *gorm.DB
	logger *slog.Logger
}

// NewStore 创建 storyarc 存储。
func NewStore(db *gorm.DB, logger *slog.Logger) *Store {
	return &Store{DB: db, logger: logger}
}

// ── StoryArc ─────────────────────────────────────────

// ListByNovelOptions 是 ListByNovel 的可选参数。
type ListByNovelOptions struct {
	PageParams storage.PageParams
	ArcType    string // 空字符串=不过滤，"main"/"sub"/"character"/"background"
	Status     string // 空字符串=不过滤，"active"/"paused"/"completed"/"abandoned"
	Search     string // 空字符串=不过滤，按 name LIKE OR description LIKE 模糊匹配
	Order      string // 空字符串=默认 importance DESC, created_at ASC；调用方显式传（App 传原排序，MCP 传 updated_at DESC）
}

// ListByNovel 分页列出某小说的叙事弧线，支持类型/状态过滤和名称搜索。前端管理页、MCP full 模式、全局搜索复用。
func (s *Store) ListByNovel(ctx context.Context, novelID int64, opts ListByNovelOptions) (*storage.PageResult[StoryArc], error) {
	pp := opts.PageParams
	pp.Normalize()

	q := s.DB.WithContext(ctx).Model(&StoryArc{}).Where("novel_id = ?", novelID)

	if opts.ArcType != "" {
		q = q.Where("arc_type = ?", opts.ArcType)
	}
	if opts.Status != "" {
		q = q.Where("status = ?", opts.Status)
	}
	if opts.Search != "" {
		q = q.Where("name LIKE ? OR description LIKE ?", "%"+opts.Search+"%", "%"+opts.Search+"%")
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("storyarc store: count: %w", err)
	}

	order := opts.Order
	if order == "" {
		order = "importance DESC, created_at ASC"
	}
	var arcs []StoryArc
	if err := q.Order(order).Offset(pp.Offset()).Limit(pp.Size).Find(&arcs).Error; err != nil {
		return nil, fmt.Errorf("storyarc store: list: %w", err)
	}

	s.logger.Debug("storyarc store: listed", "novel_id", novelID, "total", total, "page", pp.Page)
	return storage.NewPageResult(arcs, total, pp.Page, pp.Size), nil
}

// ListNonArchived 返回 active 和 paused 的弧线。completed/abandoned 由 MCP 工具按需单查，store 不限制。
func (s *Store) ListNonArchived(ctx context.Context, novelID int64) ([]StoryArc, error) {
	var arcs []StoryArc
	if err := s.DB.WithContext(ctx).
		Where("novel_id = ? AND status IN ?", novelID, []string{"active", "paused"}).
		Order("importance DESC, created_at ASC").
		Find(&arcs).Error; err != nil {
		return nil, fmt.Errorf("storyarc store: list non-archived: %w", err)
	}
	return arcs, nil
}

// ── ArcNode ──────────────────────────────────────────

// ListByArcs 批量取多条弧线的全部节点，按 (story_arc_id, target_chapter, id) 排序。
// MCP 工具展开弧线链和前端渲染用。返回的是全量节点，不做窗口切分。
func (s *Store) ListByArcs(ctx context.Context, arcIDs []int64) ([]ArcNode, error) {
	if len(arcIDs) == 0 {
		return nil, nil
	}
	var nodes []ArcNode
	if err := s.DB.WithContext(ctx).
		Where("story_arc_id IN ?", arcIDs).
		Order("story_arc_id, target_chapter ASC, id ASC").
		Find(&nodes).Error; err != nil {
		return nil, fmt.Errorf("storyarc store: list by arcs: %w", err)
	}
	return nodes, nil
}

// ListNodesOptions 是 ListNodesByNovel 的可选参数。
type ListNodesOptions struct {
	PageParams storage.PageParams
	Search     string // 空字符串=不过滤，按 title LIKE OR description LIKE 模糊匹配
	Order      string // 空字符串=默认 story_arc_id, target_chapter ASC, id ASC
}

// ListNodesByNovel 分页列出某小说的全部弧线节点，支持名称搜索。
// 前端全量拉取（Size=-1）和全局搜索复用。
// 废弃 ListNodesByChapterRange（当前无调用方需要章节范围查；per-arc 窗口切分走 ListNodesBeforeByArc/AfterByArc/PendingBeforeByArc）。
func (s *Store) ListNodesByNovel(ctx context.Context, novelID int64, opts ListNodesOptions) (*storage.PageResult[ArcNode], error) {
	pp := opts.PageParams
	pp.Normalize()

	q := s.DB.WithContext(ctx).Model(&ArcNode{}).Where("novel_id = ?", novelID)

	if opts.Search != "" {
		q = q.Where("title LIKE ? OR description LIKE ?", "%"+opts.Search+"%", "%"+opts.Search+"%")
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("storyarc store: count nodes: %w", err)
	}

	order := opts.Order
	if order == "" {
		order = "story_arc_id, target_chapter ASC, id ASC" // 与废弃的 ListNodesByChapterRange 一致，同弧线节点聚簇连续
	}
	var nodes []ArcNode
	if err := q.Order(order).Offset(pp.Offset()).Limit(pp.Size).Find(&nodes).Error; err != nil {
		return nil, fmt.Errorf("storyarc store: list nodes: %w", err)
	}

	s.logger.Debug("storyarc store: listed nodes", "novel_id", novelID, "total", total, "page", pp.Page)
	return storage.NewPageResult(nodes, total, pp.Page, pp.Size), nil
}

// ListNodesBeforeByArc 对每条弧线分别取 target_chapter < chapterNum 的最近 limit 条节点。
// 返回 map[arcID]nodes，保证每条弧线独占窗口。
func (s *Store) ListNodesBeforeByArc(ctx context.Context, arcIDs []int64, chapterNum int, limit int) (map[int64][]ArcNode, error) {
	result := make(map[int64][]ArcNode)
	for _, id := range arcIDs {
		var nodes []ArcNode
		if err := s.DB.WithContext(ctx).
			Where("story_arc_id = ? AND target_chapter < ?", id, chapterNum).
			Order("target_chapter DESC").
			Limit(limit).
			Find(&nodes).Error; err != nil {
			return nil, fmt.Errorf("storyarc store: nodes before arc %d: %w", id, err)
		}
		result[id] = nodes
	}
	return result, nil
}

// ListPendingNodesBeforeByArc 对每条弧线分别取 target_chapter < chapterNum 且 pending 的全部节点，兜底截断 100。
func (s *Store) ListPendingNodesBeforeByArc(ctx context.Context, arcIDs []int64, chapterNum int) (map[int64][]ArcNode, error) {
	result := make(map[int64][]ArcNode)
	for _, id := range arcIDs {
		var nodes []ArcNode
		if err := s.DB.WithContext(ctx).
			Where("story_arc_id = ? AND target_chapter < ? AND status = ?", id, chapterNum, "pending").
			Order("target_chapter ASC").
			Limit(100).
			Find(&nodes).Error; err != nil {
			return nil, fmt.Errorf("storyarc store: pending before arc %d: %w", id, err)
		}
		result[id] = nodes
	}
	return result, nil
}

// ListNodesAfterByArc 对每条弧线分别取 target_chapter >= chapterNum 的全部节点，兜底截断 100。
func (s *Store) ListNodesAfterByArc(ctx context.Context, arcIDs []int64, chapterNum int) (map[int64][]ArcNode, error) {
	result := make(map[int64][]ArcNode)
	for _, id := range arcIDs {
		var nodes []ArcNode
		if err := s.DB.WithContext(ctx).
			Where("story_arc_id = ? AND target_chapter >= ?", id, chapterNum).
			Order("target_chapter ASC").
			Limit(100).
			Find(&nodes).Error; err != nil {
			return nil, fmt.Errorf("storyarc store: nodes after arc %d: %w", id, err)
		}
		result[id] = nodes
	}
	return result, nil
}

// GetBreakpoint 返回暂停弧线的断点及其前后节点：
//   - before: 最近 2 个已完成/废弃节点（target_chapter 升序）
//   - pending: 断点 + 下一个 pending（最多 2 个，pending[0] 为断点）
func (s *Store) GetBreakpoint(ctx context.Context, arcID int64) (before []ArcNode, pending []ArcNode, err error) {
	var pendings []ArcNode
	if err := s.DB.WithContext(ctx).
		Where("story_arc_id = ? AND status = ?", arcID, "pending").
		Order("target_chapter ASC, id ASC").
		Limit(2).
		Find(&pendings).Error; err != nil {
		return nil, nil, fmt.Errorf("storyarc store: breakpoint pending: %w", err)
	}

	// 取最近 2 个已完成/废弃节点，倒序后翻转回时间升序
	var beforeDesc []ArcNode
	if err := s.DB.WithContext(ctx).
		Where("story_arc_id = ? AND status IN ?", arcID, []string{"completed", "abandoned"}).
		Order("target_chapter DESC, id DESC").
		Limit(2).
		Find(&beforeDesc).Error; err != nil {
		return nil, nil, fmt.Errorf("storyarc store: breakpoint before: %w", err)
	}
	for i := len(beforeDesc) - 1; i >= 0; i-- {
		before = append(before, beforeDesc[i])
	}

	return before, pendings, nil
}

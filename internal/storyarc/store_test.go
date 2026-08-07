package storyarc

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/storage"
)

func openArcDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&StoryArc{}, &ArcNode{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func testArcLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestArcListByNovel(t *testing.T) {
	db := openArcDB(t)
	s := NewStore(db, testArcLogger())
	ctx := context.Background()

	db.Create(&StoryArc{NovelID: 1, Name: "主线", ArcType: "main", Status: "active", Importance: 5})
	db.Create(&StoryArc{NovelID: 1, Name: "支线", ArcType: "sub", Status: "paused", Importance: 3})

	result, _ := s.ListByNovel(ctx, 1, ListByNovelOptions{})
	if result.Total != 2 {
		t.Errorf("expected 2, got %d", result.Total)
	}
}

func TestArcListByNovel_Filter(t *testing.T) {
	db := openArcDB(t)
	s := NewStore(db, testArcLogger())
	ctx := context.Background()

	db.Create(&StoryArc{NovelID: 1, Name: "主线", ArcType: "main", Status: "active", Importance: 5})
	db.Create(&StoryArc{NovelID: 1, Name: "支线", ArcType: "sub", Status: "completed", Importance: 2})

	result, _ := s.ListByNovel(ctx, 1, ListByNovelOptions{Status: "active"})
	if result.Total != 1 {
		t.Errorf("filter active: expected 1, got %d", result.Total)
	}
}

func TestArcListNonArchived(t *testing.T) {
	db := openArcDB(t)
	s := NewStore(db, testArcLogger())
	ctx := context.Background()

	db.Create(&StoryArc{NovelID: 1, Name: "A", Status: "active", Importance: 1})
	db.Create(&StoryArc{NovelID: 1, Name: "B", Status: "paused", Importance: 1})
	db.Create(&StoryArc{NovelID: 1, Name: "C", Status: "completed", Importance: 1})
	db.Create(&StoryArc{NovelID: 1, Name: "D", Status: "abandoned", Importance: 1})

	arcs, _ := s.ListNonArchived(ctx, 1)
	if len(arcs) != 2 {
		t.Errorf("expected 2 non-archived (active+paused), got %d", len(arcs))
	}
}

func TestArcListByArcs(t *testing.T) {
	db := openArcDB(t)
	s := NewStore(db, testArcLogger())
	ctx := context.Background()

	arc := StoryArc{NovelID: 1, Name: "主线", Status: "active", Importance: 5}
	db.Create(&arc)
	db.Create(&ArcNode{NovelID: 1, StoryArcID: arc.ID, Title: "节点1", TargetChapter: 5, Status: "pending"})
	db.Create(&ArcNode{NovelID: 1, StoryArcID: arc.ID, Title: "节点2", TargetChapter: 10, Status: "pending"})

	nodes, _ := s.ListByArcs(ctx, []int64{arc.ID})
	if len(nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(nodes))
	}
}

func TestArcListNodesByNovel(t *testing.T) {
	db := openArcDB(t)
	s := NewStore(db, testArcLogger())
	ctx := context.Background()

	db.Create(&ArcNode{NovelID: 1, StoryArcID: 1, Title: "早", TargetChapter: 5, Status: "pending"})
	db.Create(&ArcNode{NovelID: 1, StoryArcID: 1, Title: "中", TargetChapter: 10, Status: "pending"})
	db.Create(&ArcNode{NovelID: 1, StoryArcID: 1, Title: "晚", TargetChapter: 15, Status: "pending"})
	db.Create(&ArcNode{NovelID: 2, StoryArcID: 1, Title: "其他小说", TargetChapter: 1, Status: "pending"})

	result, _ := s.ListNodesByNovel(ctx, 1, ListNodesOptions{
		PageParams: storage.PageParams{Size: -1},
	})
	if result.Total != 3 {
		t.Errorf("expected 3 nodes for novel 1, got %d", result.Total)
	}
}

func TestArcNodesBeforeByArc(t *testing.T) {
	db := openArcDB(t)
	s := NewStore(db, testArcLogger())
	ctx := context.Background()

	arc := StoryArc{NovelID: 1, Name: "主线", Status: "active", Importance: 5}
	db.Create(&arc)
	for i := 1; i <= 10; i++ {
		db.Create(&ArcNode{NovelID: 1, StoryArcID: arc.ID, Title: "n", TargetChapter: i, Status: "pending"})
	}

	result, _ := s.ListNodesBeforeByArc(ctx, []int64{arc.ID}, 6, 3)
	nodes := result[arc.ID]
	if len(nodes) != 3 {
		t.Errorf("expected 3 before ch6, got %d", len(nodes))
	}
}

func TestArcPendingNodesBeforeByArc(t *testing.T) {
	db := openArcDB(t)
	s := NewStore(db, testArcLogger())
	ctx := context.Background()

	arc := StoryArc{NovelID: 1, Name: "主线", Status: "active", Importance: 5}
	db.Create(&arc)
	db.Create(&ArcNode{NovelID: 1, StoryArcID: arc.ID, Title: "pending", TargetChapter: 3, Status: "pending"})
	db.Create(&ArcNode{NovelID: 1, StoryArcID: arc.ID, Title: "done", TargetChapter: 5, Status: "completed"})
	db.Create(&ArcNode{NovelID: 1, StoryArcID: arc.ID, Title: "future", TargetChapter: 10, Status: "pending"})

	result, _ := s.ListPendingNodesBeforeByArc(ctx, []int64{arc.ID}, 8)
	if len(result[arc.ID]) != 1 {
		t.Errorf("expected 1 pending before ch8, got %d", len(result[arc.ID]))
	}
}

func TestArcNodesAfterByArc(t *testing.T) {
	db := openArcDB(t)
	s := NewStore(db, testArcLogger())
	ctx := context.Background()

	arc := StoryArc{NovelID: 1, Name: "主线", Status: "active", Importance: 5}
	db.Create(&arc)
	db.Create(&ArcNode{NovelID: 1, StoryArcID: arc.ID, Title: "past", TargetChapter: 5, Status: "pending"})
	db.Create(&ArcNode{NovelID: 1, StoryArcID: arc.ID, Title: "now", TargetChapter: 10, Status: "pending"})

	result, _ := s.ListNodesAfterByArc(ctx, []int64{arc.ID}, 10)
	if len(result[arc.ID]) != 1 {
		t.Errorf("expected 1 at ch>=10, got %d", len(result[arc.ID]))
	}
}

func TestArcGetBreakpoint(t *testing.T) {
	db := openArcDB(t)
	s := NewStore(db, testArcLogger())
	ctx := context.Background()

	arc := StoryArc{NovelID: 1, Name: "暂停弧", Status: "paused", Importance: 3}
	db.Create(&arc)
	db.Create(&ArcNode{NovelID: 1, StoryArcID: arc.ID, Title: "完成1", TargetChapter: 3, Status: "completed"})
	db.Create(&ArcNode{NovelID: 1, StoryArcID: arc.ID, Title: "完成2", TargetChapter: 5, Status: "completed"})
	db.Create(&ArcNode{NovelID: 1, StoryArcID: arc.ID, Title: "断点", TargetChapter: 8, Status: "pending"})
	db.Create(&ArcNode{NovelID: 1, StoryArcID: arc.ID, Title: "下一个", TargetChapter: 12, Status: "pending"})

	before, pending, err := s.GetBreakpoint(ctx, arc.ID)
	if err != nil {
		t.Fatalf("GetBreakpoint: %v", err)
	}
	if len(before) != 2 {
		t.Errorf("expected 2 completed before, got %d", len(before))
	}
	if len(pending) != 2 {
		t.Errorf("expected 2 pending (breakpoint + next), got %d", len(pending))
	}
}

// ── CRUD ────────────────────────────────────────────────────

func TestArcCreate(t *testing.T) {
	db := openArcDB(t)
	ctx := context.Background()

	arc := StoryArc{NovelID: 1, Name: "主线", ArcType: "main", Description: "描述", Importance: 5, Status: "active"}
	if err := db.WithContext(ctx).Create(&arc).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	if arc.ID == 0 {
		t.Error("ID should be set after create")
	}

	var found StoryArc
	db.First(&found, arc.ID)
	if found.Name != "主线" {
		t.Errorf("expected 主线, got %s", found.Name)
	}
	if found.Status != "active" {
		t.Errorf("expected active, got %s", found.Status)
	}
}

func TestArcUpdate(t *testing.T) {
	db := openArcDB(t)
	ctx := context.Background()

	arc := StoryArc{NovelID: 1, Name: "旧弧线", ArcType: "sub", Status: "active", Importance: 3}
	db.WithContext(ctx).Create(&arc)

	type UpdateInput struct {
		Name   string `json:"name,omitempty"`
		Status string `json:"status,omitempty"`
	}
	input := UpdateInput{Status: "paused", Name: ""}
	if err := db.WithContext(ctx).Model(&StoryArc{}).Where("id = ?", arc.ID).Updates(&input).Error; err != nil {
		t.Fatalf("update: %v", err)
	}

	var updated StoryArc
	db.WithContext(ctx).First(&updated, arc.ID)
	if updated.Status != "paused" {
		t.Errorf("status: expected paused, got %s", updated.Status)
	}
	if updated.Name != "旧弧线" {
		t.Errorf("name should be unchanged (empty string skipped), got %s", updated.Name)
	}
}

func TestArcDelete(t *testing.T) {
	db := openArcDB(t)
	ctx := context.Background()

	arc := StoryArc{NovelID: 1, Name: "待删弧线", ArcType: "sub", Status: "active", Importance: 2}
	db.WithContext(ctx).Create(&arc)
	db.WithContext(ctx).Create(&ArcNode{NovelID: 1, StoryArcID: arc.ID, Title: "节点1", TargetChapter: 3, Status: "pending"})
	db.WithContext(ctx).Create(&ArcNode{NovelID: 1, StoryArcID: arc.ID, Title: "节点2", TargetChapter: 7, Status: "pending"})

	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("story_arc_id = ?", arc.ID).Delete(&ArcNode{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", arc.ID).Delete(&StoryArc{}).Error
	})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}

	var found StoryArc
	if db.First(&found, arc.ID).Error == nil {
		t.Error("arc should be deleted")
	}
	var nodeCount int64
	db.Model(&ArcNode{}).Where("story_arc_id = ?", arc.ID).Count(&nodeCount)
	if nodeCount != 0 {
		t.Errorf("nodes should be cascade-deleted, got %d", nodeCount)
	}
}

// ── ArcNode CRUD ────────────────────────────────────────────

func TestArcNodeCreate(t *testing.T) {
	db := openArcDB(t)
	ctx := context.Background()

	node := ArcNode{NovelID: 1, StoryArcID: 1, Title: "关键节点", TargetChapter: 5, Status: "pending"}
	if err := db.WithContext(ctx).Create(&node).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	if node.ID == 0 {
		t.Error("ID should be set after create")
	}

	var found ArcNode
	db.First(&found, node.ID)
	if found.Title != "关键节点" {
		t.Errorf("expected 关键节点, got %s", found.Title)
	}
	if found.Status != "pending" {
		t.Errorf("expected pending, got %s", found.Status)
	}
}

func TestArcNodeUpdate(t *testing.T) {
	db := openArcDB(t)
	ctx := context.Background()

	node := ArcNode{NovelID: 1, StoryArcID: 1, Title: "旧节点", TargetChapter: 3, Status: "pending"}
	db.WithContext(ctx).Create(&node)

	type UpdateInput struct {
		Title         string `json:"title,omitempty"`
		Status        string `json:"status,omitempty"`
		ActualChapter int    `json:"actual_chapter,omitempty"`
	}
	input := UpdateInput{Status: "completed", ActualChapter: 3}
	if err := db.WithContext(ctx).Model(&ArcNode{}).Where("id = ?", node.ID).Updates(&input).Error; err != nil {
		t.Fatalf("update: %v", err)
	}

	var updated ArcNode
	db.WithContext(ctx).First(&updated, node.ID)
	if updated.Status != "completed" {
		t.Errorf("status: expected completed, got %s", updated.Status)
	}
	if updated.ActualChapter != 3 {
		t.Errorf("actual_chapter: expected 3, got %d", updated.ActualChapter)
	}
}

func TestArcNodeDelete(t *testing.T) {
	db := openArcDB(t)
	ctx := context.Background()

	node := ArcNode{NovelID: 1, StoryArcID: 1, Title: "待删节点", TargetChapter: 5, Status: "pending"}
	db.WithContext(ctx).Create(&node)

	if err := db.WithContext(ctx).Where("id = ?", node.ID).Delete(&ArcNode{}).Error; err != nil {
		t.Fatalf("delete: %v", err)
	}

	var found ArcNode
	if db.First(&found, node.ID).Error == nil {
		t.Error("node should be deleted")
	}
}

// ── 4b: Search / Order ─────────────────────────────────

func TestArcListByNovel_Search(t *testing.T) {
	db := openArcDB(t)
	s := NewStore(db, testArcLogger())
	ctx := context.Background()

	db.Create(&StoryArc{NovelID: 1, Name: "复仇之路", ArcType: "main", Status: "active", Importance: 5})
	db.Create(&StoryArc{NovelID: 1, Name: "爱情线", ArcType: "sub", Status: "active", Importance: 3, Description: "副线发展"})
	db.Create(&StoryArc{NovelID: 1, Name: "背景线", ArcType: "background", Status: "active", Importance: 1})

	// 搜 name（只匹配 name，description 不含"复仇"）
	r, _ := s.ListByNovel(ctx, 1, ListByNovelOptions{Search: "复仇"})
	if r.Total != 1 {
		t.Errorf("search name 复仇: expected 1, got %d", r.Total)
	}

	// 搜 description
	r, _ = s.ListByNovel(ctx, 1, ListByNovelOptions{Search: "副线"})
	if r.Total != 1 {
		t.Errorf("search description 副线: expected 1, got %d", r.Total)
	}

	// 无命中
	r, _ = s.ListByNovel(ctx, 1, ListByNovelOptions{Search: "不存在"})
	if r.Total != 0 {
		t.Errorf("search no match: expected 0, got %d", r.Total)
	}
}

func TestArcListByNovel_Order(t *testing.T) {
	db := openArcDB(t)
	s := NewStore(db, testArcLogger())
	ctx := context.Background()

	db.Create(&StoryArc{NovelID: 1, Name: "B", ArcType: "main", Status: "active", Importance: 3})
	db.Create(&StoryArc{NovelID: 1, Name: "A", ArcType: "main", Status: "active", Importance: 5})
	db.Create(&StoryArc{NovelID: 1, Name: "C", ArcType: "main", Status: "active", Importance: 1})

	// 默认排序：importance DESC
	r, _ := s.ListByNovel(ctx, 1, ListByNovelOptions{
		PageParams: storage.PageParams{Size: -1},
	})
	if r.Items[0].Name != "A" || r.Items[2].Name != "C" {
		t.Errorf("default order importance DESC: expected A,C, got %s,%s", r.Items[0].Name, r.Items[2].Name)
	}

	// 显式 Order: name ASC
	r, _ = s.ListByNovel(ctx, 1, ListByNovelOptions{
		PageParams: storage.PageParams{Size: -1},
		Order:      "name ASC",
	})
	if r.Items[0].Name != "A" || r.Items[1].Name != "B" || r.Items[2].Name != "C" {
		t.Errorf("order name ASC: expected A,B,C, got %s,%s,%s", r.Items[0].Name, r.Items[1].Name, r.Items[2].Name)
	}
}

func TestArcListNodesByNovel_Search(t *testing.T) {
	db := openArcDB(t)
	s := NewStore(db, testArcLogger())
	ctx := context.Background()

	db.Create(&ArcNode{NovelID: 1, StoryArcID: 1, Title: "发现真相", TargetChapter: 5, Status: "pending"})
	db.Create(&ArcNode{NovelID: 1, StoryArcID: 1, Title: "决战", TargetChapter: 10, Status: "pending", Description: "最终对决"})
	db.Create(&ArcNode{NovelID: 1, StoryArcID: 1, Title: "尾声", TargetChapter: 15, Status: "pending"})

	// 搜 title（只匹配 title，description 不含"真相"）
	r, _ := s.ListNodesByNovel(ctx, 1, ListNodesOptions{Search: "真相"})
	if r.Total != 1 {
		t.Errorf("search title 真相: expected 1, got %d", r.Total)
	}

	// 搜 description
	r, _ = s.ListNodesByNovel(ctx, 1, ListNodesOptions{Search: "对决"})
	if r.Total != 1 {
		t.Errorf("search description 对决: expected 1, got %d", r.Total)
	}

	// 无命中
	r, _ = s.ListNodesByNovel(ctx, 1, ListNodesOptions{Search: "不存在"})
	if r.Total != 0 {
		t.Errorf("search no match: expected 0, got %d", r.Total)
	}
}

func TestArcListNodesByNovel_Order(t *testing.T) {
	db := openArcDB(t)
	s := NewStore(db, testArcLogger())
	ctx := context.Background()

	db.Create(&ArcNode{NovelID: 1, StoryArcID: 1, Title: "C", TargetChapter: 15, Status: "pending"})
	db.Create(&ArcNode{NovelID: 1, StoryArcID: 1, Title: "A", TargetChapter: 5, Status: "pending"})
	db.Create(&ArcNode{NovelID: 1, StoryArcID: 1, Title: "B", TargetChapter: 10, Status: "pending"})

	// 默认排序：target_chapter ASC
	r, _ := s.ListNodesByNovel(ctx, 1, ListNodesOptions{
		PageParams: storage.PageParams{Size: -1},
	})
	if r.Items[0].Title != "A" || r.Items[2].Title != "C" {
		t.Errorf("default order target_chapter ASC: expected A,C, got %s,%s", r.Items[0].Title, r.Items[2].Title)
	}

	// 显式 Order: title DESC
	r, _ = s.ListNodesByNovel(ctx, 1, ListNodesOptions{
		PageParams: storage.PageParams{Size: -1},
		Order:      "title DESC",
	})
	if r.Items[0].Title != "C" || r.Items[2].Title != "A" {
		t.Errorf("order title DESC: expected C,A, got %s,%s", r.Items[0].Title, r.Items[2].Title)
	}
}

package timeline

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/storage"
)

func openTlDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&TimelineEntry{}, &ChapterPlan{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func testTlLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// TestTlListByNovel_All 验证 Size=-1 全量拉取（替代废弃的 ListByChapterRange(0,0)）。
// 排序按默认 target_chapter ASC, importance DESC。
func TestTlListByNovel_All(t *testing.T) {
	db := openTlDB(t)
	s := NewStore(db, testTlLogger())
	ctx := context.Background()

	db.Create(&TimelineEntry{NovelID: 1, Title: "伏笔A", TargetChapter: 10, Category: "foreshadowing", Status: "pending", Importance: 3})
	db.Create(&TimelineEntry{NovelID: 1, Title: "伏笔B", TargetChapter: 20, Category: "foreshadowing", Status: "pending", Importance: 5})
	db.Create(&TimelineEntry{NovelID: 1, Title: "伏笔C", TargetChapter: 30, Category: "foreshadowing", Status: "pending", Importance: 1})
	db.Create(&TimelineEntry{NovelID: 2, Title: "其他小说", TargetChapter: 1, Status: "pending"})

	result, _ := s.ListByNovel(ctx, 1, ListByNovelOptions{
		PageParams: storage.PageParams{Size: -1},
	})
	if result.Total != 3 {
		t.Fatalf("expected 3 entries for novel 1, got %d", result.Total)
	}
	// 默认排序 target_chapter ASC
	if result.Items[0].Title != "伏笔A" || result.Items[2].Title != "伏笔C" {
		t.Errorf("default order target_chapter ASC: expected 伏笔A,伏笔C, got %s,%s",
			result.Items[0].Title, result.Items[2].Title)
	}
}

func TestTlListByNovel_Filter(t *testing.T) {
	db := openTlDB(t)
	s := NewStore(db, testTlLogger())
	ctx := context.Background()

	db.Create(&TimelineEntry{NovelID: 1, Title: "伏笔", Category: "foreshadowing", Status: "pending"})
	db.Create(&TimelineEntry{NovelID: 1, Title: "指令", Category: "user_directive", Status: "resolved"})

	result, _ := s.ListByNovel(ctx, 1, ListByNovelOptions{Category: "foreshadowing"})
	if result.Total != 1 {
		t.Errorf("filter by category: expected 1, got %d", result.Total)
	}

	result2, _ := s.ListByNovel(ctx, 1, ListByNovelOptions{Status: "pending"})
	if result2.Total != 1 {
		t.Errorf("filter by status: expected 1, got %d", result2.Total)
	}
}

func TestTlListBefore(t *testing.T) {
	db := openTlDB(t)
	s := NewStore(db, testTlLogger())
	ctx := context.Background()

	for i := 1; i <= 10; i++ {
		db.Create(&TimelineEntry{NovelID: 1, Title: "e", TargetChapter: i, Status: "pending"})
	}

	result, _ := s.ListBefore(ctx, 1, 6, 3)
	if len(result) != 3 {
		t.Errorf("expected 3, got %d", len(result))
	}
	for _, e := range result {
		if e.TargetChapter >= 6 {
			t.Errorf("all should be < 6, got target=%d", e.TargetChapter)
		}
	}
}

func TestTlListPendingBefore(t *testing.T) {
	db := openTlDB(t)
	s := NewStore(db, testTlLogger())
	ctx := context.Background()

	db.Create(&TimelineEntry{NovelID: 1, Title: "pending", TargetChapter: 5, Status: "pending"})
	db.Create(&TimelineEntry{NovelID: 1, Title: "resolved", TargetChapter: 3, Status: "resolved"})
	db.Create(&TimelineEntry{NovelID: 1, Title: "future", TargetChapter: 10, Status: "pending"})

	result, _ := s.ListPendingBefore(ctx, 1, 8)
	if len(result) != 1 {
		t.Errorf("expected 1 pending before ch8, got %d", len(result))
	}
}

func TestTlListAfter(t *testing.T) {
	db := openTlDB(t)
	s := NewStore(db, testTlLogger())
	ctx := context.Background()

	db.Create(&TimelineEntry{NovelID: 1, Title: "past", TargetChapter: 5, Status: "pending"})
	db.Create(&TimelineEntry{NovelID: 1, Title: "now", TargetChapter: 10, Status: "pending"})
	db.Create(&TimelineEntry{NovelID: 1, Title: "future", TargetChapter: 15, Status: "pending"})

	result, _ := s.ListAfter(ctx, 1, 10)
	if len(result) != 2 {
		t.Errorf("expected 2 with target >= 10, got %d", len(result))
	}
}

func TestTlListByNovel_Pagination(t *testing.T) {
	db := openTlDB(t)
	s := NewStore(db, testTlLogger())
	ctx := context.Background()

	for i := 1; i <= 5; i++ {
		db.Create(&TimelineEntry{NovelID: 1, Title: "e", TargetChapter: i, Status: "pending"})
	}

	result, _ := s.ListByNovel(ctx, 1, ListByNovelOptions{
		PageParams: storage.PageParams{Page: 1, Size: 2},
	})
	if len(result.Items) != 2 {
		t.Errorf("page 1: expected 2, got %d", len(result.Items))
	}
	if result.Total != 5 {
		t.Errorf("total should be 5, got %d", result.Total)
	}
}

// ── CRUD ────────────────────────────────────────────────────

func TestTlCreateEntry(t *testing.T) {
	db := openTlDB(t)
	ctx := context.Background()

	entry := TimelineEntry{
		NovelID: 1, Title: "伏笔A", Category: "foreshadowing",
		TargetChapter: 10, Importance: 5, Status: "pending",
		DetailJSON: `{"key":"val"}`,
	}
	if err := db.WithContext(ctx).Create(&entry).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	if entry.ID == 0 {
		t.Error("ID should be set after create")
	}

	var found TimelineEntry
	db.First(&found, entry.ID)
	if found.Title != "伏笔A" {
		t.Errorf("expected 伏笔A, got %s", found.Title)
	}
	if found.Status != "pending" {
		t.Errorf("expected pending, got %s", found.Status)
	}
}

func TestTlUpdateEntry(t *testing.T) {
	db := openTlDB(t)
	ctx := context.Background()

	entry := TimelineEntry{NovelID: 1, Title: "旧伏笔", Content: "旧内容", Status: "pending", TargetChapter: 5}
	db.WithContext(ctx).Create(&entry)

	type UpdateInput struct {
		Title   string `json:"title,omitempty"`
		Content string `json:"content,omitempty"`
		Status  string `json:"status,omitempty"`
	}
	input := UpdateInput{Status: "resolved", Content: ""}
	if err := db.WithContext(ctx).Model(&TimelineEntry{}).Where("id = ?", entry.ID).Updates(&input).Error; err != nil {
		t.Fatalf("update: %v", err)
	}

	var updated TimelineEntry
	db.WithContext(ctx).First(&updated, entry.ID)
	if updated.Status != "resolved" {
		t.Errorf("status: expected resolved, got %s", updated.Status)
	}
	if updated.Content != "旧内容" {
		t.Errorf("content should be unchanged (empty string skipped), got %s", updated.Content)
	}
}

func TestTlDeleteEntry(t *testing.T) {
	db := openTlDB(t)
	ctx := context.Background()

	entry := TimelineEntry{NovelID: 1, Title: "待删伏笔", Category: "foreshadowing", TargetChapter: 3, Status: "pending"}
	db.WithContext(ctx).Create(&entry)

	if err := db.WithContext(ctx).Where("id = ?", entry.ID).Delete(&TimelineEntry{}).Error; err != nil {
		t.Fatalf("delete: %v", err)
	}

	var found TimelineEntry
	if db.First(&found, entry.ID).Error == nil {
		t.Error("entry should be deleted")
	}
}

// ── 4b: Search / Order ─────────────────────────────────

func TestTlListByNovel_Search(t *testing.T) {
	db := openTlDB(t)
	s := NewStore(db, testTlLogger())
	ctx := context.Background()

	db.Create(&TimelineEntry{NovelID: 1, Title: "复仇", TargetChapter: 5, Category: "foreshadowing", Status: "pending"})
	db.Create(&TimelineEntry{NovelID: 1, Title: "决战", Content: "复仇的高潮", TargetChapter: 10, Status: "pending"})
	db.Create(&TimelineEntry{NovelID: 1, Title: "尾声", TargetChapter: 15, Status: "pending"})

	// 搜 title（"复仇"命中 title 字段一次）
	r, _ := s.ListByNovel(ctx, 1, ListByNovelOptions{Search: "复仇"})
	if r.Total != 2 {
		t.Errorf("search 复仇: expected 2 (title+content), got %d", r.Total)
	}

	// 搜 content
	r, _ = s.ListByNovel(ctx, 1, ListByNovelOptions{Search: "高潮"})
	if r.Total != 1 {
		t.Errorf("search content 高潮: expected 1, got %d", r.Total)
	}

	// 无命中
	r, _ = s.ListByNovel(ctx, 1, ListByNovelOptions{Search: "不存在"})
	if r.Total != 0 {
		t.Errorf("search no match: expected 0, got %d", r.Total)
	}
}

func TestTlListByNovel_Order(t *testing.T) {
	db := openTlDB(t)
	s := NewStore(db, testTlLogger())
	ctx := context.Background()

	db.Create(&TimelineEntry{NovelID: 1, Title: "C", TargetChapter: 15, Importance: 1, Status: "pending"})
	db.Create(&TimelineEntry{NovelID: 1, Title: "A", TargetChapter: 5, Importance: 5, Status: "pending"})
	db.Create(&TimelineEntry{NovelID: 1, Title: "B", TargetChapter: 10, Importance: 3, Status: "pending"})

	// 默认排序：target_chapter ASC, importance DESC
	r, _ := s.ListByNovel(ctx, 1, ListByNovelOptions{
		PageParams: storage.PageParams{Size: -1},
	})
	if r.Items[0].Title != "A" || r.Items[2].Title != "C" {
		t.Errorf("default order: expected A,C, got %s,%s", r.Items[0].Title, r.Items[2].Title)
	}

	// 显式 Order: title DESC
	r, _ = s.ListByNovel(ctx, 1, ListByNovelOptions{
		PageParams: storage.PageParams{Size: -1},
		Order:      "title DESC",
	})
	if r.Items[0].Title != "C" || r.Items[2].Title != "A" {
		t.Errorf("order title DESC: expected C,A, got %s,%s", r.Items[0].Title, r.Items[2].Title)
	}
}

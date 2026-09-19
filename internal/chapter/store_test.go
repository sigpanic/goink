package chapter

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/storage"
	"github.com/sigpanic/goink/internal/volume"
)

func openChDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&Chapter{}, &volume.Volume{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func testChLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestChListAllByNovel(t *testing.T) {
	db := openChDB(t)
	s := NewStore(db, testChLogger())
	ctx := context.Background()

	db.Create(&Chapter{NovelID: 1, ChapterNumber: 1, SortOrder: 2, Title: "开头"})
	db.Create(&Chapter{NovelID: 1, ChapterNumber: 2, SortOrder: 1, Title: "发展"})
	db.Create(&Chapter{NovelID: 2, ChapterNumber: 1, SortOrder: 1, Title: "另一部"})

	chapters, _ := s.ListAllByNovel(ctx, 1)
	if len(chapters) != 2 {
		t.Errorf("expected 2, got %d", len(chapters))
	}
	if chapters[0].ChapterNumber != 2 {
		t.Errorf("expected sort_order-first chapter 2, got %d", chapters[0].ChapterNumber)
	}
}

func TestChListAllByNovelOrdersVolumesThenUnassigned(t *testing.T) {
	db := openChDB(t)
	s := NewStore(db, testChLogger())
	ctx := context.Background()

	v1 := volume.Volume{NovelID: 1, Name: "第一卷", SortOrder: 2}
	v2 := volume.Volume{NovelID: 1, Name: "第二卷", SortOrder: 1}
	if err := db.Create(&v1).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&v2).Error; err != nil {
		t.Fatal(err)
	}
	db.Create(&Chapter{NovelID: 1, ChapterNumber: 1, SortOrder: 1})
	db.Create(&Chapter{NovelID: 1, ChapterNumber: 2, VolumeID: &v1.ID, SortOrder: 1})
	db.Create(&Chapter{NovelID: 1, ChapterNumber: 3, VolumeID: &v2.ID, SortOrder: 2})
	db.Create(&Chapter{NovelID: 1, ChapterNumber: 4, VolumeID: &v2.ID, SortOrder: 1})

	chapters, err := s.ListAllByNovel(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	want := []int{4, 3, 2, 1}
	for i, ch := range chapters {
		if ch.ChapterNumber != want[i] {
			t.Errorf("index %d chapter_number = %d, want %d", i, ch.ChapterNumber, want[i])
		}
	}
}

func TestChListByNovel_Desc(t *testing.T) {
	db := openChDB(t)
	s := NewStore(db, testChLogger())
	ctx := context.Background()

	db.Create(&Chapter{NovelID: 1, ChapterNumber: 1, SortOrder: 2})
	db.Create(&Chapter{NovelID: 1, ChapterNumber: 2, SortOrder: 1})

	result, _ := s.ListByNovel(ctx, 1, ListByNovelOptions{Order: "desc", PageParams: storage.PageParams{Size: -1}})
	if result.Items[0].ChapterNumber != 1 {
		t.Errorf("desc: expected chapter with greatest sort_order first, got %d", result.Items[0].ChapterNumber)
	}
}

func TestChGetByNovelAndNumber(t *testing.T) {
	db := openChDB(t)
	s := NewStore(db, testChLogger())
	ctx := context.Background()

	db.Create(&Chapter{NovelID: 1, ChapterNumber: 3, Title: "高潮"})

	ch, err := s.GetByNovelAndNumber(ctx, 1, 3)
	if err != nil {
		t.Fatalf("GetByNovelAndNumber: %v", err)
	}
	if ch.Title != "高潮" {
		t.Errorf("expected 高潮, got %s", ch.Title)
	}
}

func TestChGetByNovelAndNumber_NotFound(t *testing.T) {
	db := openChDB(t)
	s := NewStore(db, testChLogger())
	ctx := context.Background()

	_, err := s.GetByNovelAndNumber(ctx, 1, 999)
	if err == nil {
		t.Error("expected error for not found")
	}
}

func TestChGetLatestNumber(t *testing.T) {
	db := openChDB(t)
	s := NewStore(db, testChLogger())
	ctx := context.Background()

	db.Create(&Chapter{NovelID: 1, ChapterNumber: 5})
	db.Create(&Chapter{NovelID: 1, ChapterNumber: 3})

	n, _ := s.GetLatestNumber(ctx, 1)
	if n != 5 {
		t.Errorf("expected 5, got %d", n)
	}
}

func TestChGetLatestNumber_Empty(t *testing.T) {
	db := openChDB(t)
	s := NewStore(db, testChLogger())
	ctx := context.Background()

	n, _ := s.GetLatestNumber(ctx, 1)
	if n != 0 {
		t.Errorf("expected 0 for empty, got %d", n)
	}
}

func TestChCreateAppendsToExplicitVolume(t *testing.T) {
	db := openChDB(t)
	s := NewStore(db, testChLogger())
	ctx := context.Background()

	v := volume.Volume{NovelID: 1, Name: "第一卷", SortOrder: 1}
	if err := db.Create(&v).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&Chapter{NovelID: 1, ChapterNumber: 1, VolumeID: &v.ID, SortOrder: 1}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&Chapter{NovelID: 1, ChapterNumber: 2, SortOrder: 9}).Error; err != nil {
		t.Fatal(err)
	}

	created, err := s.Create(ctx, nil, 1, &v.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if created.VolumeID == nil || *created.VolumeID != v.ID {
		t.Errorf("volume_id = %v, want %d", created.VolumeID, v.ID)
	}
	if created.SortOrder != 2 || created.ChapterNumber != 3 || created.Title != "第3章" {
		t.Errorf("created = %+v, want volume sort/number/title = %d/3/第3章", created, 2)
	}
}

func TestChCreateAppendsToUnassignedGroup(t *testing.T) {
	db := openChDB(t)
	s := NewStore(db, testChLogger())
	ctx := context.Background()

	v := volume.Volume{NovelID: 1, Name: "第一卷", SortOrder: 1}
	if err := db.Create(&v).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&Chapter{NovelID: 1, ChapterNumber: 1, VolumeID: &v.ID, SortOrder: 100}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&Chapter{NovelID: 1, ChapterNumber: 2, SortOrder: 2}).Error; err != nil {
		t.Fatal(err)
	}

	created, err := s.Create(ctx, nil, 1, nil, "未分卷新章")
	if err != nil {
		t.Fatal(err)
	}
	if created.VolumeID != nil || created.SortOrder != 3 || created.ChapterNumber != 3 {
		t.Errorf("created = %+v, want unassigned sort/number = 3/3", created)
	}
}

func TestChCreateRejectsForeignVolume(t *testing.T) {
	db := openChDB(t)
	s := NewStore(db, testChLogger())
	ctx := context.Background()

	v := volume.Volume{NovelID: 2, Name: "别家卷", SortOrder: 1}
	if err := db.Create(&v).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(ctx, nil, 1, &v.ID, "新章"); !errors.Is(err, volume.ErrNotFound) {
		t.Errorf("err = %v, want volume.ErrNotFound", err)
	}

	var count int64
	if err := db.Model(&Chapter{}).Where("novel_id = ?", 1).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("chapter count = %d, want 0", count)
	}
}

func TestChGetRecent(t *testing.T) {
	db := openChDB(t)
	s := NewStore(db, testChLogger())
	ctx := context.Background()

	for num, sort := range map[int]int{1: 5, 2: 1, 3: 4, 4: 2, 5: 3} {
		db.Create(&Chapter{NovelID: 1, ChapterNumber: num, SortOrder: sort})
	}

	recent, _ := s.GetRecent(ctx, 1, 2)
	if len(recent) != 2 {
		t.Fatalf("expected 2, got %d", len(recent))
	}
	if recent[0].ChapterNumber != 1 || recent[1].ChapterNumber != 3 {
		t.Errorf("recent = %d/%d, want chapters 1/3 by descending sort_order", recent[0].ChapterNumber, recent[1].ChapterNumber)
	}
}

func TestChGetRecentUsesReverseCompositeReadingOrder(t *testing.T) {
	db := openChDB(t)
	s := NewStore(db, testChLogger())
	ctx := context.Background()

	v1 := volume.Volume{NovelID: 1, Name: "第一卷", SortOrder: 1}
	v2 := volume.Volume{NovelID: 1, Name: "第二卷", SortOrder: 2}
	if err := db.Create(&v1).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&v2).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&Chapter{NovelID: 1, ChapterNumber: 1, VolumeID: &v1.ID, SortOrder: 1}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&Chapter{NovelID: 1, ChapterNumber: 2, VolumeID: &v2.ID, SortOrder: 1}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&Chapter{NovelID: 1, ChapterNumber: 3, SortOrder: 1}).Error; err != nil {
		t.Fatal(err)
	}

	recent, err := s.GetRecent(ctx, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	want := []int{3, 2}
	for i, ch := range recent {
		if ch.ChapterNumber != want[i] {
			t.Errorf("index %d chapter_number = %d, want %d", i, ch.ChapterNumber, want[i])
		}
	}
}

func TestChSearchByNovelOrdersBySortOrder(t *testing.T) {
	db := openChDB(t)
	s := NewStore(db, testChLogger())
	ctx := context.Background()

	db.Create(&Chapter{NovelID: 1, ChapterNumber: 1, SortOrder: 2, Title: "相同"})
	db.Create(&Chapter{NovelID: 1, ChapterNumber: 2, SortOrder: 1, Title: "相同"})

	chapters, err := s.SearchByNovel(ctx, 1, "相同", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 2 || chapters[0].ChapterNumber != 2 {
		t.Errorf("search results = %+v, want chapter 2 first", chapters)
	}
}

func TestListByNovel_Pagination(t *testing.T) {
	db := openChDB(t)
	s := NewStore(db, testChLogger())
	ctx := context.Background()

	for i := 1; i <= 10; i++ {
		db.Create(&Chapter{NovelID: 1, ChapterNumber: i, SortOrder: 11 - i})
	}

	result, _ := s.ListByNovel(ctx, 1, ListByNovelOptions{
		PageParams: storage.PageParams{Page: 2, Size: 3},
	})
	if result.Page != 2 {
		t.Errorf("expected page 2, got %d", result.Page)
	}
	if len(result.Items) != 3 {
		t.Errorf("expected 3 items, got %d", len(result.Items))
	}
	want := []int{7, 6, 5}
	for i, ch := range result.Items {
		if ch.ChapterNumber != want[i] {
			t.Errorf("page item %d chapter_number = %d, want %d", i, ch.ChapterNumber, want[i])
		}
	}
}

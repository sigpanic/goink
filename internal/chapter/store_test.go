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

	first := Chapter{NovelID: 1, SortOrder: 2, Title: "开头"}
	second := Chapter{NovelID: 1, SortOrder: 1, Title: "发展"}
	db.Create(&first)
	db.Create(&second)
	db.Create(&Chapter{NovelID: 2, SortOrder: 1, Title: "另一部"})

	chapters, _ := s.ListAllByNovel(ctx, 1)
	if len(chapters) != 2 {
		t.Errorf("expected 2, got %d", len(chapters))
	}
	if chapters[0].ID != second.ID {
		t.Errorf("first chapter id = %d, want %d", chapters[0].ID, second.ID)
	}
}

func TestChCountByNovel(t *testing.T) {
	db := openChDB(t)
	s := NewStore(db, testChLogger())
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if err := db.Create(&Chapter{NovelID: 1, SortOrder: i + 1}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Create(&Chapter{NovelID: 2, SortOrder: 1}).Error; err != nil {
		t.Fatal(err)
	}

	count, err := s.CountByNovel(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Errorf("count = %d, want 3", count)
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
	unassigned := Chapter{NovelID: 1, SortOrder: 1}
	v1Chapter := Chapter{NovelID: 1, VolumeID: &v1.ID, SortOrder: 1}
	v2Later := Chapter{NovelID: 1, VolumeID: &v2.ID, SortOrder: 2}
	v2First := Chapter{NovelID: 1, VolumeID: &v2.ID, SortOrder: 1}
	for _, ch := range []*Chapter{&unassigned, &v1Chapter, &v2Later, &v2First} {
		if err := db.Create(ch).Error; err != nil {
			t.Fatal(err)
		}
	}

	chapters, err := s.ListAllByNovel(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	want := []int64{v2First.ID, v2Later.ID, v1Chapter.ID, unassigned.ID}
	for i, ch := range chapters {
		if ch.ID != want[i] {
			t.Errorf("index %d chapter id = %d, want %d", i, ch.ID, want[i])
		}
	}
}

func TestChGetReadingNumberByIDUsesCompositeOrder(t *testing.T) {
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
	unassigned := Chapter{NovelID: 1, SortOrder: 1}
	v1Chapter := Chapter{NovelID: 1, VolumeID: &v1.ID, SortOrder: 1}
	v2Later := Chapter{NovelID: 1, VolumeID: &v2.ID, SortOrder: 2}
	v2First := Chapter{NovelID: 1, VolumeID: &v2.ID, SortOrder: 1}
	for _, ch := range []*Chapter{&unassigned, &v1Chapter, &v2Later, &v2First} {
		if err := db.Create(ch).Error; err != nil {
			t.Fatal(err)
		}
	}

	for _, tt := range []struct {
		id   int64
		want int
	}{
		{v2First.ID, 1},
		{v2Later.ID, 2},
		{v1Chapter.ID, 3},
		{unassigned.ID, 4},
	} {
		got, err := s.GetReadingNumberByID(ctx, 1, tt.id)
		if err != nil {
			t.Fatal(err)
		}
		if got != tt.want {
			t.Errorf("chapter %d reading number = %d, want %d", tt.id, got, tt.want)
		}
	}

	if _, err := s.GetReadingNumberByID(ctx, 1, 999); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("missing chapter error = %v, want gorm.ErrRecordNotFound", err)
	}
}

func TestChListByNovel_Desc(t *testing.T) {
	db := openChDB(t)
	s := NewStore(db, testChLogger())
	ctx := context.Background()

	first := Chapter{NovelID: 1, SortOrder: 2}
	second := Chapter{NovelID: 1, SortOrder: 1}
	db.Create(&first)
	db.Create(&second)

	result, _ := s.ListByNovel(ctx, 1, ListByNovelOptions{Order: "desc", PageParams: storage.PageParams{Size: -1}})
	if result.Items[0].ID != first.ID {
		t.Errorf("desc: first chapter id = %d, want %d", result.Items[0].ID, first.ID)
	}
}

func TestChGetByID(t *testing.T) {
	db := openChDB(t)
	s := NewStore(db, testChLogger())
	ctx := context.Background()

	want := Chapter{NovelID: 1, Title: "高潮"}
	db.Create(&want)

	ch, err := s.GetByID(ctx, 1, want.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if ch.Title != "高潮" {
		t.Errorf("expected 高潮, got %s", ch.Title)
	}
}

func TestChGetByID_NotFound(t *testing.T) {
	db := openChDB(t)
	s := NewStore(db, testChLogger())
	ctx := context.Background()

	_, err := s.GetByID(ctx, 1, 999)
	if err == nil {
		t.Error("expected error for not found")
	}
}

func TestChUpdateTitle(t *testing.T) {
	db := openChDB(t)
	s := NewStore(db, testChLogger())
	ctx := context.Background()

	ch := Chapter{NovelID: 1, Title: "旧标题"}
	if err := db.Create(&ch).Error; err != nil {
		t.Fatal(err)
	}

	if err := s.UpdateTitle(ctx, 1, ch.ID, "新标题"); err != nil {
		t.Fatalf("UpdateTitle: %v", err)
	}
	got, err := s.GetByID(ctx, 1, ch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "新标题" {
		t.Errorf("title = %q, want 新标题", got.Title)
	}
	if err := s.UpdateTitle(ctx, 2, ch.ID, "不应更新"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("foreign novel error = %v, want gorm.ErrRecordNotFound", err)
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
	if err := db.Create(&Chapter{NovelID: 1, VolumeID: &v.ID, SortOrder: 1}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&Chapter{NovelID: 1, SortOrder: 9}).Error; err != nil {
		t.Fatal(err)
	}

	created, err := s.Create(ctx, nil, 1, &v.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if created.VolumeID == nil || *created.VolumeID != v.ID {
		t.Errorf("volume_id = %v, want %d", created.VolumeID, v.ID)
	}
	if created.SortOrder != 2 {
		t.Errorf("sort_order = %d, want 2", created.SortOrder)
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
	if err := db.Create(&Chapter{NovelID: 1, VolumeID: &v.ID, SortOrder: 100}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&Chapter{NovelID: 1, SortOrder: 2}).Error; err != nil {
		t.Fatal(err)
	}

	created, err := s.Create(ctx, nil, 1, nil, "未分卷新章")
	if err != nil {
		t.Fatal(err)
	}
	if created.VolumeID != nil || created.SortOrder != 3 {
		t.Errorf("created = %+v, want unassigned sort_order = 3", created)
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

	for _, sortOrder := range []int{5, 1, 4, 2, 3} {
		db.Create(&Chapter{NovelID: 1, SortOrder: sortOrder})
	}

	recent, _ := s.GetRecent(ctx, 1, 2)
	if len(recent) != 2 {
		t.Fatalf("expected 2, got %d", len(recent))
	}
	if recent[0].SortOrder != 5 || recent[1].SortOrder != 4 {
		t.Errorf("recent sort_order = %d/%d, want 5/4", recent[0].SortOrder, recent[1].SortOrder)
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
	v1Chapter := Chapter{NovelID: 1, VolumeID: &v1.ID, SortOrder: 1}
	if err := db.Create(&v1Chapter).Error; err != nil {
		t.Fatal(err)
	}
	v2Chapter := Chapter{NovelID: 1, VolumeID: &v2.ID, SortOrder: 1}
	if err := db.Create(&v2Chapter).Error; err != nil {
		t.Fatal(err)
	}
	unassigned := Chapter{NovelID: 1, SortOrder: 1}
	if err := db.Create(&unassigned).Error; err != nil {
		t.Fatal(err)
	}

	recent, err := s.GetRecent(ctx, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	want := []int64{unassigned.ID, v2Chapter.ID}
	for i, ch := range recent {
		if ch.ID != want[i] {
			t.Errorf("index %d chapter id = %d, want %d", i, ch.ID, want[i])
		}
	}
}

func TestChSearchByNovelOrdersBySortOrder(t *testing.T) {
	db := openChDB(t)
	s := NewStore(db, testChLogger())
	ctx := context.Background()

	db.Create(&Chapter{NovelID: 1, SortOrder: 2, Title: "相同"})
	db.Create(&Chapter{NovelID: 1, SortOrder: 1, Title: "相同"})

	chapters, err := s.SearchByNovel(ctx, 1, "相同", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 2 || chapters[0].SortOrder != 1 {
		t.Errorf("search results = %+v, want sort_order 1 first", chapters)
	}
}

func TestListByNovel_Pagination(t *testing.T) {
	db := openChDB(t)
	s := NewStore(db, testChLogger())
	ctx := context.Background()

	for i := 1; i <= 10; i++ {
		db.Create(&Chapter{NovelID: 1, SortOrder: 11 - i})
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
	want := []int{4, 5, 6}
	for i, ch := range result.Items {
		if ch.SortOrder != want[i] {
			t.Errorf("page item %d sort_order = %d, want %d", i, ch.SortOrder, want[i])
		}
	}
}

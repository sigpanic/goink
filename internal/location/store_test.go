package location

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/storage"
)

func openLocDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&Location{}, &LocationRelation{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func testLocLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestLocListByNovel_All(t *testing.T) {
	db := openLocDB(t)
	s := NewStore(db, testLocLogger())
	ctx := context.Background()

	db.Create(&Location{NovelID: 1, Name: "森林"})
	db.Create(&Location{NovelID: 1, Name: "城堡"})
	db.Create(&Location{NovelID: 2, Name: "沙漠"})

	result, _ := s.ListByNovel(ctx, 1, ListByNovelOptions{
		PageParams: storage.PageParams{Size: -1},
	})
	if len(result.Items) != 2 {
		t.Errorf("expected 2, got %d", len(result.Items))
	}
}

func TestLocListByNovel_Filter(t *testing.T) {
	db := openLocDB(t)
	s := NewStore(db, testLocLogger())
	ctx := context.Background()

	db.Create(&Location{NovelID: 1, Name: "迷雾森林", LocationType: "森林"})
	db.Create(&Location{NovelID: 1, Name: "黑铁城堡", LocationType: "城堡"})

	result, _ := s.ListByNovel(ctx, 1, ListByNovelOptions{LocationType: "森林"})
	if result.Total != 1 {
		t.Errorf("filter by type: expected 1, got %d", result.Total)
	}
}

func TestLocListByNovel_Search(t *testing.T) {
	db := openLocDB(t)
	s := NewStore(db, testLocLogger())
	ctx := context.Background()

	db.Create(&Location{NovelID: 1, Name: "迷雾森林"})
	db.Create(&Location{NovelID: 1, Name: "黑铁城堡"})

	result, _ := s.ListByNovel(ctx, 1, ListByNovelOptions{Search: "迷雾"})
	if result.Total != 1 {
		t.Errorf("search: expected 1, got %d", result.Total)
	}
}

func TestLocGetChildren(t *testing.T) {
	db := openLocDB(t)
	s := NewStore(db, testLocLogger())
	ctx := context.Background()

	parent := Location{NovelID: 1, Name: "王宫"}
	db.Create(&parent)
	db.Create(&Location{NovelID: 1, Name: "大殿", ParentLocationID: &parent.ID})
	db.Create(&Location{NovelID: 1, Name: "密室", ParentLocationID: &parent.ID})

	children, _ := s.GetChildren(ctx, parent.ID)
	if len(children) != 2 {
		t.Errorf("expected 2 children, got %d", len(children))
	}
}

func TestLocGetByIDs(t *testing.T) {
	db := openLocDB(t)
	s := NewStore(db, testLocLogger())
	ctx := context.Background()

	l1 := Location{NovelID: 1, Name: "A"}
	l2 := Location{NovelID: 1, Name: "B"}
	db.Create(&l1)
	db.Create(&l2)

	locs, _ := s.GetByIDs(ctx, []int64{l1.ID, l2.ID})
	if len(locs) != 2 {
		t.Errorf("expected 2, got %d", len(locs))
	}
}

func TestLocRelationsByLocation(t *testing.T) {
	db := openLocDB(t)
	s := NewStore(db, testLocLogger())
	ctx := context.Background()

	db.Create(&LocationRelation{NovelID: 1, LocationA: 1, LocationB: 2, RelationType: "相邻"})
	db.Create(&LocationRelation{NovelID: 1, LocationA: 1, LocationB: 3, RelationType: "山路"})

	rels, _ := s.ListRelationsByLocation(ctx, 1)
	if len(rels) != 2 {
		t.Errorf("expected 2 relations for location 1, got %d", len(rels))
	}
}

func TestLocRelationsInvolving(t *testing.T) {
	db := openLocDB(t)
	s := NewStore(db, testLocLogger())
	ctx := context.Background()

	db.Create(&LocationRelation{NovelID: 1, LocationA: 1, LocationB: 2, RelationType: "相邻"})
	db.Create(&LocationRelation{NovelID: 1, LocationA: 3, LocationB: 4, RelationType: "山路"})

	rels, _ := s.ListRelationsInvolving(ctx, []int64{1, 2})
	if len(rels) != 1 {
		t.Errorf("expected 1 relation involving {1,2}, got %d", len(rels))
	}
}

func TestLocUpsertRelation(t *testing.T) {
	db := openLocDB(t)
	s := NewStore(db, testLocLogger())
	ctx := context.Background()

	rel := &LocationRelation{NovelID: 1, LocationA: 1, LocationB: 2, RelationType: "相邻"}
	if err := s.UpsertRelation(ctx, rel); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	// 第二次 upsert 同对，应该更新
	rel2 := &LocationRelation{NovelID: 1, LocationA: 1, LocationB: 2, RelationType: "骑马半天"}
	if err := s.UpsertRelation(ctx, rel2); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	var count int64
	db.Model(&LocationRelation{}).Count(&count)
	if count != 1 {
		t.Errorf("upsert should not create duplicate, got %d rows", count)
	}
}

func TestLocListByNovel_Pagination(t *testing.T) {
	db := openLocDB(t)
	s := NewStore(db, testLocLogger())
	ctx := context.Background()

	for _, name := range []string{"a", "b", "c", "d"} {
		db.Create(&Location{NovelID: 1, Name: name})
	}

	result, _ := s.ListByNovel(ctx, 1, ListByNovelOptions{
		PageParams: storage.PageParams{Page: 1, Size: 2},
	})
	if result.Items == nil || len(result.Items) != 2 {
		t.Errorf("expected 2, got %d", len(result.Items))
	}
}

// ── CRUD ────────────────────────────────────────────────────

func TestLocCreate(t *testing.T) {
	db := openLocDB(t)
	ctx := context.Background()

	loc := Location{NovelID: 1, Name: "森林", LocationType: "自然", Tags: `["秘境","古老"]`}
	if err := db.WithContext(ctx).Create(&loc).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	if loc.ID == 0 {
		t.Error("ID should be set after create")
	}

	var found Location
	db.First(&found, loc.ID)
	if found.Name != "森林" {
		t.Errorf("expected 森林, got %s", found.Name)
	}
}

func TestLocUpdate(t *testing.T) {
	db := openLocDB(t)
	ctx := context.Background()

	loc := Location{NovelID: 1, Name: "旧名", LocationType: "旧类型", Description: "旧描述"}
	db.WithContext(ctx).Create(&loc)

	type UpdateInput struct {
		Name        string `json:"name,omitempty"`
		Description string `json:"description,omitempty"`
	}
	input := UpdateInput{Name: "新名", Description: ""}
	if err := db.WithContext(ctx).Model(&Location{}).Where("id = ?", loc.ID).Updates(&input).Error; err != nil {
		t.Fatalf("update: %v", err)
	}

	var updated Location
	db.WithContext(ctx).First(&updated, loc.ID)
	if updated.Name != "新名" {
		t.Errorf("expected 新名, got %s", updated.Name)
	}
	if updated.Description != "旧描述" {
		t.Errorf("description should be unchanged (zero value skipped), got %s", updated.Description)
	}
}

func TestLocUpdateClearParent(t *testing.T) {
	db := openLocDB(t)
	ctx := context.Background()

	parent := Location{NovelID: 1, Name: "王宫"}
	db.WithContext(ctx).Create(&parent)
	child := Location{NovelID: 1, Name: "大殿", ParentLocationID: &parent.ID}
	db.WithContext(ctx).Create(&child)

	// clear_parent → UPDATE parent_location_id = nil, then Updates other fields
	if err := db.WithContext(ctx).Model(&Location{}).Where("id = ?", child.ID).Update("parent_location_id", nil).Error; err != nil {
		t.Fatalf("clear parent: %v", err)
	}
	if err := db.WithContext(ctx).Model(&Location{}).Where("id = ?", child.ID).Updates(&Location{Name: "大殿(独立)"}).Error; err != nil {
		t.Fatalf("update after clear: %v", err)
	}

	var updated Location
	db.WithContext(ctx).First(&updated, child.ID)
	if updated.ParentLocationID != nil {
		t.Error("parent should be cleared")
	}
	if updated.Name != "大殿(独立)" {
		t.Errorf("expected 大殿(独立), got %s", updated.Name)
	}
}

func TestLocDelete(t *testing.T) {
	db := openLocDB(t)
	ctx := context.Background()

	parent := Location{NovelID: 1, Name: "城堡"}
	db.WithContext(ctx).Create(&parent)
	child := Location{NovelID: 1, Name: "大厅", ParentLocationID: &parent.ID}
	db.WithContext(ctx).Create(&child)
	other := Location{NovelID: 1, Name: "森林"}
	db.WithContext(ctx).Create(&other)
	db.WithContext(ctx).Create(&LocationRelation{NovelID: 1, LocationA: parent.ID, LocationB: other.ID, RelationType: "相邻"})

	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Location{}).Where("parent_location_id = ?", parent.ID).Update("parent_location_id", nil).Error; err != nil {
			return err
		}
		if err := tx.Where("location_a = ? OR location_b = ?", parent.ID, parent.ID).Delete(&LocationRelation{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", parent.ID).Delete(&Location{}).Error
	})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}

	var found Location
	if db.First(&found, parent.ID).Error == nil {
		t.Error("parent should be deleted")
	}
	db.First(&found, child.ID)
	if found.ParentLocationID != nil {
		t.Error("child parent should be nil after reparenting")
	}
	var relCount int64
	db.Model(&LocationRelation{}).Where("location_a = ? OR location_b = ?", parent.ID, parent.ID).Count(&relCount)
	if relCount != 0 {
		t.Errorf("relations should be cascade-deleted, got %d", relCount)
	}
	var otherLoc Location
	if db.First(&otherLoc, other.ID).Error != nil {
		t.Error("other should remain")
	}
}

package character

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/storage"
)

func openCharDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&Character{}, &CharacterRelation{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func testCharLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestListAllByNovel(t *testing.T) {
	db := openCharDB(t)
	s := NewStore(db, testCharLogger())
	ctx := context.Background()

	db.Create(&Character{NovelID: 1, Name: "张三"})
	db.Create(&Character{NovelID: 1, Name: "李四"})
	db.Create(&Character{NovelID: 2, Name: "王五"})

	chars, err := s.ListAllByNovel(ctx, 1)
	if err != nil {
		t.Fatalf("ListAllByNovel: %v", err)
	}
	if len(chars) != 2 {
		t.Errorf("expected 2, got %d", len(chars))
	}
}

func TestListByNovel_Search(t *testing.T) {
	db := openCharDB(t)
	s := NewStore(db, testCharLogger())
	ctx := context.Background()

	db.Create(&Character{NovelID: 1, Name: "张三丰"})
	db.Create(&Character{NovelID: 1, Name: "李四娘"})

	result, err := s.ListByNovel(ctx, 1, ListByNovelOptions{Search: "张三", PageParams: storage.PageParams{Size: -1}})
	if err != nil {
		t.Fatalf("ListByNovel: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("search should return 1, got %d", result.Total)
	}
	if result.Items[0].Name != "张三丰" {
		t.Errorf("expected 张三丰, got %s", result.Items[0].Name)
	}
}

func TestListByNovel_Pagination(t *testing.T) {
	db := openCharDB(t)
	s := NewStore(db, testCharLogger())
	ctx := context.Background()

	for _, name := range []string{"a", "b", "c", "d", "e"} {
		db.Create(&Character{NovelID: 1, Name: name})
	}

	result, _ := s.ListByNovel(ctx, 1, ListByNovelOptions{
		PageParams: storage.PageParams{Page: 1, Size: 2},
	})
	if len(result.Items) != 2 {
		t.Errorf("page 1 size 2: expected 2 items, got %d", len(result.Items))
	}
	if result.Total != 5 {
		t.Errorf("total should be 5, got %d", result.Total)
	}
}

func TestGetByIDs(t *testing.T) {
	db := openCharDB(t)
	s := NewStore(db, testCharLogger())
	ctx := context.Background()

	c1 := Character{NovelID: 1, Name: "张三"}
	c2 := Character{NovelID: 1, Name: "李四"}
	db.Create(&c1)
	db.Create(&c2)

	chars, err := s.GetByIDs(ctx, []int64{c1.ID, c2.ID})
	if err != nil {
		t.Fatalf("GetByIDs: %v", err)
	}
	if len(chars) != 2 {
		t.Errorf("expected 2, got %d", len(chars))
	}
}

func TestGetByIDs_Empty(t *testing.T) {
	db := openCharDB(t)
	s := NewStore(db, testCharLogger())
	ctx := context.Background()

	chars, err := s.GetByIDs(ctx, []int64{})
	if err != nil {
		t.Errorf("empty input should not error: %v", err)
	}
	if chars != nil {
		t.Errorf("expected nil, got %v", chars)
	}
}

func TestListCurrentByNovel(t *testing.T) {
	db := openCharDB(t)
	s := NewStore(db, testCharLogger())
	ctx := context.Background()

	db.Create(&Character{NovelID: 1, Name: "张三"})
	db.Create(&Character{NovelID: 1, Name: "李四"})
	db.Create(&CharacterRelation{NovelID: 1, SourceCharacterID: 1, TargetCharacterID: 2, RelationDescribe: "朋友", IsCurrent: true})
	db.Create(&CharacterRelation{NovelID: 1, SourceCharacterID: 2, TargetCharacterID: 1, RelationDescribe: "亦敌亦友", IsCurrent: false})

	rels, err := s.ListCurrentByNovel(ctx, 1)
	if err != nil {
		t.Fatalf("ListCurrentByNovel: %v", err)
	}
	if len(rels) != 1 {
		t.Errorf("expected 1 current relation, got %d", len(rels))
	}
}

func TestListByCharacter(t *testing.T) {
	db := openCharDB(t)
	s := NewStore(db, testCharLogger())
	ctx := context.Background()

	db.Create(&CharacterRelation{NovelID: 1, SourceCharacterID: 1, TargetCharacterID: 2, RelationDescribe: "朋友", IsCurrent: true})
	db.Create(&CharacterRelation{NovelID: 1, SourceCharacterID: 3, TargetCharacterID: 1, RelationDescribe: "敌人", IsCurrent: true})

	rels, err := s.ListByCharacter(ctx, 1)
	if err != nil {
		t.Fatalf("ListByCharacter: %v", err)
	}
	if len(rels) != 2 {
		t.Errorf("character 1 should have 2 relations (any direction), got %d", len(rels))
	}
}

func TestListByCharacters(t *testing.T) {
	db := openCharDB(t)
	s := NewStore(db, testCharLogger())
	ctx := context.Background()

	db.Create(&CharacterRelation{NovelID: 1, SourceCharacterID: 1, TargetCharacterID: 2, RelationDescribe: "朋友", IsCurrent: true})
	db.Create(&CharacterRelation{NovelID: 1, SourceCharacterID: 3, TargetCharacterID: 4, RelationDescribe: "师徒", IsCurrent: true})

	rels, err := s.ListByCharacters(ctx, []int64{1, 2})
	if err != nil {
		t.Fatalf("ListByCharacters: %v", err)
	}
	if len(rels) != 1 {
		t.Errorf("should find only relation between 1 and 2, got %d", len(rels))
	}
}

func TestGetHistory(t *testing.T) {
	db := openCharDB(t)
	s := NewStore(db, testCharLogger())
	ctx := context.Background()

	db.Create(&CharacterRelation{NovelID: 1, SourceCharacterID: 1, TargetCharacterID: 2, RelationDescribe: "朋友", IsCurrent: false})
	db.Create(&CharacterRelation{NovelID: 1, SourceCharacterID: 1, TargetCharacterID: 2, RelationDescribe: "敌人", IsCurrent: true})

	history, err := s.GetHistory(ctx, 1, 2)
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if len(history) != 2 {
		t.Errorf("expected 2 history records, got %d", len(history))
	}
	// 反向也能查到
	reverse, _ := s.GetHistory(ctx, 2, 1)
	if len(reverse) != len(history) {
		t.Error("direction should not matter for history")
	}
}

func TestListBetweenCharacters(t *testing.T) {
	db := openCharDB(t)
	s := NewStore(db, testCharLogger())
	ctx := context.Background()

	db.Create(&CharacterRelation{NovelID: 1, SourceCharacterID: 1, TargetCharacterID: 2, RelationDescribe: "朋友", IsCurrent: true})
	db.Create(&CharacterRelation{NovelID: 1, SourceCharacterID: 1, TargetCharacterID: 3, RelationDescribe: "敌人", IsCurrent: true})

	rels, err := s.ListBetweenCharacters(ctx, []int64{1, 2})
	if err != nil {
		t.Fatalf("ListBetweenCharacters: %v", err)
	}
	if len(rels) != 1 {
		t.Errorf("should find 1 relation within set {1,2}, got %d", len(rels))
	}
}

func TestDeactivate(t *testing.T) {
	db := openCharDB(t)
	s := NewStore(db, testCharLogger())
	ctx := context.Background()

	rel := CharacterRelation{NovelID: 1, SourceCharacterID: 1, TargetCharacterID: 2, RelationDescribe: "朋友", IsCurrent: true}
	db.Create(&rel)

	if err := s.Deactivate(ctx, rel.ID); err != nil {
		t.Fatalf("Deactivate: %v", err)
	}

	var updated CharacterRelation
	db.First(&updated, rel.ID)
	if updated.IsCurrent {
		t.Error("relation should be deactivated")
	}
}

func TestDeactivate_NotFound(t *testing.T) {
	db := openCharDB(t)
	s := NewStore(db, testCharLogger())
	ctx := context.Background()

	err := s.Deactivate(ctx, 9999)
	if err == nil {
		t.Error("should error for non-existent relation")
	}
}

// ── CRUD ──────────────────────────────────────────────────

func TestCharCreate(t *testing.T) {
	db := openCharDB(t)
	s := NewStore(db, testCharLogger())
	ctx := context.Background()

	char := Character{NovelID: 1, Name: "张三", Description: "主角", Personality: `{"traits":["勇敢"]}`, Abilities: `["剑术"]`}
	if err := s.DB.WithContext(ctx).Create(&char).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	if char.ID == 0 {
		t.Error("ID should be set after create")
	}

	var found Character
	db.First(&found, char.ID)
	if found.Name != "张三" {
		t.Errorf("expected 张三, got %s", found.Name)
	}
	if found.NovelID != 1 {
		t.Errorf("expected novel_id=1, got %d", found.NovelID)
	}
}

func TestCharUpdate(t *testing.T) {
	db := openCharDB(t)
	s := NewStore(db, testCharLogger())
	ctx := context.Background()

	char := Character{NovelID: 1, Name: "旧名", Description: "旧描述", Abilities: `["旧能力"]`}
	s.DB.WithContext(ctx).Create(&char)

	type UpdateInput struct {
		Name        string `json:"name,omitempty"`
		Description string `json:"description,omitempty"`
		Abilities   string `json:"abilities,omitempty"`
	}
	input := UpdateInput{Name: "新名", Abilities: ""}
	if err := s.DB.WithContext(ctx).Model(&Character{}).Where("id = ?", char.ID).Updates(&input).Error; err != nil {
		t.Fatalf("update: %v", err)
	}

	var updated Character
	s.DB.WithContext(ctx).First(&updated, char.ID)
	if updated.Name != "新名" {
		t.Errorf("name: expected 新名, got %s", updated.Name)
	}
	if updated.Description != "旧描述" {
		t.Errorf("description: expected 旧描述 unchanged (zero value skipped), got %s", updated.Description)
	}
	if updated.Abilities != `["旧能力"]` {
		t.Errorf("abilities: expected [\"旧能力\"] unchanged (empty string skipped), got %s", updated.Abilities)
	}
}

func TestCharDelete(t *testing.T) {
	db := openCharDB(t)
	s := NewStore(db, testCharLogger())
	ctx := context.Background()

	c1 := Character{NovelID: 1, Name: "张三"}
	c2 := Character{NovelID: 1, Name: "李四"}
	s.DB.WithContext(ctx).Create(&c1)
	s.DB.WithContext(ctx).Create(&c2)

	s.DB.WithContext(ctx).Create(&CharacterRelation{
		NovelID: 1, SourceCharacterID: c1.ID, TargetCharacterID: c2.ID,
		RelationDescribe: "朋友", IsCurrent: true,
	})

	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("source_character_id = ? OR target_character_id = ?", c1.ID, c1.ID).Delete(&CharacterRelation{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", c1.ID).Delete(&Character{}).Error
	})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}

	var found Character
	if db.First(&found, c1.ID).Error == nil {
		t.Error("character should be deleted")
	}

	var relCount int64
	db.Model(&CharacterRelation{}).Where("source_character_id = ? OR target_character_id = ?", c1.ID, c1.ID).Count(&relCount)
	if relCount != 0 {
		t.Errorf("relations should be cascade-deleted, got %d", relCount)
	}

	var remaining Character
	if db.First(&remaining, c2.ID).Error != nil {
		t.Error("c2 should remain")
	}
}

package volume

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// testVolLogger 返回丢弃输出的 logger（store 需要非 nil logger）。
func testVolLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// openVolDB 建内存 DB + volumes 表。chapters 表用最小 schema（只用 volume_id/sort_order），
// 避免 volume 包 import chapter 包（会成环）。
func openVolDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&Volume{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.Exec(`CREATE TABLE chapters (
		id integer PRIMARY KEY AUTOINCREMENT,
		novel_id integer NOT NULL,
		volume_id integer,
		sort_order integer DEFAULT 0
	)`).Error; err != nil {
		t.Fatalf("create chapters: %v", err)
	}
	return db
}

// newTestStore 建 Store（统一注入测试 logger）。
func newTestStore(db *gorm.DB) *Store {
	return NewStore(db, testVolLogger())
}

// seedChapter 直接插入章节行（绕过 chapter 包）。
func seedChapter(t *testing.T, db *gorm.DB, novelID int64, volumeID *int64, sort int) {
	t.Helper()
	if err := db.Exec(
		"INSERT INTO chapters (novel_id, volume_id, sort_order) VALUES (?, ?, ?)",
		novelID, volumeID, sort).Error; err != nil {
		t.Fatalf("seed chapter: %v", err)
	}
}

func mustCreate(t *testing.T, db *gorm.DB, novelID int64, name string) *Volume {
	t.Helper()
	v, err := newTestStore(db).Create(context.Background(), nil, novelID, name)
	if err != nil {
		t.Fatalf("create volume %q: %v", name, err)
	}
	return v
}

func chapterSorts(t *testing.T, db *gorm.DB, novelID int64) map[int64]int {
	t.Helper()
	type row struct {
		ID        int64
		SortOrder int
	}
	var rows []row
	if err := db.Raw("SELECT id, sort_order FROM chapters WHERE novel_id = ? ORDER BY id", novelID).
		Scan(&rows).Error; err != nil {
		t.Fatalf("read chapter sorts: %v", err)
	}
	m := make(map[int64]int, len(rows))
	for _, r := range rows {
		m[r.ID] = r.SortOrder
	}
	return m
}

// ── CRUD ─────────────────────────────────────────────────

// Create 追加到末尾，sort_order 递增。
func TestVolumeCreateAppends(t *testing.T) {
	db := openVolDB(t)
	s := newTestStore(db)

	v1 := mustCreate(t, db, 1, "第一卷")
	v2 := mustCreate(t, db, 1, "第二卷")

	if v1.SortOrder != 1 || v2.SortOrder != 2 {
		t.Errorf("sort_order = %d/%d, want 1/2", v1.SortOrder, v2.SortOrder)
	}
	// 其他小说互不影响
	v3 := mustCreate(t, db, 2, "别家卷")
	if v3.SortOrder != 1 {
		t.Errorf("novel 2 sort_order = %d, want 1", v3.SortOrder)
	}
	_ = s
}

// 同一小说内卷名不可重复。
func TestVolumeCreateDuplicateName(t *testing.T) {
	db := openVolDB(t)
	ctx := context.Background()
	s := newTestStore(db)
	mustCreate(t, db, 1, "第一卷")

	if _, err := s.Create(ctx, nil, 1, "第一卷"); !errors.Is(err, ErrNameTaken) {
		t.Errorf("err = %v, want ErrNameTaken", err)
	}
	// 不同小说可用同名
	if _, err := s.Create(ctx, nil, 2, "第一卷"); err != nil {
		t.Errorf("other novel same name should succeed, got %v", err)
	}
}

// GetByID 校验小说归属。
func TestVolumeGetByIDForeignNovel(t *testing.T) {
	db := openVolDB(t)
	ctx := context.Background()
	v := mustCreate(t, db, 1, "第一卷")

	if _, err := newTestStore(db).GetByID(ctx, nil, 2, v.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

// Update 重命名；重名报错；同名无操作。
func TestVolumeUpdate(t *testing.T) {
	db := openVolDB(t)
	ctx := context.Background()
	s := newTestStore(db)
	v1 := mustCreate(t, db, 1, "第一卷")
	v2 := mustCreate(t, db, 1, "第二卷")

	if err := s.Update(ctx, nil, 1, v1.ID, "卷一"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	got, _ := s.GetByID(ctx, nil, 1, v1.ID)
	if got.Name != "卷一" {
		t.Errorf("name = %q, want 卷一", got.Name)
	}

	if err := s.Update(ctx, nil, 1, v1.ID, "第二卷"); !errors.Is(err, ErrNameTaken) {
		t.Errorf("err = %v, want ErrNameTaken", err)
	}

	// 改成自身名字应成功（排除自身）
	if err := s.Update(ctx, nil, 1, v2.ID, "第二卷"); err != nil {
		t.Errorf("same-name update should succeed, got %v", err)
	}
}

// Delete 卷下有章节时拒绝。
func TestVolumeDeleteRefusesWhenHasChapters(t *testing.T) {
	db := openVolDB(t)
	ctx := context.Background()
	v := mustCreate(t, db, 1, "第一卷")
	vid := v.ID
	seedChapter(t, db, 1, &vid, 1)

	if err := newTestStore(db).Delete(ctx, nil, 1, v.ID); !errors.Is(err, ErrHasChapters) {
		t.Errorf("err = %v, want ErrHasChapters", err)
	}
}

// Delete 空卷可删。
func TestVolumeDeleteEmpty(t *testing.T) {
	db := openVolDB(t)
	ctx := context.Background()
	s := newTestStore(db)
	v := mustCreate(t, db, 1, "第一卷")

	if err := s.Delete(ctx, nil, 1, v.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.GetByID(ctx, nil, 1, v.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("volume should be gone, got %v", err)
	}
}

// ListByNovel 按 sort_order 升序；LastByNovel 取末尾；无卷返回 nil。
func TestVolumeListAndLast(t *testing.T) {
	db := openVolDB(t)
	ctx := context.Background()
	s := newTestStore(db)

	if last, err := s.LastByNovel(ctx, nil, 1); err != nil || last != nil {
		t.Errorf("empty novel last = %v, err = %v, want nil/nil", last, err)
	}

	mustCreate(t, db, 1, "第一卷")
	v2 := mustCreate(t, db, 1, "第二卷")

	list, err := s.ListByNovel(ctx, nil, 1)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 || list[0].Name != "第一卷" || list[1].Name != "第二卷" {
		t.Errorf("list = %+v", list)
	}

	last, err := s.LastByNovel(ctx, nil, 1)
	if err != nil {
		t.Fatalf("last: %v", err)
	}
	if last == nil || last.ID != v2.ID {
		t.Errorf("last = %+v, want id=%d", last, v2.ID)
	}
}

// ── Reorder ──────────────────────────────────────────────

// Reorder 全量重排，与唯一索引不冲突。
func TestVolumeReorder(t *testing.T) {
	db := openVolDB(t)
	ctx := context.Background()
	s := newTestStore(db)
	v1 := mustCreate(t, db, 1, "第一卷")
	v2 := mustCreate(t, db, 1, "第二卷")
	v3 := mustCreate(t, db, 1, "第三卷")

	// 反转顺序
	if err := s.Reorder(ctx, nil, 1, []int64{v3.ID, v2.ID, v1.ID}); err != nil {
		t.Fatalf("reorder: %v", err)
	}

	list, _ := s.ListByNovel(ctx, nil, 1)
	want := []int64{v3.ID, v2.ID, v1.ID}
	for i, v := range list {
		if v.ID != want[i] {
			t.Errorf("position %d = id %d, want %d", i, v.ID, want[i])
		}
		if v.SortOrder != i+1 {
			t.Errorf("position %d sort_order = %d, want %d", i, v.SortOrder, i+1)
		}
	}
}

// Reorder 缺漏或混入他卷都报错。
func TestVolumeReorderRejectsPartial(t *testing.T) {
	db := openVolDB(t)
	ctx := context.Background()
	s := newTestStore(db)
	v1 := mustCreate(t, db, 1, "第一卷")
	mustCreate(t, db, 1, "第二卷")
	foreign := mustCreate(t, db, 2, "别家卷")

	if err := s.Reorder(ctx, nil, 1, []int64{v1.ID}); err == nil {
		t.Error("partial reorder should fail")
	}
	if err := s.Reorder(ctx, nil, 1, []int64{v1.ID, foreign.ID}); err == nil {
		t.Error("foreign volume in reorder should fail")
	}
}

// ── sort_order 分配 ──────────────────────────────────────

// 空小说：第一个章节 sort_order = 1。
func TestAllocateFirstChapter(t *testing.T) {
	db := openVolDB(t)
	ctx := context.Background()
	s := newTestStore(db)

	err := db.Transaction(func(tx *gorm.DB) error {
		pos, err := s.AllocateChapterSortOrder(ctx, tx, 1, 0)
		if err != nil {
			return err
		}
		if pos != 1 {
			t.Errorf("pos = %d, want 1", pos)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// 卷内追加：取卷内 max+1，不影响其他章节。
func TestAllocateAppendsWithinVolume(t *testing.T) {
	db := openVolDB(t)
	ctx := context.Background()
	s := newTestStore(db)
	v1 := mustCreate(t, db, 1, "第一卷")
	v2 := mustCreate(t, db, 1, "第二卷")
	seedChapter(t, db, 1, &v1.ID, 1)
	seedChapter(t, db, 1, &v2.ID, 2)

	err := db.Transaction(func(tx *gorm.DB) error {
		pos, err := s.AllocateChapterSortOrder(ctx, tx, 1, v1.ID)
		if err != nil {
			return err
		}
		// 卷内 max(1)+1 = 2，插在 2 位，原 sort>=2 的章节整体 +1
		if pos != 2 {
			t.Errorf("pos = %d, want 2", pos)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	sorts := chapterSorts(t, db, 1)
	if sorts[1] != 1 {
		t.Errorf("chapter1 sort = %d, want 1", sorts[1])
	}
	if sorts[2] != 3 {
		t.Errorf("chapter2 sort = %d, want 3 (shifted)", sorts[2])
	}
}

// 空卷插在前面：锚点取前卷章节的 max，插入后腾位。
func TestAllocateEmptyVolumeAfterExisting(t *testing.T) {
	db := openVolDB(t)
	ctx := context.Background()
	s := newTestStore(db)
	v1 := mustCreate(t, db, 1, "第一卷")
	v2 := mustCreate(t, db, 1, "第二卷")
	seedChapter(t, db, 1, &v1.ID, 1)
	seedChapter(t, db, 1, &v1.ID, 2)

	err := db.Transaction(func(tx *gorm.DB) error {
		pos, err := s.AllocateChapterSortOrder(ctx, tx, 1, v2.ID)
		if err != nil {
			return err
		}
		if pos != 3 { // 前卷 max(2)+1
			t.Errorf("pos = %d, want 3", pos)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// 空卷 + 存量未分卷章节：退化为全局 max+1（追加末尾），不插到最前。
func TestAllocateEmptyVolumeAfterUnassigned(t *testing.T) {
	db := openVolDB(t)
	ctx := context.Background()
	s := newTestStore(db)
	seedChapter(t, db, 1, nil, 1)
	seedChapter(t, db, 1, nil, 2)
	v := mustCreate(t, db, 1, "新卷")

	err := db.Transaction(func(tx *gorm.DB) error {
		pos, err := s.AllocateChapterSortOrder(ctx, tx, 1, v.ID)
		if err != nil {
			return err
		}
		if pos != 3 {
			t.Errorf("pos = %d, want 3 (append at end)", pos)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// 未分卷（volumeID=0）：全局 max+1。
func TestAllocateUnassigned(t *testing.T) {
	db := openVolDB(t)
	ctx := context.Background()
	s := newTestStore(db)
	v := mustCreate(t, db, 1, "第一卷")
	seedChapter(t, db, 1, &v.ID, 1)
	seedChapter(t, db, 1, nil, 2)

	err := db.Transaction(func(tx *gorm.DB) error {
		pos, err := s.AllocateChapterSortOrder(ctx, tx, 1, 0)
		if err != nil {
			return err
		}
		if pos != 3 {
			t.Errorf("pos = %d, want 3", pos)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// 卷不存在（或不属于该小说）时报 ErrNotFound。
func TestAllocateUnknownVolume(t *testing.T) {
	db := openVolDB(t)
	ctx := context.Background()
	s := newTestStore(db)
	foreign := mustCreate(t, db, 2, "别家卷")

	err := db.Transaction(func(tx *gorm.DB) error {
		_, err := s.AllocateChapterSortOrder(ctx, tx, 1, foreign.ID)
		return err
	})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

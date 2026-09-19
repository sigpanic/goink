//go:build cgo

package mcp_tools_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/chapter"
	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/mcp_tools"
	"github.com/sigpanic/goink/internal/platform"
	"github.com/sigpanic/goink/internal/volume"
	"github.com/sigpanic/goink/internal/writing"
)

// ── 测试环境 ──────────────────────────────────────────────

// edit 落盘后的 file:changed 事件经 emitFileChanged 推送，
// 普通 context 无 wails 运行时键，测试中自动跳过事件推送。

// setupRWEnv 准备真实 DB（sqlite）+ 小说文件目录环境。
// git.ReadFile/WriteFile 是纯文件系统操作，无需 git 二进制。
func setupRWEnv(t *testing.T) (*gorm.DB, mcp_tools.ToolContext, context.Context) {
	t.Helper()
	dataDir := t.TempDir()
	t.Setenv("GOINK_TESTING", "1")
	t.Setenv("GOINK_DATA_DIR", dataDir)
	platform.ResetDataDirCache()
	config.Set(&config.AppConfig{})

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "rw.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&chapter.Chapter{}, &volume.Volume{}, &writing.WritingLog{}); err != nil {
		t.Fatal(err)
	}

	tc := mcp_tools.ToolContext{DB: db, NovelID: 1}
	return db, tc, context.Background()
}

func execRW(t *testing.T, ctx context.Context, tc mcp_tools.ToolContext, name string, args map[string]any) *mcp_tools.ToolResult {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	reg := mcp_tools.NewRegistry(logger)
	mcp_tools.RegisterRWTools(reg)
	b, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	return reg.Execute(ctx, name, b, tc, nil)
}

func execEdit(t *testing.T, ctx context.Context, tc mcp_tools.ToolContext, args map[string]any) *mcp_tools.ToolResult {
	t.Helper()
	return execRW(t, ctx, tc, "edit", args)
}

func execRead(t *testing.T, ctx context.Context, tc mcp_tools.ToolContext, path string) *mcp_tools.ToolResult {
	t.Helper()
	return execRW(t, ctx, tc, "read", map[string]any{"path": path})
}

func seedVolume(t *testing.T, db *gorm.DB, novelID int64, name string, sort int) int64 {
	t.Helper()
	v := volume.Volume{NovelID: novelID, Name: name, SortOrder: sort}
	if err := db.Create(&v).Error; err != nil {
		t.Fatalf("seed volume: %v", err)
	}
	return v.ID
}

func seedChapter(t *testing.T, db *gorm.DB, novelID int64, vid *int64, num, sort int) int64 {
	t.Helper()
	ch := chapter.Chapter{
		NovelID:       novelID,
		ChapterNumber: num,
		VolumeID:      vid,
		SortOrder:     sort,
		Title:         fmt.Sprintf("第%d章", num),
	}
	if err := db.Create(&ch).Error; err != nil {
		t.Fatalf("seed chapter: %v", err)
	}
	return ch.ID
}

func fetchChapter(t *testing.T, db *gorm.DB, id int64) chapter.Chapter {
	t.Helper()
	var ch chapter.Chapter
	if err := db.First(&ch, id).Error; err != nil {
		t.Fatalf("fetch chapter %d: %v", id, err)
	}
	return ch
}

func chapterCount(t *testing.T, db *gorm.DB, novelID int64) int64 {
	t.Helper()
	var n int64
	if err := db.Model(&chapter.Chapter{}).Where("novel_id = ?", novelID).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

func novelFile(t *testing.T, novelID int64, rel string) string {
	t.Helper()
	return filepath.Join(config.NovelDirPath(novelID), filepath.FromSlash(rel))
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func editArgs(path, changeType, content string) map[string]any {
	return map[string]any{
		"path":        path,
		"change_type": changeType,
		"new_content": content,
		"reason":      "测试",
	}
}

func editArgsTitle(path, changeType, content, title string) map[string]any {
	args := editArgs(path, changeType, content)
	args["title"] = title
	return args
}

// ── 新建通道（chapters/{vid}/new.md）──────────────────────

// 新建章节：建记录拿 id → 写物理扁平文件 → 返回 chapter_id/volume_id。
func TestEditNewChannel_CreatesRecordAndFile(t *testing.T) {
	db, tc, ctx := setupRWEnv(t)
	vol := seedVolume(t, db, 1, "第一卷", 1)

	res := execEdit(t, ctx, tc, editArgsTitle(fmt.Sprintf("chapters/%d/new.md", vol), "full_replace", "夜色沉沉。", "夜入皇城"))
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Error)
	}
	if got := res.Data["chapter_id"].(int64); got != 1 {
		t.Errorf("chapter_id = %v, want 1", res.Data["chapter_id"])
	}
	if got := res.Data["volume_id"].(int64); got != vol {
		t.Errorf("volume_id = %v, want %d", res.Data["volume_id"], vol)
	}
	if got := res.Data["path"]; got != "chapters/id_1.md" {
		t.Errorf("path = %v, want chapters/id_1.md", got)
	}

	if got := mustReadFile(t, novelFile(t, 1, "chapters/id_1.md")); got != "夜色沉沉。" {
		t.Errorf("file content = %q", got)
	}

	ch := fetchChapter(t, db, 1)
	if ch.VolumeID == nil || *ch.VolumeID != vol {
		t.Errorf("volume_id = %v, want %d", ch.VolumeID, vol)
	}
	if ch.SortOrder != 1 {
		t.Errorf("sort_order = %d, want 1", ch.SortOrder)
	}
	if ch.ChapterNumber != 1 {
		t.Errorf("chapter_number = %d, want 1", ch.ChapterNumber)
	}
	if ch.Title != "夜入皇城" {
		t.Errorf("title = %q, want 夜入皇城", ch.Title)
	}
}

// 不带卷的 new.md：小说有卷时默认分进最后一卷（贴卷尾）。
func TestEditNewChannel_DefaultLastVolume(t *testing.T) {
	db, tc, ctx := setupRWEnv(t)
	v1 := seedVolume(t, db, 1, "第一卷", 1)
	v2 := seedVolume(t, db, 1, "第二卷", 2)
	seedChapter(t, db, 1, &v1, 1, 1) // 第一卷 1 章
	seedChapter(t, db, 1, &v2, 2, 2) // 第二卷 2 章（最后一卷）

	res := execEdit(t, ctx, tc, editArgs("chapters/new.md", "full_replace", "新章内容。"))
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Error)
	}
	newID := res.Data["chapter_id"].(int64)
	if got := res.Data["volume_id"].(int64); got != v2 {
		t.Errorf("volume_id = %v, want last volume %d", got, v2)
	}

	ch := fetchChapter(t, db, newID)
	if ch.VolumeID == nil || *ch.VolumeID != v2 {
		t.Errorf("volume_id = %v, want %d", ch.VolumeID, v2)
	}
	if ch.SortOrder != 3 { // 贴最后一卷尾：max(1,2)+1
		t.Errorf("sort_order = %d, want 3", ch.SortOrder)
	}
	if ch.ChapterNumber != 3 {
		t.Errorf("chapter_number = %d, want 3", ch.ChapterNumber)
	}
	if ch.Title != "第3章" { // 缺省标题按分配的章节号
		t.Errorf("title = %q, want 第3章", ch.Title)
	}
}

// 不带卷的 new.md：整本书无卷时创建未分卷章节（VolumeID=NULL，未分卷组末尾追加）。
func TestEditNewChannel_NoVolume_Unassigned(t *testing.T) {
	db, tc, ctx := setupRWEnv(t)
	seedChapter(t, db, 1, nil, 1, 1) // 存量未分卷章节 sort=1

	res := execEdit(t, ctx, tc, editArgs("chapters/new.md", "full_replace", "无卷新章。"))
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Error)
	}
	newID := res.Data["chapter_id"].(int64)
	if got := res.Data["volume_id"].(int64); got != 0 {
		t.Errorf("volume_id = %v, want 0 (unassigned)", got)
	}

	ch := fetchChapter(t, db, newID)
	if ch.VolumeID != nil {
		t.Errorf("volume_id = %v, want nil", ch.VolumeID)
	}
	if ch.SortOrder != 2 { // 未分卷组 max+1
		t.Errorf("sort_order = %d, want 2", ch.SortOrder)
	}
	if got := mustReadFile(t, novelFile(t, 1, fmt.Sprintf("chapters/id_%d.md", newID))); got != "无卷新章。" {
		t.Errorf("file content = %q", got)
	}
}

// 大纲对称：不带卷的 outlines/new.md 无卷时创建未分卷章节记录。
func TestEditOutlineNewChannel_NoVolume(t *testing.T) {
	db, tc, ctx := setupRWEnv(t)

	res := execEdit(t, ctx, tc, editArgs("outlines/new.md", "full_replace", "# 未分卷大纲"))
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Error)
	}
	newID := res.Data["chapter_id"].(int64)

	ch := fetchChapter(t, db, newID)
	if ch.VolumeID != nil {
		t.Errorf("volume_id = %v, want nil", ch.VolumeID)
	}
	if ch.SortOrder != 1 || ch.ChapterNumber != 1 {
		t.Errorf("sort_order = %d, chapter_number = %d, want 1/1", ch.SortOrder, ch.ChapterNumber)
	}
	if got := mustReadFile(t, novelFile(t, 1, fmt.Sprintf("outlines/id_%d.md", newID))); got != "# 未分卷大纲" {
		t.Errorf("file content = %q", got)
	}
}

// 卷必须属于当前小说。
func TestEditNewChannel_ForeignVolume(t *testing.T) {
	db, tc, ctx := setupRWEnv(t)
	seedVolume(t, db, 2, "别家卷", 1) // 属于 novel 2

	res := execEdit(t, ctx, tc, editArgs("chapters/1/new.md", "full_replace", "内容"))
	if res.Success {
		t.Fatal("expected failure for foreign volume")
	}
	if !contains(res.Error, "卷不存在") {
		t.Errorf("error = %s", res.Error)
	}
}

// new.md 只支持 full_replace。
func TestEditNewChannel_RequiresFullReplace(t *testing.T) {
	db, tc, ctx := setupRWEnv(t)
	vol := seedVolume(t, db, 1, "第一卷", 1)

	res := execEdit(t, ctx, tc, editArgs(fmt.Sprintf("chapters/%d/new.md", vol), "search_replace", "内容"))
	if res.Success {
		t.Fatal("expected failure for non-full_replace on new.md")
	}
	if !contains(res.Error, "full_replace") {
		t.Errorf("error = %s", res.Error)
	}
}

// ── sort_order 卷内语义 ──────────────────────────────────

// 给前面的卷加章节：只在目标卷内追加，不影响其他卷或未分卷组。
func TestEditNewChannel_AppendsWithinTargetVolume(t *testing.T) {
	db, tc, ctx := setupRWEnv(t)
	vol1 := seedVolume(t, db, 1, "第一卷", 1)
	vol2 := seedVolume(t, db, 1, "第二卷", 2)
	c1 := seedChapter(t, db, 1, &vol1, 1, 1)
	c2 := seedChapter(t, db, 1, &vol2, 2, 2)
	c3 := seedChapter(t, db, 1, nil, 3, 3) // 未分卷

	res := execEdit(t, ctx, tc, editArgs(fmt.Sprintf("chapters/%d/new.md", vol1), "full_replace", "插入卷一的内容"))
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Error)
	}
	newID := res.Data["chapter_id"].(int64)

	newCh := fetchChapter(t, db, newID)
	if newCh.SortOrder != 2 || newCh.ChapterNumber != 4 {
		t.Errorf("new chapter sort_order=%d chapter_number=%d, want 2/4", newCh.SortOrder, newCh.ChapterNumber)
	}
	if got := fetchChapter(t, db, c2).SortOrder; got != 2 {
		t.Errorf("c2 sort_order = %d, want 2 (other volume unchanged)", got)
	}
	if got := fetchChapter(t, db, c3).SortOrder; got != 3 {
		t.Errorf("c3 (unassigned) sort_order = %d, want 3 (unchanged)", got)
	}
	if got := fetchChapter(t, db, c1).SortOrder; got != 1 {
		t.Errorf("c1 sort_order = %d, want 1 (before insert point)", got)
	}
}

// 给末尾的卷加章节：追加到该卷末尾，不影响其他分组。
func TestEditNewChannel_AppendToLastVolume(t *testing.T) {
	db, tc, ctx := setupRWEnv(t)
	vol1 := seedVolume(t, db, 1, "第一卷", 1)
	vol2 := seedVolume(t, db, 1, "第二卷", 2)
	seedChapter(t, db, 1, &vol1, 1, 1)
	seedChapter(t, db, 1, &vol2, 2, 2)
	c3 := seedChapter(t, db, 1, nil, 3, 3)

	res := execEdit(t, ctx, tc, editArgs(fmt.Sprintf("chapters/%d/new.md", vol2), "full_replace", "卷二新章"))
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Error)
	}
	newID := res.Data["chapter_id"].(int64)

	if got := fetchChapter(t, db, newID).SortOrder; got != 3 {
		t.Errorf("new chapter sort_order = %d, want 3", got)
	}
	if got := fetchChapter(t, db, c3).SortOrder; got != 3 {
		t.Errorf("c3 sort_order = %d, want 3 (unassigned unchanged)", got)
	}
}

// 空卷 + 存量未分卷章节：卷内第一章从 1 开始。
func TestEditNewChannel_EmptyVolumeAfterUnassignedChapters(t *testing.T) {
	db, tc, ctx := setupRWEnv(t)
	seedChapter(t, db, 1, nil, 1, 1)
	seedChapter(t, db, 1, nil, 2, 2)
	vol := seedVolume(t, db, 1, "新卷", 1)

	res := execEdit(t, ctx, tc, editArgs(fmt.Sprintf("chapters/%d/new.md", vol), "full_replace", "新卷第一章"))
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Error)
	}
	newID := res.Data["chapter_id"].(int64)

	ch := fetchChapter(t, db, newID)
	if ch.SortOrder != 1 {
		t.Errorf("sort_order = %d, want 1", ch.SortOrder)
	}
	if ch.ChapterNumber != 3 {
		t.Errorf("chapter_number = %d, want 3", ch.ChapterNumber)
	}
}

// ── 已有章节 id 路径 ─────────────────────────────────────

// id 路径要求记录存在，否则引导走 new.md。
func TestEditExistingID_MissingRecord(t *testing.T) {
	_, tc, ctx := setupRWEnv(t)
	res := execEdit(t, ctx, tc, editArgs("chapters/id_99.md", "full_replace", "内容"))
	if res.Success {
		t.Fatal("expected failure for missing chapter record")
	}
	if !contains(res.Error, "章节记录不存在") || !contains(res.Error, "new.md") {
		t.Errorf("error = %s", res.Error)
	}
}

// 已有章节：search_replace 生效 + title 更新。
func TestEditExistingID_SearchReplaceAndTitle(t *testing.T) {
	db, tc, ctx := setupRWEnv(t)
	vol := seedVolume(t, db, 1, "第一卷", 1)
	id := seedChapter(t, db, 1, &vol, 1, 1)
	if err := os.MkdirAll(filepath.Dir(novelFile(t, 1, "chapters/id_1.md")), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(novelFile(t, 1, "chapters/id_1.md"), []byte("陆沉渊走向城门。"), 0o644); err != nil {
		t.Fatal(err)
	}

	args := editArgs(fmt.Sprintf("chapters/id_%d.md", id), "search_replace", "宫门")
	args["search_text"] = "城门"
	args["title"] = "夜入皇城"
	res := execEdit(t, ctx, tc, args)
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Error)
	}

	if got := mustReadFile(t, novelFile(t, 1, "chapters/id_1.md")); got != "陆沉渊走向宫门。" {
		t.Errorf("file content = %q", got)
	}
	if got := fetchChapter(t, db, id).Title; got != "夜入皇城" {
		t.Errorf("title = %q, want 夜入皇城", got)
	}
}

// ── 大纲先行 ─────────────────────────────────────────────

// outlines/{vid}/new.md 建记录 → chapters/id_{id}.md 写正文复用同一条记录。
func TestEditOutlineNewChannel_OutlineFirst(t *testing.T) {
	db, tc, ctx := setupRWEnv(t)
	vol := seedVolume(t, db, 1, "第一卷", 1)

	res := execEdit(t, ctx, tc, editArgsTitle(fmt.Sprintf("outlines/%d/new.md", vol), "full_replace", "# 夜入皇城\n- 场景：皇城", "夜入皇城"))
	if !res.Success {
		t.Fatalf("outline new channel failed: %s", res.Error)
	}
	chapterID := res.Data["chapter_id"].(int64)

	if got := mustReadFile(t, novelFile(t, 1, "outlines/id_1.md")); got == "" {
		t.Error("outline file should exist")
	}
	ch := fetchChapter(t, db, chapterID)
	if ch.Title != "夜入皇城" || ch.SortOrder != 1 {
		t.Errorf("record = %+v, want title=夜入皇城 sort=1", ch)
	}

	// 大纲落定后写正文：同一条记录，不再新建
	res2 := execEdit(t, ctx, tc, editArgs(fmt.Sprintf("chapters/id_%d.md", chapterID), "full_replace", "正文若干字。"))
	if !res2.Success {
		t.Fatalf("body write failed: %s", res2.Error)
	}
	if got := chapterCount(t, db, 1); got != 1 {
		t.Errorf("chapter count = %d, want 1", got)
	}
	if got := mustReadFile(t, novelFile(t, 1, "chapters/id_1.md")); got != "正文若干字。" {
		t.Errorf("body content = %q", got)
	}
	if got := fetchChapter(t, db, chapterID).WordCount; got == 0 {
		t.Error("word_count should be updated after body write")
	}
}

// ── 两级容错路径 ─────────────────────────────────────────

// chapters/{vid}/id_{id}.md 归一化为扁平路径读写，不产生嵌套目录文件。
func TestEditTwoLevelTolerance(t *testing.T) {
	db, tc, ctx := setupRWEnv(t)
	vol := seedVolume(t, db, 1, "第一卷", 1)

	created := execEdit(t, ctx, tc, editArgs(fmt.Sprintf("chapters/%d/new.md", vol), "full_replace", "原始内容。"))
	if !created.Success {
		t.Fatalf("create failed: %s", created.Error)
	}
	id := created.Data["chapter_id"].(int64)

	args := editArgs(fmt.Sprintf("chapters/%d/id_%d.md", vol, id), "search_replace", "修改后的内容。")
	args["search_text"] = "原始内容。"
	res := execEdit(t, ctx, tc, args)
	if !res.Success {
		t.Fatalf("two-level edit failed: %s", res.Error)
	}
	if got := res.Data["path"]; got != fmt.Sprintf("chapters/id_%d.md", id) {
		t.Errorf("path = %v, want flat physical path", got)
	}
	if got := mustReadFile(t, novelFile(t, 1, fmt.Sprintf("chapters/id_%d.md", id))); got != "修改后的内容。" {
		t.Errorf("flat file content = %q", got)
	}
	if _, err := os.Stat(novelFile(t, 1, fmt.Sprintf("chapters/%d", vol))); !os.IsNotExist(err) {
		t.Error("two-level alias must not create nested directories")
	}
}

// ── read ─────────────────────────────────────────────────

func TestReadNewMD_Rejected(t *testing.T) {
	_, tc, ctx := setupRWEnv(t)
	res := execRead(t, ctx, tc, "chapters/1/new.md")
	if res.Success {
		t.Fatal("expected failure reading new.md")
	}
	if !contains(res.Error, "new.md") {
		t.Errorf("error = %s", res.Error)
	}
}

func TestReadChapter_DisplayTitleAndTolerance(t *testing.T) {
	db, tc, ctx := setupRWEnv(t)
	vol := seedVolume(t, db, 1, "第一卷", 1)
	created := execEdit(t, ctx, tc, editArgsTitle(fmt.Sprintf("chapters/%d/new.md", vol), "full_replace", "正文内容。\n第二行。", "夜入皇城"))
	if !created.Success {
		t.Fatalf("create failed: %s", created.Error)
	}
	id := created.Data["chapter_id"].(int64)

	res := execRead(t, ctx, tc, fmt.Sprintf("chapters/id_%d.md", id))
	if !res.Success {
		t.Fatalf("read flat failed: %s", res.Error)
	}
	if got := res.Data["display"]; got != "夜入皇城" {
		t.Errorf("display = %v, want 夜入皇城", got)
	}

	res2 := execRead(t, ctx, tc, fmt.Sprintf("chapters/%d/id_%d.md", vol, id))
	if !res2.Success {
		t.Fatalf("read two-level failed: %s", res2.Error)
	}
	if res2.Data["content"] != res.Data["content"] {
		t.Error("two-level path should read the same flat file")
	}

	// 大纲读取 display 带后缀
	if err := os.MkdirAll(filepath.Dir(novelFile(t, 1, "outlines/id_1.md")), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(novelFile(t, 1, "outlines/id_1.md"), []byte("大纲"), 0o644); err != nil {
		t.Fatal(err)
	}
	res3 := execRead(t, ctx, tc, fmt.Sprintf("outlines/id_%d.md", id))
	if !res3.Success {
		t.Fatalf("read outline failed: %s", res3.Error)
	}
	if got := res3.Data["display"]; got != "夜入皇城（大纲）" {
		t.Errorf("outline display = %v, want 夜入皇城（大纲）", got)
	}
}

// ── 补偿 ─────────────────────────────────────────────────

// 写文件失败（chapters 路径被文件占用导致 MkdirAll 失败）时，
// 补偿删除刚建的章节记录，不留下孤儿记录。
func TestNewChannel_CompensatesOnWriteFailure(t *testing.T) {
	db, tc, ctx := setupRWEnv(t)
	vol := seedVolume(t, db, 1, "第一卷", 1)

	// 把 novel 目录下的 chapters 占位成普通文件 → MkdirAll 失败
	if err := os.MkdirAll(config.NovelDirPath(1), 0o755); err != nil {
		t.Fatal(err)
	}
	blocker := novelFile(t, 1, "chapters")
	if err := os.WriteFile(blocker, []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := execEdit(t, ctx, tc, editArgs(fmt.Sprintf("chapters/%d/new.md", vol), "full_replace", "内容"))
	if res.Success {
		t.Fatal("expected failure when chapters path is blocked")
	}
	if got := chapterCount(t, db, 1); got != 0 {
		t.Errorf("chapter count = %d, want 0 (record compensated)", got)
	}
}

// ── 通用文件流程回归 ─────────────────────────────────────

// goink.md 不触发章节记录逻辑。
func TestEditPlainFile_GoinkMD(t *testing.T) {
	db, tc, ctx := setupRWEnv(t)
	res := execEdit(t, ctx, tc, editArgs("goink.md", "full_replace", "# 故事状态\n- 时间线：…"))
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Error)
	}
	if got := mustReadFile(t, novelFile(t, 1, "goink.md")); got == "" {
		t.Error("goink.md should be written")
	}
	if got := chapterCount(t, db, 1); got != 0 {
		t.Errorf("chapter count = %d, want 0", got)
	}
}

// 旧 num 路径被拒绝并给出新路径提示。
func TestLegacyNumPath_Rejected(t *testing.T) {
	_, tc, ctx := setupRWEnv(t)
	res := execEdit(t, ctx, tc, editArgs("chapters/001.md", "full_replace", "内容"))
	if res.Success {
		t.Fatal("expected failure for legacy num path")
	}
	if !contains(res.Error, "chapters/id_") {
		t.Errorf("error should hint new path format: %s", res.Error)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

// ── review 建议项补充用例 ────────────────────────────────

// 内容未变化但 title 有变化时，仍执行标题更新（no-op 不吞掉 title）。
func TestEditNoOp_TitleOnlyUpdate(t *testing.T) {
	db, tc, ctx := setupRWEnv(t)
	vol := seedVolume(t, db, 1, "第一卷", 1)
	id := seedChapter(t, db, 1, &vol, 1, 1)
	if err := os.MkdirAll(filepath.Dir(novelFile(t, 1, "chapters/id_1.md")), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(novelFile(t, 1, "chapters/id_1.md"), []byte("正文内容。"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := execEdit(t, ctx, tc, editArgsTitle(fmt.Sprintf("chapters/id_%d.md", id), "full_replace", "正文内容。", "夜入皇城"))
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Error)
	}
	if got := fetchChapter(t, db, id).Title; got != "夜入皇城" {
		t.Errorf("title = %q, want 夜入皇城", got)
	}
	if got := mustReadFile(t, novelFile(t, 1, "chapters/id_1.md")); got != "正文内容。" {
		t.Errorf("file should stay unchanged, got %q", got)
	}
}

// title 参数对 outlines/ 路径同样生效。
func TestEditOutlinePath_TitleUpdate(t *testing.T) {
	db, tc, ctx := setupRWEnv(t)
	vol := seedVolume(t, db, 1, "第一卷", 1)
	id := seedChapter(t, db, 1, &vol, 1, 1)
	if err := os.MkdirAll(filepath.Dir(novelFile(t, 1, "outlines/id_1.md")), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(novelFile(t, 1, "outlines/id_1.md"), []byte("# 旧大纲"), 0o644); err != nil {
		t.Fatal(err)
	}

	args := editArgs(fmt.Sprintf("outlines/id_%d.md", id), "full_replace", "# 新大纲")
	args["title"] = "改题大纲"
	res := execEdit(t, ctx, tc, args)
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Error)
	}
	if got := fetchChapter(t, db, id).Title; got != "改题大纲" {
		t.Errorf("title = %q, want 改题大纲", got)
	}
}

// 记录存在但物理文件缺失时，read 报「文件不存在」。
func TestReadChapter_FileMissing(t *testing.T) {
	db, tc, ctx := setupRWEnv(t)
	vol := seedVolume(t, db, 1, "第一卷", 1)
	id := seedChapter(t, db, 1, &vol, 1, 1) // 只建记录，不写文件

	res := execRead(t, ctx, tc, fmt.Sprintf("chapters/id_%d.md", id))
	if res.Success {
		t.Fatal("expected failure for missing file")
	}
	if !contains(res.Error, "文件不存在") {
		t.Errorf("error = %s", res.Error)
	}
}

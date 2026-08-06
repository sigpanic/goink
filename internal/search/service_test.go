package search

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf8"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/chapter"
	"github.com/sigpanic/goink/internal/character"
	"github.com/sigpanic/goink/internal/location"
	"github.com/sigpanic/goink/internal/storage"
	"github.com/sigpanic/goink/internal/storyarc"
	"github.com/sigpanic/goink/internal/timeline"
)

var dbSeq atomic.Int64

func openSearchDB(t *testing.T) *gorm.DB {
	t.Helper()
	name := t.Name() + "_" + strconv.Itoa(int(dbSeq.Add(1)))
	dsn := "file:" + name + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.Exec("PRAGMA foreign_keys = ON")
	if err := db.AutoMigrate(
		&character.Character{},
		&location.Location{},
		&timeline.TimelineEntry{},
		&storyarc.StoryArc{},
		&storyarc.ArcNode{},
		&chapter.Chapter{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func newTestService(db *gorm.DB) *Service {
	logger := testLogger()
	return NewService(
		logger,
		character.NewStore(db, logger),
		location.NewStore(db, logger),
		timeline.NewStore(db, logger),
		storyarc.NewStore(db, logger),
		chapter.NewStore(db, logger),
		nil, // vectorStore
	)
}

// ── buildContext ─────────────────────────────────────────

func TestBuildContext_Basic(t *testing.T) {
	content := "夜色中那人缓步走来，腰间长剑泛着冷光。张三心头一凛，这人的步伐他认得。"
	query := "张三"
	bytePos := strings.Index(content, query)
	matchRunePos := utf8.RuneCountInString(content[:bytePos])
	prefix, hit, _ := buildContext(content, matchRunePos, query)

	if hit != "张三" {
		t.Errorf("expected hit 张三, got: %s", hit)
	}
	if !strings.Contains(prefix, "缓步走来") {
		t.Errorf("expected prefix near hit in context, got: %s", prefix)
	}
}

func TestBuildContext_AtStart(t *testing.T) {
	content := "张三心头一凛，这人的步伐他认得，是当年在华山见过的那人。夜色中那人缓步走来。"
	query := "张三"
	prefix, hit, _ := buildContext(content, 0, query)

	if hit != "张三" {
		t.Errorf("expected hit 张三, got: %s", hit)
	}
	if strings.HasPrefix(prefix, "...") {
		t.Errorf("expected no ellipsis at start, got: %s", prefix)
	}
}

func TestBuildContext_AtEnd(t *testing.T) {
	content := "夜色中那人缓步走来，腰间长剑泛着冷光。来者正是张三"
	query := "张三"
	bytePos := strings.Index(content, query)
	matchRunePos := utf8.RuneCountInString(content[:bytePos])
	_, hit, suffix := buildContext(content, matchRunePos, query)

	if hit != "张三" {
		t.Errorf("expected hit 张三, got: %s", hit)
	}
	if strings.HasSuffix(suffix, "...") {
		t.Errorf("expected no ellipsis at end, got: %s", suffix)
	}
}

// ── searchEntities ───────────────────────────────────────

func TestSearchEntities_Character(t *testing.T) {
	db := openSearchDB(t)
	svc := newTestService(db)
	ctx := context.Background()

	db.Create(&character.Character{NovelID: 1, Name: "张三"})
	db.Create(&character.Character{NovelID: 1, Name: "李四"})
	db.Create(&character.Character{NovelID: 2, Name: "张三丰"})

	results := svc.searchEntities(ctx, 1, "张三")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Type != "character" {
		t.Errorf("expected type character, got %s", results[0].Type)
	}
	if results[0].Title != "张三" {
		t.Errorf("expected 张三, got %s", results[0].Title)
	}
	if results[0].PanelID != "characters" {
		t.Errorf("expected panel characters, got %s", results[0].PanelID)
	}
}

func TestSearchEntities_Location(t *testing.T) {
	db := openSearchDB(t)
	svc := newTestService(db)
	ctx := context.Background()

	db.Create(&location.Location{NovelID: 1, Name: "华山"})
	db.Create(&location.Location{NovelID: 1, Name: "少林寺"})

	results := svc.searchEntities(ctx, 1, "华")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Title != "华山" {
		t.Errorf("expected 华山, got %s", results[0].Title)
	}
}

func TestSearchEntities_Timeline(t *testing.T) {
	db := openSearchDB(t)
	svc := newTestService(db)
	ctx := context.Background()

	db.Create(&timeline.TimelineEntry{NovelID: 1, Category: "foreshadowing", Title: "张三复仇", Content: "复仇之路", TargetChapter: 20, Status: "pending"})

	results := svc.searchEntities(ctx, 1, "复仇")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Type != "timeline" {
		t.Errorf("expected type timeline, got %s", results[0].Type)
	}
	if results[0].Subtitle != "伏笔" {
		t.Errorf("expected 伏笔 subtitle, got %s", results[0].Subtitle)
	}
}

func TestSearchEntities_StoryArc(t *testing.T) {
	db := openSearchDB(t)
	svc := newTestService(db)
	ctx := context.Background()

	db.Create(&storyarc.StoryArc{NovelID: 1, Name: "复仇之路", Description: "主角复仇", ArcType: "main", Status: "active"})

	results := svc.searchEntities(ctx, 1, "复仇")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Type != "storyarc" {
		t.Errorf("expected type storyarc, got %s", results[0].Type)
	}
}

func TestSearchEntities_Chapter(t *testing.T) {
	db := openSearchDB(t)
	svc := newTestService(db)
	ctx := context.Background()

	db.Create(&chapter.Chapter{NovelID: 1, ChapterNumber: 1, Title: "初入江湖"})
	db.Create(&chapter.Chapter{NovelID: 1, ChapterNumber: 2, Title: "华山论剑"})

	results := svc.searchEntities(ctx, 1, "江湖")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Title != "初入江湖" {
		t.Errorf("expected 初入江湖, got %s", results[0].Title)
	}
}

func TestSearchEntities_NoMatch(t *testing.T) {
	db := openSearchDB(t)
	svc := newTestService(db)
	ctx := context.Background()

	db.Create(&character.Character{NovelID: 1, Name: "张三"})

	results := svc.searchEntities(ctx, 1, "不存在的")
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestSearchEntities_Limit(t *testing.T) {
	db := openSearchDB(t)
	svc := newTestService(db)
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		db.Create(&character.Character{NovelID: 1, Name: "测试人"})
	}

	results := svc.searchEntities(ctx, 1, "测试")
	if len(results) > EntityLimit {
		t.Errorf("expected at most %d results, got %d", EntityLimit, len(results))
	}
}

// ── searchRAG ────────────────────────────────────────────

func TestSearchRAG_NilVectorStore(t *testing.T) {
	db := openSearchDB(t)
	svc := newTestService(db)
	ctx := context.Background()

	results := svc.searchRAG(ctx, 1, "测试")
	if results != nil {
		t.Errorf("expected nil results when vectorStore is nil, got %d", len(results))
	}
}

// ── SearchAll ────────────────────────────────────────────

func TestSearchAll_EmptyQuery(t *testing.T) {
	db := openSearchDB(t)
	svc := newTestService(db)
	ctx := context.Background()

	results, err := svc.SearchAll(ctx, 1, "")
	if err != nil {
		t.Fatalf("SearchAll: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil for empty query, got %d results", len(results))
	}
}

func TestSearchAll_WhitespaceQuery(t *testing.T) {
	db := openSearchDB(t)
	svc := newTestService(db)
	ctx := context.Background()

	results, err := svc.SearchAll(ctx, 1, "   ")
	if err != nil {
		t.Fatalf("SearchAll: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil for whitespace query, got %d results", len(results))
	}
}

func TestSearchAll_EntitiesOnly(t *testing.T) {
	db := openSearchDB(t)
	svc := newTestService(db)
	ctx := context.Background()

	db.Create(&character.Character{NovelID: 1, Name: "张三"})
	db.Create(&timeline.TimelineEntry{NovelID: 1, Category: "foreshadowing", Title: "张三出场", Content: "", TargetChapter: 5, Status: "pending"})

	results, err := svc.SearchAll(ctx, 1, "张三")
	if err != nil {
		t.Fatalf("SearchAll: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}

	hasChar := false
	hasTimeline := false
	for _, r := range results {
		if r.Type == "character" {
			hasChar = true
		}
		if r.Type == "timeline" {
			hasTimeline = true
		}
	}
	if !hasChar {
		t.Error("expected character result")
	}
	if !hasTimeline {
		t.Error("expected timeline result")
	}
}

func TestSearchAll_MultipleEntityTypes(t *testing.T) {
	db := openSearchDB(t)
	svc := newTestService(db)
	ctx := context.Background()

	db.Create(&character.Character{NovelID: 1, Name: "华山弟子"})
	db.Create(&location.Location{NovelID: 1, Name: "华山"})
	db.Create(&storyarc.StoryArc{NovelID: 1, Name: "华山论剑", ArcType: "main", Status: "active"})

	results, err := svc.SearchAll(ctx, 1, "华山")
	if err != nil {
		t.Fatalf("SearchAll: %v", err)
	}

	types := make(map[string]int)
	for _, r := range results {
		types[r.Type]++
	}

	if types["character"] < 1 {
		t.Error("expected character result for 华山")
	}
	if types["location"] < 1 {
		t.Error("expected location result for 华山")
	}
	if types["storyarc"] < 1 {
		t.Error("expected storyarc result for 华山")
	}
}

// ── UpdateCachedChapter ──────────────────────────────────

func TestUpdateCachedChapter_NoCache(t *testing.T) {
	db := openSearchDB(t)
	svc := newTestService(db)

	// 不应 panic，缓存未初始化时是 no-op
	svc.UpdateCachedChapter(1, 5, "新内容")
}

func TestSearchAll_NoResults(t *testing.T) {
	db := openSearchDB(t)
	svc := newTestService(db)
	ctx := context.Background()

	db.Create(&character.Character{NovelID: 1, Name: "张三"})

	results, err := svc.SearchAll(ctx, 1, "完全不存在的关键词xyz")
	if err != nil {
		t.Fatalf("SearchAll: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

// ── SearchResult合并顺序 ─────────────────────────────────

func TestSearchAll_ResultOrder(t *testing.T) {
	db := openSearchDB(t)
	svc := newTestService(db)
	ctx := context.Background()

	// 创建多类型数据，验证实体结果在 RAG 之前
	db.Create(&character.Character{NovelID: 1, Name: "测试角色"})
	db.Create(&timeline.TimelineEntry{NovelID: 1, Category: "user_directive", Title: "测试指令", Content: "", TargetChapter: 3, Status: "pending"})

	results, err := svc.SearchAll(ctx, 1, "测试")
	if err != nil {
		t.Fatalf("SearchAll: %v", err)
	}

	// 实体结果应排在前面（character、timeline 等都在 RAG 之前）
	for _, r := range results {
		if r.Type == "rag" {
			break // RAG 开始，后续不应有实体
		}
		if r.Relevance != 0 {
			t.Errorf("entity result should not have relevance set, got %.2f for %s", r.Relevance, r.Title)
		}
	}
}

// ── searchContent ────────────────────────────────────────

func TestSearchContent_NoCache(t *testing.T) {
	db := openSearchDB(t)
	svc := newTestService(db)
	ctx := context.Background()

	// 没有任何缓存，也没有章节文件，应返回空
	results := svc.searchContent(ctx, 1, "测试")
	if len(results) != 0 {
		t.Errorf("expected 0 results without cache, got %d", len(results))
	}
}

// ── searchContent 位置验证 ─────────────────────────────

func TestSearchContent_MatchPosition(t *testing.T) {
	db := openSearchDB(t)
	svc := newTestService(db)
	ctx := context.Background()

	content := "第一章 开端\n\n张三走进了房间，看到李四正在等待。\n\n他缓缓开口：\"你来了。\""
	query := "李四"
	// 查询在 rune 偏移 16 处（"看到"后面）
	queryRunePos := utf8.RuneCountInString(content[:strings.Index(content, query)])

	// 直接注入缓存
	svc.mu.Lock()
	svc.cache[1] = map[int]string{1: content}
	svc.mu.Unlock()

	results := svc.searchContent(ctx, 1, query)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if r.MatchPosition != queryRunePos {
		t.Errorf("MatchPosition = %d, want %d", r.MatchPosition, queryRunePos)
	}
	if r.MatchLen != utf8.RuneCountInString(query) {
		t.Errorf("MatchLen = %d, want %d", r.MatchLen, utf8.RuneCountInString(query))
	}
	if r.Type != "content" {
		t.Errorf("Type = %q, want content", r.Type)
	}
}

func TestSearchContent_MultipleMatches(t *testing.T) {
	db := openSearchDB(t)
	svc := newTestService(db)
	ctx := context.Background()

	content := "张三走来。\n张三离开。"
	query := "张三"

	svc.mu.Lock()
	svc.cache[2] = map[int]string{1: content}
	svc.mu.Unlock()

	results := svc.searchContent(ctx, 2, query)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	// 第一个匹配在位置 0
	if results[0].MatchPosition != 0 {
		t.Errorf("first match position = %d, want 0", results[0].MatchPosition)
	}
	// 第二个匹配在 "张三走来。\n" 之后
	secondPos := utf8.RuneCountInString(content[:strings.LastIndex(content, query)])
	if results[1].MatchPosition != secondPos {
		t.Errorf("second match position = %d, want %d", results[1].MatchPosition, secondPos)
	}
}

// ── Result 字段 ─────────────────────────────────────────

func TestResult_MatchPositionFields(t *testing.T) {
	// 验证 RAG 结果通过 MatchPosition/MatchLen 传递位置信息
	r := Result{
		Type:          "rag",
		MatchPosition: 42,
		MatchLen:      380,
		Relevance:     0.85,
	}
	if r.MatchPosition != 42 {
		t.Errorf("MatchPosition = %d", r.MatchPosition)
	}
	if r.MatchLen != 380 {
		t.Errorf("MatchLen = %d", r.MatchLen)
	}
}

// ── RAG results ──────────────────────────────────────────

func TestSearchRAG_EmptyResults(t *testing.T) {
	db := openSearchDB(t)
	svc := newTestService(db)
	ctx := context.Background()

	// vectorStore 为 nil 时返回 nil
	results := svc.searchRAG(ctx, 999, "查询")
	if results != nil {
		t.Errorf("expected nil, got %d results", len(results))
	}
}

// ── PageParams ───────────────────────────────────────────

func TestPageParams_Normalize(t *testing.T) {
	pp := storage.PageParams{}
	pp.Normalize()
	if pp.Page != 1 {
		t.Errorf("expected page 1, got %d", pp.Page)
	}
	if pp.Size != 0 {
		t.Errorf("expected size 0 (zero passthrough), got %d", pp.Size)
	}

	// 负数归一化为 -1（全量）
	pp = storage.PageParams{Size: -1}
	pp.Normalize()
	if pp.Size != -1 {
		t.Errorf("expected size -1, got %d", pp.Size)
	}
}

// ── 集成测试：模拟真实小说场景 ────────────────────────────────

// novelID 固定为 1，构造一部武侠小说「武林外史」的全部数据并执行搜索。
func TestSearchAll_RealisticNovel(t *testing.T) {
	db := openSearchDB(t)
	svc := newTestService(db)
	ctx := context.Background()
	novelID := int64(1)

	// ── 人物 ────────────────────────────────────────────
	db.Create(&character.Character{NovelID: novelID, Name: "张三", Description: "少年侠客，身负血海深仇"})
	db.Create(&character.Character{NovelID: novelID, Name: "李四", Description: "魔教教主，武功深不可测"})
	db.Create(&character.Character{NovelID: novelID, Name: "王五", Description: "华山派掌门，张三的授业恩师"})
	db.Create(&character.Character{NovelID: novelID, Name: "赵灵儿", Description: "神秘女子，身世成谜"})

	// ── 地点 ────────────────────────────────────────────
	db.Create(&location.Location{NovelID: novelID, Name: "华山", LocationType: "门派", Description: "华山派所在之地"})
	db.Create(&location.Location{NovelID: novelID, Name: "黑木崖", LocationType: "巢穴", Description: "魔教总坛"})
	db.Create(&location.Location{NovelID: novelID, Name: "京城", LocationType: "城市", Description: "天子脚下"})

	// ── 章节 ────────────────────────────────────────────
	db.Create(&chapter.Chapter{NovelID: novelID, ChapterNumber: 1, Title: "初入江湖", Summary: "张三离开华山，踏上复仇之路"})
	db.Create(&chapter.Chapter{NovelID: novelID, ChapterNumber: 2, Title: "黑木崖之战", Summary: "张三潜入魔教总坛，遭遇李四"})
	db.Create(&chapter.Chapter{NovelID: novelID, ChapterNumber: 3, Title: "京城风云", Summary: "张三来到京城，发现更大的阴谋"})

	// ── 时间线 ───────────────────────────────────────────
	db.Create(&timeline.TimelineEntry{
		NovelID: novelID, Category: "foreshadowing", Status: "pending",
		Title: "青龙刀的传说", Content: "王五提起传说中的青龙刀，暗示其与张三的身世有关",
		TargetChapter: 5, SourceChapter: 1, Importance: 5, Source: "ai",
	})
	db.Create(&timeline.TimelineEntry{
		NovelID: novelID, Category: "user_directive", Status: "pending",
		Title: "决战黑木崖", Content: "张三需要在黑木崖与李四展开最终决战",
		TargetChapter: 10, Importance: 4, Source: "user",
	})

	// ── 故事弧 ──────────────────────────────────────────
	arc1 := storyarc.StoryArc{NovelID: novelID, Name: "复仇之路", Description: "张三为报灭门之仇踏上江湖", ArcType: "main", Status: "active", Importance: 5}
	db.Create(&arc1)
	arc2 := storyarc.StoryArc{NovelID: novelID, Name: "寻找青龙刀", Description: "寻找传说中能斩妖除魔的青龙刀", ArcType: "sub", Status: "active", Importance: 3}
	db.Create(&arc2)

	// ── 章节正文缓存（模拟 git 文件） ────────────────────
	svc.mu.Lock()
	svc.cache[novelID] = map[int]string{
		1: `华山之巅，云雾缭绕。
王五站在崖边，负手而立，望着远处的群山出神。
"师父。"张三走到他身后，轻声唤道。
王五没有回头，缓缓开口："你可知我为何收你为徒？"
张三一愣，拱手道："师父慈悲，收留我这孤儿。"
"不。"王五转过身，目光如电，"因为你的父亲，正是当年名震江湖的青龙刀张远山。"
张三心头剧震，双腿一软，几乎站立不住。青龙刀张远山——这个名字他听过无数次，那是二十年前以一己之力斩杀魔教前任教主、平定武林浩劫的传奇人物。
"我父亲……是被人害死的？"
王五点头，眼中闪过一丝悲戚："黑木崖。李四的师父，当年设计暗算了你父亲。"`,
		2: `黑木崖险峻陡峭，常年笼罩在浓雾之中。
张三换上夜行衣，借着月色沿着峭壁攀援而上。他的手指深深扣入岩缝，每一下都带着刻骨的恨意。
崖顶火光闪烁，魔教弟子三五成群，巡逻哨岗密布。
张三轻功卓绝，无声无息地掠过几处明哨，直入总坛大殿。
大殿正中，一人背对着他，正是魔教教主李四。
"等了三年，你终于来了。"李四缓缓转身，嘴角挂着冷笑，"张远山的儿子，果然有胆色。"
张三拔出腰间的龙泉剑，剑尖直指李四："今日便是你的死期，为我父亲报仇。"
李四哈哈一笑，大袖一挥，一股黑色的掌风呼啸而出："就凭你？就连你父亲当年都败在我师父手上！"`,
		3: `京城繁华似锦，车水马龙。
但张三无心欣赏这些，他的心里只有两个字——复仇。
在京城的一处偏僻茶楼里，他见到了那位神秘的女子赵灵儿。
"我知道你在找什么。"赵灵儿轻轻掀开斗笠，露出一张绝世容颜，"李四不是你的最终敌人。"
张三握紧茶杯："什么意思？"
"你父亲的青龙刀至今下落不明，而能驱动这把刀的秘密，只有皇宫里那个人知道。"
"皇帝？"
赵灵儿摇头，压低声音："大内总管魏公公。他才是这一切的幕后黑手。当年害死你父亲的，不光是魔教。"
张三手中的茶杯啪地碎裂，茶水四溅。`,
	}
	svc.mu.Unlock()

	// ── 测试 1：搜索人物名「张三」 ──────────────────────
	t.Run("search protagnist name", func(t *testing.T) {
		results, err := svc.SearchAll(ctx, novelID, "张三")
		if err != nil {
			t.Fatalf("SearchAll: %v", err)
		}
		// 应命中：人物、章节正文、可能章节标题
		if len(results) == 0 {
			t.Fatal("expected results for '张三'")
		}
		foundChar := false
		foundContent := false
		for _, r := range results {
			if r.Type == "character" && r.Title == "张三" {
				foundChar = true
			}
			if r.Type == "content" {
				foundContent = true
				if r.MatchHit == "" {
					t.Error("content result should have match_hit")
				}
				if r.MatchHit != "张三" {
					t.Errorf("match_hit should be 张三, got: %s", r.MatchHit)
				}
			}
		}
		if !foundChar {
			t.Error("expected character result for 张三")
		}
		if !foundContent {
			t.Error("expected content result for 张三")
		}
	})

	// ── 测试 2：搜索地名「华山」 ─────────────────────────
	t.Run("search location name", func(t *testing.T) {
		results, err := svc.SearchAll(ctx, novelID, "华山")
		if err != nil {
			t.Fatalf("SearchAll: %v", err)
		}
		foundLoc := false
		foundContent := false
		for _, r := range results {
			if r.Type == "location" && r.Title == "华山" {
				foundLoc = true
			}
			if r.Type == "content" && r.MatchHit == "华山" {
				foundContent = true
			}
		}
		if !foundLoc {
			t.Error("expected location result for 华山")
		}
		if !foundContent {
			t.Error("expected content result for 华山")
		}
	})

	// ── 测试 3：搜索伏笔关键词「青龙刀」 ─────────────────
	t.Run("search foreshadowing keyword", func(t *testing.T) {
		results, err := svc.SearchAll(ctx, novelID, "青龙刀")
		if err != nil {
			t.Fatalf("SearchAll: %v", err)
		}
		foundTimeline := false
		foundArc := false
		foundContent := false
		for _, r := range results {
			switch r.Type {
			case "timeline":
				if r.Title == "青龙刀的传说" {
					foundTimeline = true
				}
			case "storyarc":
				if strings.Contains(r.Title, "青龙刀") {
					foundArc = true
				}
			case "content":
				if r.MatchHit == "青龙刀" {
					foundContent = true
				}
			}
		}
		if !foundTimeline {
			t.Error("expected timeline result for 青龙刀")
		}
		if !foundArc {
			t.Error("expected storyarc result for 青龙刀")
		}
		if !foundContent {
			t.Error("expected content result for 青龙刀 (mentioned in Zhang San's origin)")
		}
	})

	// ── 测试 4：搜索用户指令 ────────────────────────────
	t.Run("search user directive", func(t *testing.T) {
		results, err := svc.SearchAll(ctx, novelID, "决战")
		if err != nil {
			t.Fatalf("SearchAll: %v", err)
		}
		foundTimeline := false
		for _, r := range results {
			if r.Type == "timeline" && r.Subtitle == "用户指令" {
				foundTimeline = true
			}
		}
		if !foundTimeline {
			t.Error("expected user directive timeline result for 决战")
		}
	})

	// ── 测试 5：结果顺序 ────────────────────────────────
	t.Run("result ordering", func(t *testing.T) {
		results, err := svc.SearchAll(ctx, novelID, "张三")
		if err != nil {
			t.Fatalf("SearchAll: %v", err)
		}
		seenRAG := false
		for _, r := range results {
			if r.Type == "rag" {
				seenRAG = true
			}
			if seenRAG && (r.Type == "character" || r.Type == "chapter" || r.Type == "content") {
				t.Errorf("entity/content result '%s' appears after RAG results", r.Title)
			}
		}
	})

	// ── 测试 6：拼写差异不会命中精确搜索 ─────────────────
	t.Run("spelling mismatch", func(t *testing.T) {
		results, err := svc.SearchAll(ctx, novelID, "张小三")
		if err != nil {
			t.Fatalf("SearchAll: %v", err)
		}
		for _, r := range results {
			if r.Type == "character" {
				t.Errorf("should not find character for 张小三, got %s", r.Title)
			}
		}
	})

	// ── 测试 7：缓存更新后搜索 ───────────────────────────
	t.Run("cache update", func(t *testing.T) {
		svc.UpdateCachedChapter(novelID, 1, "这是新写的第一章内容，里面提到了张三的新武器。")
		results, err := svc.SearchAll(ctx, novelID, "新武器")
		if err != nil {
			t.Fatalf("SearchAll: %v", err)
		}
		found := false
		for _, r := range results {
			if r.Type == "content" && r.MatchHit == "新武器" {
				found = true
			}
		}
		if !found {
			t.Error("expected content result for '新武器' after cache update")
		}
	})

	// ── 测试 8：缓存更新前的旧内容不再出现 ───────────────
	t.Run("old content gone", func(t *testing.T) {
		svc.UpdateCachedChapter(novelID, 1, "完全不同的新内容")
		results, err := svc.SearchAll(ctx, novelID, "王五站在崖边")
		if err != nil {
			t.Fatalf("SearchAll: %v", err)
		}
		for _, r := range results {
			if r.ChapterNum == 1 && r.Type == "content" {
				t.Errorf("chapter 1 content was replaced, should not find old text, got: %s", r.MatchHit)
			}
		}
	})

	// ── 测试 9：不同小说不会串数据 ────────────────────────
	t.Run("different novel isolation", func(t *testing.T) {
		db.Create(&character.Character{NovelID: 99, Name: "独立角色"})
		results, err := svc.SearchAll(ctx, novelID, "独立角色")
		if err != nil {
			t.Fatalf("SearchAll: %v", err)
		}
		for _, r := range results {
			if r.Title == "独立角色" {
				t.Error("should not see character from novel 99")
			}
		}
	})

	// ── 测试 10：正文匹配的上下文居中 ─────────────────────
	t.Run("content context centered", func(t *testing.T) {
		svc.UpdateCachedChapter(novelID, 5, "那年冬天，北风呼啸，张三独自一人走在荒凉的官道上。腰间配着父亲遗留的龙泉剑，剑鞘上斑驳的纹路诉说着岁月的沧桑。他抬头望着远方的天际线，眼神中既有迷茫也有坚定。他知道，这一去黑木崖，生死未卜。")
		results, err := svc.SearchAll(ctx, novelID, "龙泉剑")
		if err != nil {
			t.Fatalf("SearchAll: %v", err)
		}
		found := false
		for _, r := range results {
			if r.Type == "content" && r.MatchHit == "龙泉剑" {
				found = true
				// 验证 hit 不在开头也不是唯一内容
				if r.MatchPrefix == "" && r.MatchSuffix == "" {
					t.Error("context should have surrounding text, not just the keyword")
				}
			}
		}
		if !found {
			t.Error("expected content result for 龙泉剑")
		}
	})
}

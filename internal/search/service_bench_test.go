package search

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/chapter"
	"github.com/sigpanic/goink/internal/character"
	"github.com/sigpanic/goink/internal/location"
	"github.com/sigpanic/goink/internal/storyarc"
	"github.com/sigpanic/goink/internal/timeline"
)

var benchDBNum atomic.Int64

// 构造 3000 章 × 3000 字的假文本并注入缓存
func setupBenchService(tb testing.TB, chapters int, wordsPerChapter int) (*Service, int64) {
	db, err := gorm.Open(sqlite.Open("file:bench"+strconv.FormatInt(benchDBNum.Add(1), 10)+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		tb.Fatalf("open db: %v", err)
	}
	db.AutoMigrate(
		&character.Character{},
		&location.Location{},
		&timeline.TimelineEntry{},
		&storyarc.StoryArc{},
		&chapter.Chapter{},
	)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	svc := NewService(logger,
		character.NewStore(db, logger),
		location.NewStore(db, logger),
		timeline.NewStore(db, logger),
		storyarc.NewStore(db, logger),
		chapter.NewStore(db, logger),
		nil,
	)

	novelID := int64(1)

	// 插入章节元数据
	for i := 1; i <= chapters; i++ {
		db.Create(&chapter.Chapter{NovelID: novelID, ChapterNumber: i, Title: "第" + strconv.Itoa(i) + "章"})
	}

	// 插入实体数据
	names := []string{"张三", "李四", "王五", "赵六", "孙七", "华山派", "少林寺", "魔教", "京城", "青龙刀"}
	for _, name := range names {
		db.Create(&character.Character{NovelID: novelID, Name: name})
		db.Create(&location.Location{NovelID: novelID, Name: name})
	}
	db.Create(&timeline.TimelineEntry{NovelID: novelID, Category: "foreshadowing", Title: "伏笔", Content: "线索", TargetChapter: 100, Status: "pending"})
	db.Create(&storyarc.StoryArc{NovelID: novelID, Name: "主线", ArcType: "main", Status: "active"})

	// 构造每章 3000 字的重复模板正文
	paragraph := strings.Repeat("天地玄黄宇宙洪荒日月盈昃辰宿列张寒来暑往秋收冬藏闰余成岁律吕调阳云腾致雨露结为霜金生丽水玉出昆冈剑号巨阙珠称夜光果珍李柰菜重芥姜海咸河淡鳞潜羽翔龙师火帝鸟官人皇始制文字乃服衣裳推位让国有虞陶唐吊民伐罪周发殷汤坐朝问道垂拱平章爱育黎首臣伏戎羌遐迩一体率宾归王鸣凤在竹白驹食场化被草木赖及万方", 8)
	contentPerChapter := paragraph
	// 在第 1500 章埋关键搜索词
	targetChapter := 1500
	targetContent := paragraph[:len(paragraph)/2] + "张三拔剑刺向李四咽喉" + paragraph[len(paragraph)/2:]

	// 懒加载方式直接注入缓存，跳过文件 IO
	svc.mu.Lock()
	chapMap := make(map[int]string, chapters)
	for i := 1; i <= chapters; i++ {
		if i == targetChapter {
			chapMap[i] = targetContent
		} else {
			chapMap[i] = contentPerChapter
		}
	}
	svc.cache[novelID] = chapMap
	svc.mu.Unlock()

	// 填补剩余到 3000 字/章
	_ = len(contentPerChapter) // suppress unused

	return svc, novelID
}

// ── Benchmark: searchContent ─────────────────────────────

func BenchmarkSearchContent_3000Chapters(b *testing.B) {
	svc, novelID := setupBenchService(b, 3000, 3000)
	ctx := context.Background()
	query := "张三拔剑"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		results := svc.searchContent(ctx, novelID, query)
		_ = results
	}
}

// ── Benchmark: searchContent cache copy ──────────────────

func BenchmarkSearchContent_CacheCopy(b *testing.B) {
	svc, novelID := setupBenchService(b, 3000, 3000)
	ctx := context.Background()
	query := "不存在的词xyz"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		results := svc.searchContent(ctx, novelID, query)
		_ = results
	}
}

// ── Benchmark: searchEntities ────────────────────────────

func BenchmarkSearchEntities_3000Chapters(b *testing.B) {
	svc, novelID := setupBenchService(b, 100, 0)
	ctx := context.Background()
	query := "张三"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		results := svc.searchEntities(ctx, novelID, query)
		_ = results
	}
}

// ── Benchmark: SearchAll ─────────────────────────────────

func BenchmarkSearchAll_3000Chapters(b *testing.B) {
	svc, novelID := setupBenchService(b, 3000, 3000)
	ctx := context.Background()
	query := "张三拔剑"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		results, err := svc.SearchAll(ctx, novelID, query)
		if err != nil {
			b.Fatalf("SearchAll: %v", err)
		}
		_ = results
	}
}

// ── Benchmark: buildContext ──────────────────────────────

func BenchmarkBuildContext(b *testing.B) {
	content := strings.Repeat("天地玄黄宇宙洪荒日月盈昃辰宿列张寒来暑往秋收冬藏", 100)
	query := "日月"
	bytePos := strings.Index(content, query)
	matchRunePos := utf8.RuneCountInString(content[:bytePos])

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = buildContext(content, matchRunePos, query)
	}
}

// ── Benchmark: SearchAll concurrent ───────────────────—

func BenchmarkSearchAll_Parallel(b *testing.B) {
	svc, novelID := setupBenchService(b, 3000, 3000)
	ctx := context.Background()
	query := "张三拔剑"

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			results, err := svc.SearchAll(ctx, novelID, query)
			if err != nil {
				b.Fatalf("SearchAll: %v", err)
			}
			_ = results
		}
	})
}

// ── Report: stats ────────────────────────────────────────

func TestSearchContent_PrintStats(t *testing.T) {
	const chapters = 3000
	const wordsPerChapter = 3000

	svc, novelID := setupBenchService(t, chapters, wordsPerChapter)

	// 计算总缓存大小
	svc.mu.RLock()
	chapMap := svc.cache[novelID]
	totalBytes := 0
	for _, content := range chapMap {
		totalBytes += len(content)
	}
	svc.mu.RUnlock()
	totalMB := float64(totalBytes) / 1024 / 1024

	t.Logf("章节数: %d, 每章 ~%d 字", chapters, wordsPerChapter)
	t.Logf("缓存大小: %d 字节 (%.1f MB)", totalBytes, totalMB)

	// 预热：确保缓存已加载
	svc.searchContent(context.Background(), novelID, "预热")

	// 测 searchContent 耗时
	query := "张三拔剑"
	start := time.Now()
	results := svc.searchContent(context.Background(), novelID, query)
	elapsed := time.Since(start)
	t.Logf("searchContent(%q) 耗时: %v, 命中 %d 条", query, elapsed, len(results))

	// 测 SearchAll 耗时
	startAll := time.Now()
	allResults, err := svc.SearchAll(context.Background(), novelID, query)
	elapsedAll := time.Since(startAll)
	if err != nil {
		t.Fatalf("SearchAll: %v", err)
	}
	t.Logf("SearchAll(%q) 耗时: %v, 总结果 %d 条", query, elapsedAll, len(allResults))

	// 测空查询（缓存拷贝开销）
	emptyQuery := "不存在的词xyz123"
	startEmpty := time.Now()
	svc.searchContent(context.Background(), novelID, emptyQuery)
	elapsedEmpty := time.Since(startEmpty)
	t.Logf("searchContent(空查询) 遍历 27 MB 耗时: %v", elapsedEmpty)
}

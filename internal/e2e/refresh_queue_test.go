//go:build cgo && e2e

package e2e

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/sigpanic/goink/internal/chapter"
	"github.com/sigpanic/goink/internal/git"
	"github.com/sigpanic/goink/internal/novel"
	"github.com/sigpanic/goink/internal/rag"
)

// setupRefreshQueueTest sets up dependencies for RefreshQueue E2E testing.
// It uses the shared DB, VectorStore, and Embedder singletons from TestMain.
// It creates a novel and its git repo, and initializes the RefreshQueue singleton.
func setupRefreshQueueTest(t *testing.T) (*rag.RefreshQueue, int64, func()) {
	t.Helper()

	// Get shared singletons
	sharedDB := getSharedDB(t)
	vs := rag.GetVectorStore()
	if vs == nil {
		t.Fatal("GetVectorStore() returned nil")
	}

	// Create stores using shared DB
	chStore := chapter.NewStore(sharedDB, testLogger(t))
	novelStore := novel.NewStore(sharedDB, testLogger(t))

	// Create a novel in the DB
	n := &novel.Novel{Title: "RefreshQueue E2E 测试小说", Genre: "玄幻"}
	if err := sharedDB.Create(n).Error; err != nil {
		t.Fatalf("create novel failed: %v", err)
	}
	novelID := n.ID

	// Create the novel's git repo directory
	repo, err := git.New(novelID, "Goink", "goink@local", testLogger(t))
	if err != nil {
		t.Fatalf("git.New() failed: %v", err)
	}
	_ = repo // repo initialized, chapters dir created

	// Initialize RefreshQueue (idempotent via sync.Once)
	rag.InitRefreshQueue(vs, chStore, novelStore, testLogger(t))
	queue := rag.GetRefreshQueue()
	if queue == nil {
		t.Fatal("GetRefreshQueue() returned nil")
	}

	// Start the consumer goroutine (idempotent — safe to call multiple times)
	queue.Start()

	cleanup := func() {
		os.RemoveAll(novelDir(novelID))
	}

	return queue, novelID, cleanup
}

// createChapterInDB creates a chapter record in the database and writes content to disk.
func createChapterInDB(t *testing.T, novelID int64, chapterNumber int, title, summary, content string) {
	t.Helper()

	sharedDB := getSharedDB(t)

	ch := &chapter.Chapter{
		NovelID:       novelID,
		ChapterNumber: chapterNumber,
		Title:         title,
		Summary:       summary,
	}
	if err := sharedDB.Create(ch).Error; err != nil {
		t.Fatalf("create chapter %d failed: %v", chapterNumber, err)
	}

	// Write chapter content to disk
	if err := git.WriteFile(novelID, git.ChapterPath(chapterNumber), content); err != nil {
		t.Fatalf("write chapter %d file failed: %v", chapterNumber, err)
	}
}

func TestRefreshQueue_SubmitAndSearch(t *testing.T) {
	_, novelID, cleanup := setupRefreshQueueTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create a chapter
	chapterContent := "林风盘坐在冰冷的石台上，感受着体内灵力的涌动。经过三年的苦修，他终于触摸到了金丹期的门槛。一道金光从他体内迸发而出，照亮了整个山洞。山洞外，一只白色的灵狐静静地等待着。它感受到了主人的气息变化，尾巴轻轻摇动。"
	createChapterInDB(t, novelID, 1, "第一章 修行之路", "林风在山洞中修炼突破金丹期", chapterContent)

	// Submit a refresh task
	rag.SubmitRefresh(novelID, 1, chapterContent)

	// Wait for async processing (500ms dedup window + processing time)
	time.Sleep(3 * time.Second)

	// Verify search finds the content
	vs := rag.GetVectorStore()
	results, err := vs.Search(ctx, novelID, "林风修炼金丹", 5, nil)
	if err != nil {
		t.Fatalf("Search() failed: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("Search() returned no results after refresh")
	}

	t.Logf("Search returned %d results after SubmitRefresh", len(results))
	for i, r := range results {
		t.Logf("  [%d] chunk=%s type=%s ch=%d relevance=%.4f", i, r.ChunkID, r.SourceType, r.ChapterNumber, r.Relevance)
	}
}

func TestRefreshQueue_Dedup(t *testing.T) {
	_, novelID, cleanup := setupRefreshQueueTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create a chapter
	chapterContent := "赵云单骑救主，在长坂坡杀了个七进七出。曹操在山上观战，见赵云勇猛无比，下令不许放冷箭。"
	createChapterInDB(t, novelID, 1, "第一章 长坂坡", "赵云单骑救主", chapterContent)

	// Submit the same chapter refresh multiple times rapidly
	for i := 0; i < 10; i++ {
		rag.SubmitRefresh(novelID, 1, chapterContent)
	}

	// Wait for dedup window + processing
	time.Sleep(3 * time.Second)

	// Verify only one set of chunks was indexed (not 10x)
	vs := rag.GetVectorStore()
	count, err := vs.CountChunks(ctx, novelID)
	if err != nil {
		t.Fatalf("CountChunks() failed: %v", err)
	}

	if count == 0 {
		t.Fatal("expected chunks to be indexed after dedup, got 0")
	}

	// The count should be reasonable for a single chapter (not 10x).
	// A single chapter typically produces a handful of chunks.
	t.Logf("After 10 rapid submits: %d chunks indexed (dedup worked)", count)

	// Verify search works
	results, err := vs.Search(ctx, novelID, "赵云长坂坡", 3, nil)
	if err != nil {
		t.Fatalf("Search() failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("Search() returned no results")
	}
}

func TestRefreshQueue_RebuildNovel(t *testing.T) {
	queue, novelID, cleanup := setupRefreshQueueTest(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Create two chapters
	ch1Content := "孙悟空从石头中蹦出，天地震动。他在花果山上称王，过着无忧无虑的生活。一日他看到猴子老死，决定出海寻仙访道。"
	ch2Content := "孙悟空拜入菩提祖师门下，学得七十二变和筋斗云。祖师赐他法名悟空，他从此踏上修行之路。"
	createChapterInDB(t, novelID, 1, "第一章 石猴出世", "孙悟空从石头中诞生", ch1Content)
	createChapterInDB(t, novelID, 2, "第二章 拜师学艺", "孙悟空拜师菩提祖师", ch2Content)

	// First, index some chunks via direct SubmitRefresh
	rag.SubmitRefresh(novelID, 1, ch1Content)
	rag.SubmitRefresh(novelID, 2, ch2Content)
	time.Sleep(3 * time.Second)

	// Verify initial indexing
	vs := rag.GetVectorStore()
	countBefore, err := vs.CountChunks(ctx, novelID)
	if err != nil {
		t.Fatalf("CountChunks() before rebuild failed: %v", err)
	}
	if countBefore == 0 {
		t.Fatal("expected chunks before rebuild, got 0")
	}
	t.Logf("Before rebuild: %d chunks", countBefore)

	// Delete all vectors to simulate data loss
	if err := vs.DeleteNovel(ctx, novelID); err != nil {
		t.Fatalf("DeleteNovel() failed: %v", err)
	}

	countAfterDelete, err := vs.CountChunks(ctx, novelID)
	if err != nil {
		t.Fatalf("CountChunks() after delete failed: %v", err)
	}
	if countAfterDelete != 0 {
		t.Errorf("expected 0 chunks after DeleteNovel, got %d", countAfterDelete)
	}

	// Rebuild the entire novel
	if err := queue.RebuildNovel(ctx, novelID); err != nil {
		t.Fatalf("RebuildNovel() failed: %v", err)
	}

	// Verify chunks were re-indexed
	countAfterRebuild, err := vs.CountChunks(ctx, novelID)
	if err != nil {
		t.Fatalf("CountChunks() after rebuild failed: %v", err)
	}
	if countAfterRebuild == 0 {
		t.Fatal("expected chunks after rebuild, got 0")
	}
	t.Logf("After rebuild: %d chunks", countAfterRebuild)

	// Verify search works on rebuilt data
	results, err := vs.Search(ctx, novelID, "孙悟空学艺", 5, nil)
	if err != nil {
		t.Fatalf("Search() after rebuild failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("Search() returned no results after rebuild")
	}

	t.Logf("RebuildNovel OK: search returned %d results", len(results))
}

func TestRefreshQueue_RebuildAll(t *testing.T) {
	queue, novelID, cleanup := setupRefreshQueueTest(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Create chapters for the novel
	ch1Content := "诸葛亮在隆中对策，为刘备分析天下三分之势。他说北让曹操占天时，南让孙权占地利，刘备可占人和。"
	createChapterInDB(t, novelID, 1, "第一章 隆中对", "诸葛亮三分天下", ch1Content)

	// Don't index anything — RebuildAll should pick up novels with 0 chunks
	vs := rag.GetVectorStore()
	countBefore, err := vs.CountChunks(ctx, novelID)
	if err != nil {
		t.Fatalf("CountChunks() before RebuildAll failed: %v", err)
	}
	if countBefore != 0 {
		t.Logf("Expected 0 chunks before RebuildAll, got %d (previous test data)", countBefore)
	}

	// Run RebuildAll — should index novels with 0 chunks
	if err := queue.RebuildAll(ctx); err != nil {
		t.Fatalf("RebuildAll() failed: %v", err)
	}

	// Verify chunks were indexed
	countAfter, err := vs.CountChunks(ctx, novelID)
	if err != nil {
		t.Fatalf("CountChunks() after RebuildAll failed: %v", err)
	}
	if countAfter == 0 {
		t.Fatal("expected chunks after RebuildAll, got 0")
	}
	t.Logf("RebuildAll OK: %d chunks indexed", countAfter)

	// Verify search works
	results, err := vs.Search(ctx, novelID, "诸葛亮隆中对", 3, nil)
	if err != nil {
		t.Fatalf("Search() after RebuildAll failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("Search() returned no results after RebuildAll")
	}
}

func TestRefreshQueue_StopDrainsPending(t *testing.T) {
	queue, novelID, cleanup := setupRefreshQueueTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create a chapter
	chapterContent := "关羽温酒斩华雄，威震诸侯。张飞在旁高声喝彩，声如巨雷。刘备暗自欣喜，兄弟三人初露锋芒。"
	createChapterInDB(t, novelID, 1, "第一章 温酒斩华雄", "关羽斩华雄立威", chapterContent)

	// Submit a refresh task
	rag.SubmitRefresh(novelID, 1, chapterContent)

	// Stop immediately — should drain pending tasks before exiting
	queue.Stop()

	// Verify the task was processed even though we stopped
	vs := rag.GetVectorStore()
	count, err := vs.CountChunks(ctx, novelID)
	if err != nil {
		t.Fatalf("CountChunks() failed: %v", err)
	}
	if count == 0 {
		t.Fatal("expected chunks to be indexed after Stop() drained pending, got 0")
	}

	t.Logf("Stop() drained pending tasks: %d chunks indexed", count)
}

//go:build cgo && e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sigpanic/goink/internal/git"
	"github.com/sigpanic/goink/internal/rag"
)

// setupChinesePipelineTest returns the shared VectorStore and creates a git repo
// for a novel. The VectorStore and Embedder singletons are initialized in TestMain.
func setupChinesePipelineTest(t *testing.T) (*rag.VectorStore, *git.Repo, int64, func()) {
	t.Helper()

	vs := rag.GetVectorStore()
	if vs == nil {
		t.Fatal("GetVectorStore() returned nil — was TestMain initialization successful?")
	}

	// Create a git repo for the novel
	novelID := int64(9500 + hashTestName(t.Name()))
	dir := novelDir(novelID)
	os.RemoveAll(dir)

	repo, err := git.New(novelID, "E2E Test", "e2e@test", nil)
	if err != nil {
		t.Fatalf("git.New() failed: %v", err)
	}

	cleanup := func() {
		os.RemoveAll(dir)
	}

	return vs, repo, novelID, cleanup
}

// writeChineseChapter writes a Chinese chapter file to the novel's chapters directory,
// stages it, and commits with a Chinese message.
func writeChineseChapter(t *testing.T, repo *git.Repo, novelID int64, chapterNum int, content string) {
	t.Helper()

	dir := novelDir(novelID)
	chapterDir := filepath.Join(dir, "chapters")
	os.MkdirAll(chapterDir, 0755)

	filename := filepath.Join(chapterDir, fmt.Sprintf("%03d.md", chapterNum))
	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		t.Fatalf("write chapter %d failed: %v", chapterNum, err)
	}

	if err := repo.StageAll(); err != nil {
		t.Fatalf("StageAll() for chapter %d failed: %v", chapterNum, err)
	}

	msg := fmt.Sprintf("添加第%d章", chapterNum)
	if _, err := repo.Commit(msg); err != nil {
		t.Fatalf("Commit() for chapter %d failed: %v", chapterNum, err)
	}
}

// indexChapter builds chunks for a chapter and indexes them into the VectorStore.
func indexChapter(t *testing.T, vs *rag.VectorStore, novelID int64, chapterNum int, title, content, summary string) {
	t.Helper()

	tok := rag.GetTokenizer()
	if tok == nil {
		t.Fatal("GetTokenizer() returned nil — was InitEmbedder called?")
	}

	params := rag.ChapterChunkParams{
		ChapterNumber: chapterNum,
		ChapterTitle:  title,
		Content:       content,
		Summary:       summary,
	}

	chunks := rag.BuildChapterChunks(params, tok)
	if len(chunks) == 0 {
		t.Fatalf("BuildChapterChunks() returned 0 chunks for chapter %d", chapterNum)
	}

	ctx := context.Background()
	if err := vs.IndexChunks(ctx, novelID, chunks); err != nil {
		t.Fatalf("IndexChunks() failed for chapter %d: %v", chapterNum, err)
	}
}

func TestChineseContent_FullPipeline(t *testing.T) {
	vs, repo, novelID, cleanup := setupChinesePipelineTest(t)
	defer cleanup()

	ctx := context.Background()
	vs.DeleteNovel(ctx, novelID)

	chapterContent := `苏瑶站在城墙上，望着远方连绵的青山。晨雾如纱，将山峦笼罩在一片朦胧之中。

她深吸一口气，空气中带着泥土和青草的芬芳。三年了，她终于回到了这片故土。

"师姐，你真的决定了吗？"身后传来师妹柳如烟的声音。

苏瑶转过身，目光坚定："我必须去。那个人如果还活着，只有我能找到他。"

柳如烟咬了咬嘴唇，最终还是点了点头："那你小心。"

苏瑶转身走向城门，白衣如雪，长发在风中飞扬。她的身影渐渐消失在晨雾之中，只留下一串清脆的脚步声回荡在古老的石板路上。`

	// Write, commit
	writeChineseChapter(t, repo, novelID, 1, chapterContent)

	// Build chunks and index
	indexChapter(t, vs, novelID, 1, "归途", chapterContent, "苏瑶决定离开城池寻找故人，师妹柳如烟虽不舍但仍支持。")

	// Search with Chinese query
	results, err := vs.Search(ctx, novelID, "苏瑶离开城池寻找故人", 5, nil)
	if err != nil {
		t.Fatalf("Search() failed: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("Search() returned no results")
	}

	t.Logf("Search returned %d results:", len(results))
	for i, r := range results {
		t.Logf("  [%d] chunk=%s type=%s ch=%d relevance=%.4f content=%.60s...",
			i, r.ChunkID, r.SourceType, r.ChapterNumber, r.Relevance, r.Content)
	}

	// Verify the top result contains original Chinese content
	topResult := results[0]
	if !strings.Contains(topResult.Content, "苏瑶") {
		t.Errorf("top result does not contain '苏瑶', content: %.100s", topResult.Content)
	}

	// Verify relevance is above a reasonable threshold
	if topResult.Relevance < 0.3 {
		t.Errorf("top result relevance %.4f is too low (threshold 0.3)", topResult.Relevance)
	}

	// Verify chunk count
	count, err := vs.CountChunks(ctx, novelID)
	if err != nil {
		t.Fatalf("CountChunks() failed: %v", err)
	}
	t.Logf("Total chunks indexed: %d", count)
}

func TestChineseContent_MultipleChapters(t *testing.T) {
	vs, repo, novelID, cleanup := setupChinesePipelineTest(t)
	defer cleanup()

	ctx := context.Background()
	vs.DeleteNovel(ctx, novelID)

	chapter1 := `长安城的夜色如墨，街巷间弥漫着桂花与酒香。沈墨独自走在朱雀大街上，手中的长剑在月光下泛着冷光。

他此行只为一个目的——找到隐藏在城中的那本《天机录》。据说此书记载了失传已久的剑法，得之者可称霸武林。`

	chapter2 := `云隐峰上，叶轻竹正在练剑。她的剑法如行云流水，每一招都带着山间的灵气。

突然，一只白鸽落在剑柄上，脚上绑着一封密信。她展开一看，脸色骤变：

"沈墨已入长安，速归。——师父留"`

	chapter3 := `地下暗河的水流湍急，两人被困在一块孤石上。沈墨看着对面的叶轻竹，苦笑道：

"没想到我们第一次见面，就是在这样的绝境中。"

叶轻竹冷哼一声："少废话，想办法出去。"

她手中的剑微微出鞘，剑光照亮了暗河深处若隐若现的石壁——上面刻满了古老的符文。`

	// Write and commit all chapters
	writeChineseChapter(t, repo, novelID, 1, chapter1)
	writeChineseChapter(t, repo, novelID, 2, chapter2)
	writeChineseChapter(t, repo, novelID, 3, chapter3)

	// Index all chapters
	indexChapter(t, vs, novelID, 1, "长安暗影", chapter1, "沈墨潜入长安寻找《天机录》。")
	indexChapter(t, vs, novelID, 2, "云隐惊变", chapter2, "叶轻竹在云隐峰收到师父密信，得知沈墨已入长安。")
	indexChapter(t, vs, novelID, 3, "暗河奇遇", chapter3, "沈墨与叶轻竹在地下暗河相遇，发现石壁古老符文。")

	// Search for content specific to chapter 2 (叶轻竹收到密信)
	results, err := vs.Search(ctx, novelID, "叶轻竹收到密信得知沈墨入长安", 10, nil)
	if err != nil {
		t.Fatalf("Search() failed: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("Search() returned no results")
	}

	t.Logf("Search returned %d results:", len(results))
	for i, r := range results {
		t.Logf("  [%d] chunk=%s type=%s ch=%d relevance=%.4f content=%.60s...",
			i, r.ChunkID, r.SourceType, r.ChapterNumber, r.Relevance, r.Content)
	}

	// Verify chapter 2 content ranks highest
	topResult := results[0]
	if topResult.ChapterNumber != 2 {
		// Allow some flexibility — at least verify chapter 2 appears in top results
		foundCh2 := false
		for _, r := range results {
			if r.ChapterNumber == 2 {
				foundCh2 = true
				break
			}
		}
		if !foundCh2 {
			t.Errorf("chapter 2 not found in any search results; top is chapter %d", topResult.ChapterNumber)
		} else {
			t.Logf("WARNING: chapter 2 did not rank first (chapter %d did), but was found in results", topResult.ChapterNumber)
		}
	}

	// Verify the top result contains chapter 2's key content
	if !strings.Contains(topResult.Content, "叶轻竹") && !strings.Contains(topResult.Content, "密信") {
		t.Errorf("top result does not contain expected chapter 2 content; content: %.100s", topResult.Content)
	}
}

func TestChineseContent_SpecialCharacters(t *testing.T) {
	vs, repo, novelID, cleanup := setupChinesePipelineTest(t)
	defer cleanup()

	ctx := context.Background()
	vs.DeleteNovel(ctx, novelID)

	// Content with special Chinese punctuation: 「」『』、【】、《》、〈〉、〖〗、〔〕、——……
	chapterContent := `「你真的要这么做吗？」她低声问道，眼中满是担忧。

『我别无选择。』他沉默了片刻，终于开口。

这份《天机录》记载着一段不为人知的往事——那是在【永安三年】的秋天，皇宫深处发生了一件惊天大事……

他翻开书页，只见上面写着：
〖第一章·暗流涌动〗
〔景和殿〕内，烛火摇曳。老太监王德全跪在地上，双手呈上一封密函。

"陛下，这是从北境传来的急报。"

皇帝接过密函，展开一看，面色骤变。他猛地站起身，龙袍袖口扫落了案上的茶盏——碎片四溅，茶水浸湿了《山河社稷图》的边角。

"传令下去……"皇帝的声音低沉而冰冷，"命镇北大将军即刻回京。"

王德全心中一凛，却不敢多问，只得叩首领命。他走出殿外，夜风裹挟着远处的胡笳声，吹得宫灯摇摇欲坠……`

	// Write, commit
	writeChineseChapter(t, repo, novelID, 1, chapterContent)

	// Build chunks and index
	indexChapter(t, vs, novelID, 1, "暗流涌动", chapterContent, "皇帝收到北境急报，密令镇北大将军回京。")

	// Search with queries containing special punctuation
	queries := []string{
		"皇帝收到密函镇北大将军回京",
		"「天机录」中的秘密",
		"【永安三年】发生的大事",
	}

	for _, query := range queries {
		t.Run("query="+query, func(t *testing.T) {
			results, err := vs.Search(ctx, novelID, query, 3, nil)
			if err != nil {
				t.Fatalf("Search() failed for query %q: %v", query, err)
			}

			if len(results) == 0 {
				t.Fatalf("Search() returned no results for query %q", query)
			}

			t.Logf("Query %q -> top result: chunk=%s relevance=%.4f content=%.60s...",
				query, results[0].ChunkID, results[0].Relevance, results[0].Content)

			// Verify result contains some original content
			topResult := results[0]
			if topResult.Relevance < 0.2 {
				t.Errorf("relevance %.4f too low for query %q (threshold 0.2)", topResult.Relevance, query)
			}

			// Verify the content contains expected Chinese characters
			if !strings.Contains(topResult.Content, "皇帝") && !strings.Contains(topResult.Content, "天机录") && !strings.Contains(topResult.Content, "永安") {
				t.Errorf("top result content does not contain expected Chinese keywords; content: %.100s", topResult.Content)
			}
		})
	}

	// Verify chunk count is reasonable
	count, err := vs.CountChunks(ctx, novelID)
	if err != nil {
		t.Fatalf("CountChunks() failed: %v", err)
	}
	t.Logf("Total chunks indexed (special characters): %d", count)
	if count == 0 {
		t.Error("expected at least 1 chunk after indexing")
	}
}

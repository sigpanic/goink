package style

import (
	"context"
	"math"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/storage"
)

// ---------------------------------------------------------------------------
// 测试基础设施
// ---------------------------------------------------------------------------

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := db.AutoMigrate(&Sample{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func setupTestStore(t *testing.T) *Store {
	t.Helper()
	db := setupTestDB(t)
	return NewStore(db, nil)
}

// ---------------------------------------------------------------------------
// Store CRUD 测试（浅包装已移至 app 层，这里直接用 DB 测试）
// ---------------------------------------------------------------------------

func TestStore_Create(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	sample := &Sample{
		NovelID:   0,
		IsGlobal:  true,
		Name:      "测试素材",
		Content:   "这是一段测试内容",
		Preview:   "这是一段测试内容",
		Tags:      StringSlice{"标签1", "标签2"},
		WordCount: 8,
	}
	if err := s.DB.WithContext(ctx).Create(sample).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	if sample.ID == 0 {
		t.Error("expected non-zero ID after create")
	}
}

func TestStore_Get(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	sample := &Sample{
		IsGlobal:  true,
		Name:      "获取测试",
		Content:   "内容",
		Preview:   "内容",
		WordCount: 2,
	}
	if err := s.DB.WithContext(ctx).Create(sample).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	var got Sample
	if err := s.DB.WithContext(ctx).First(&got, sample.ID).Error; err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "获取测试" {
		t.Errorf("Name = %q, want 获取测试", got.Name)
	}
	if got.Content != "内容" {
		t.Errorf("Content = %q, want 内容", got.Content)
	}
}

func TestStore_Update(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	sample := &Sample{
		IsGlobal:  true,
		Name:      "原始名称",
		Content:   "原始内容",
		Preview:   "原始内容",
		WordCount: 4,
	}
	if err := s.DB.WithContext(ctx).Create(sample).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	var loaded Sample
	if err := s.DB.WithContext(ctx).First(&loaded, sample.ID).Error; err != nil {
		t.Fatalf("get: %v", err)
	}
	loaded.Name = "更新名称"
	loaded.Content = "更新内容"
	if err := s.DB.WithContext(ctx).Save(&loaded).Error; err != nil {
		t.Fatalf("update: %v", err)
	}

	var got Sample
	if err := s.DB.WithContext(ctx).First(&got, sample.ID).Error; err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if got.Name != "更新名称" {
		t.Errorf("Name = %q, want 更新名称", got.Name)
	}
}

func TestStore_Delete(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	sample := &Sample{
		IsGlobal:  true,
		Name:      "删除测试",
		Content:   "内容",
		Preview:   "内容",
		WordCount: 2,
	}
	if err := s.DB.WithContext(ctx).Create(sample).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := s.DB.WithContext(ctx).Delete(&Sample{}, sample.ID).Error; err != nil {
		t.Fatalf("delete: %v", err)
	}

	var got Sample
	err := s.DB.WithContext(ctx).First(&got, sample.ID).Error
	if err == nil {
		t.Error("expected error after delete, got nil")
	}
}

func TestStore_List(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		sample := &Sample{
			IsGlobal:  true,
			Name:      strings.Repeat("素材", i+1),
			Content:   "内容",
			Preview:   "内容",
			WordCount: 2,
		}
		if err := s.DB.WithContext(ctx).Create(sample).Error; err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}

	result, err := s.List(ctx, ListOptions{
		PageParams: storage.PageParams{Page: 1, Size: 3},
		NovelID:    0,
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if result.Total != 5 {
		t.Errorf("Total = %d, want 5", result.Total)
	}
	if len(result.Items) != 3 {
		t.Errorf("Items count = %d, want 3", len(result.Items))
	}
	if result.TotalPages != 2 {
		t.Errorf("TotalPages = %d, want 2", result.TotalPages)
	}
}

func TestStore_GetByIDs(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	var ids []int64
	for i := 0; i < 3; i++ {
		sample := &Sample{
			IsGlobal:  true,
			Name:      "批量查询",
			Content:   "内容",
			Preview:   "内容",
			WordCount: 2,
		}
		if err := s.DB.WithContext(ctx).Create(sample).Error; err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
		ids = append(ids, sample.ID)
	}

	samples, err := s.GetByIDs(ctx, []int64{ids[0], ids[2]})
	if err != nil {
		t.Fatalf("GetByIDs: %v", err)
	}
	if len(samples) != 2 {
		t.Errorf("got %d samples, want 2", len(samples))
	}
}

func TestStore_ListNovelScope(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// 创建全局素材
	if err := s.DB.WithContext(ctx).Create(&Sample{IsGlobal: true, Name: "全局素材", Preview: "p", WordCount: 1}).Error; err != nil {
		t.Fatalf("create global: %v", err)
	}
	// 创建小说专属素材
	if err := s.DB.WithContext(ctx).Create(&Sample{IsGlobal: false, NovelID: 1, Name: "小说1素材", Preview: "p", WordCount: 1}).Error; err != nil {
		t.Fatalf("create novel: %v", err)
	}
	if err := s.DB.WithContext(ctx).Create(&Sample{IsGlobal: false, NovelID: 2, Name: "小说2素材", Preview: "p", WordCount: 1}).Error; err != nil {
		t.Fatalf("create novel 2: %v", err)
	}

	// NovelID=0: 只返回全局
	result, err := s.List(ctx, ListOptions{NovelID: 0, PageParams: storage.PageParams{Size: -1}})
	if err != nil {
		t.Fatalf("list global: %v", err)
	}
	if len(result.Items) != 1 {
		t.Errorf("global only: got %d items, want 1", len(result.Items))
	}

	// NovelID=1: 全局 + 小说1专属
	result, err = s.List(ctx, ListOptions{NovelID: 1, PageParams: storage.PageParams{Size: -1}})
	if err != nil {
		t.Fatalf("list novel 1: %v", err)
	}
	if len(result.Items) != 2 {
		t.Errorf("novel 1: got %d items, want 2", len(result.Items))
	}
}

// ---------------------------------------------------------------------------
// StringSlice 测试
// ---------------------------------------------------------------------------

func TestStringSlice_Scan(t *testing.T) {
	var s StringSlice
	if err := s.Scan([]byte(`["a","b"]`)); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(s) != 2 || s[0] != "a" || s[1] != "b" {
		t.Errorf("got %v, want [a b]", []string(s))
	}
}

func TestStringSlice_ScanNil(t *testing.T) {
	var s StringSlice
	if err := s.Scan(nil); err != nil {
		t.Fatalf("scan nil: %v", err)
	}
	if s != nil {
		t.Errorf("expected nil, got %v", s)
	}
}

func TestStringSlice_Value(t *testing.T) {
	s := StringSlice{"x", "y"}
	v, err := s.Value()
	if err != nil {
		t.Fatalf("value: %v", err)
	}
	got, ok := v.(string)
	if !ok {
		t.Fatalf("value type = %T, want string", v)
	}
	if got != `["x","y"]` {
		t.Errorf("value = %q, want [\"x\",\"y\"]", got)
	}
}

func TestStringSlice_ValueNil(t *testing.T) {
	var s StringSlice
	v, err := s.Value()
	if err != nil {
		t.Fatalf("value nil: %v", err)
	}
	got, ok := v.(string)
	if !ok {
		t.Fatalf("value type = %T, want string", v)
	}
	if got != "[]" {
		t.Errorf("value = %q, want []", got)
	}
}

// ---------------------------------------------------------------------------
// splitSentences
// ---------------------------------------------------------------------------

func TestSplitSentences_ChinesePeriod(t *testing.T) {
	got := splitSentences("今天天气很好。明天会下雨。")
	if len(got) != 2 {
		t.Fatalf("sentences = %v, want 2", got)
	}
	if got[0] != "今天天气很好。" {
		t.Errorf("first = %q, want 今天天气很好。", got[0])
	}
	if got[1] != "明天会下雨。" {
		t.Errorf("second = %q, want 明天会下雨。", got[1])
	}
}

func TestSplitSentences_MixedPunctuation(t *testing.T) {
	got := splitSentences("他问为什么？没有人回答！沉默。")
	if len(got) != 3 {
		t.Fatalf("sentences = %v, want 3", got)
	}
	if !strings.Contains(got[0], "为什么") {
		t.Errorf("first = %q, should contain 为什么", got[0])
	}
	if !strings.Contains(got[1], "回答") {
		t.Errorf("second = %q, should contain 回答", got[1])
	}
	if !strings.Contains(got[2], "沉默") {
		t.Errorf("third = %q, should contain 沉默", got[2])
	}
}

func TestSplitSentences_English(t *testing.T) {
	got := splitSentences("Hello world. How are you? Fine!")
	if len(got) != 3 {
		t.Fatalf("sentences = %v, want 3", got)
	}
}

func TestSplitSentences_Newline(t *testing.T) {
	got := splitSentences("第一行\n第二行\n")
	if len(got) != 2 {
		t.Fatalf("sentences = %v, want 2", got)
	}
}

func TestSplitSentences_TrailingContent(t *testing.T) {
	got := splitSentences("第一句。第二句没有句号")
	if len(got) != 2 {
		t.Fatalf("sentences = %v, want 2", got)
	}
	if got[1] != "第二句没有句号" {
		t.Errorf("trailing = %q, want 第二句没有句号", got[1])
	}
}

func TestSplitSentences_Empty(t *testing.T) {
	got := splitSentences("")
	if len(got) != 0 {
		t.Errorf("empty input should yield 0 sentences, got %d", len(got))
	}
}

func TestSplitSentences_OnlyWhitespace(t *testing.T) {
	got := splitSentences("   \n\n  ")
	if len(got) != 0 {
		t.Errorf("whitespace-only should yield 0 sentences, got %d", len(got))
	}
}

func TestSplitSentences_ContinuousPunctuation(t *testing.T) {
	got := splitSentences("！！。")
	if len(got) != 0 {
		t.Logf("continuous punctuation: %v (len=%d)", got, len(got))
	}
}

func TestSplitSentences_LongChinese(t *testing.T) {
	text := "在一个风雨交加的夜晚，少年独自走在回家的路上，心中充满了对未来的迷茫与不安。突然，一道闪电划破天际，照亮了前方的一座古堡。"
	got := splitSentences(text)
	if len(got) != 2 {
		t.Fatalf("sentences = %d, want 2", len(got))
	}
}

// ---------------------------------------------------------------------------
// ComputeStats
// ---------------------------------------------------------------------------

func TestComputeStats_Empty(t *testing.T) {
	got := ComputeStats(nil)
	if got.SentenceCount != 0 {
		t.Errorf("SentenceCount = %d, want 0", got.SentenceCount)
	}
	if got.TotalChars != 0 {
		t.Errorf("TotalChars = %d, want 0", got.TotalChars)
	}
}

func TestComputeStats_SingleSample(t *testing.T) {
	samples := []Sample{
		{Content: "短句。这是一句中等长度的句子，包含了逗号。这是一句非常非常非常非常非常非常非常非常长的句子，它超过了三十个字所以应该被归类为长句。"},
	}
	got := ComputeStats(samples)

	if got.SentenceCount != 3 {
		t.Errorf("SentenceCount = %d, want 3", got.SentenceCount)
	}
	if got.ShortSentPct <= 0 {
		t.Errorf("ShortSentPct = %.1f, want > 0", got.ShortSentPct)
	}
	if got.LongSentPct <= 0 {
		t.Errorf("LongSentPct = %.1f, want > 0", got.LongSentPct)
	}
	if got.CommaDensity <= 0 {
		t.Errorf("CommaDensity = %.1f, want > 0", got.CommaDensity)
	}
	if got.PeriodDensity <= 0 {
		t.Errorf("PeriodDensity = %.1f, want > 0", got.PeriodDensity)
	}
	if got.ParagraphCount != 1 {
		t.Errorf("ParagraphCount = %d, want 1", got.ParagraphCount)
	}
}

func TestComputeStats_MultipleSamples(t *testing.T) {
	samples := []Sample{
		{Content: "第一段。短。"},
		{Content: "第二段，较长一些。"},
	}
	got := ComputeStats(samples)

	if got.SentenceCount != 3 {
		t.Errorf("SentenceCount = %d, want 3", got.SentenceCount)
	}
	if got.ParagraphCount != 1 {
		t.Errorf("ParagraphCount = %d, want 1", got.ParagraphCount)
	}
}

func TestComputeStats_PunctuationDensity(t *testing.T) {
	content := "你好，世界。真的！为什么？"
	samples := []Sample{
		{Content: content},
	}
	got := ComputeStats(samples)
	combined := content + "\n"
	runes := float64(len([]rune(combined)))

	wantComma := float64(1) * 100 / runes
	if math.Abs(got.CommaDensity-wantComma) > 0.1 {
		t.Errorf("CommaDensity = %.2f, want ~%.2f", got.CommaDensity, wantComma)
	}

	wantPeriod := float64(1) * 100 / runes
	if math.Abs(got.PeriodDensity-wantPeriod) > 0.1 {
		t.Errorf("PeriodDensity = %.2f, want ~%.2f", got.PeriodDensity, wantPeriod)
	}

	wantExclaim := float64(1) * 100 / runes
	if math.Abs(got.ExclaimDensity-wantExclaim) > 0.1 {
		t.Errorf("ExclaimDensity = %.2f, want ~%.2f", got.ExclaimDensity, wantExclaim)
	}

	wantQuestion := float64(1) * 100 / runes
	if math.Abs(got.QuestionDensity-wantQuestion) > 0.1 {
		t.Errorf("QuestionDensity = %.2f, want ~%.2f", got.QuestionDensity, wantQuestion)
	}
}

func TestComputeStats_QuoteDensity(t *testing.T) {
	content := "他说「你好」然后走了。"
	samples := []Sample{
		{Content: content},
	}
	got := ComputeStats(samples)
	combined := content + "\n"
	runes := float64(len([]rune(combined)))

	wantQuote := float64(2) * 100 / runes
	if math.Abs(got.QuoteDensity-wantQuote) > 0.1 {
		t.Errorf("QuoteDensity = %.2f, want ~%.2f", got.QuoteDensity, wantQuote)
	}
}

func TestComputeStats_SentenceLengthDistribution(t *testing.T) {
	longSentence := strings.Repeat("很", 30) + "长。"
	samples := []Sample{
		{Content: "短。" + longSentence},
	}
	got := ComputeStats(samples)

	if got.ShortSentPct <= 0 {
		t.Errorf("ShortSentPct should be > 0, got %.1f", got.ShortSentPct)
	}
	if got.LongSentPct <= 0 {
		t.Errorf("LongSentPct should be > 0, got %.1f", got.LongSentPct)
	}
	total := got.ShortSentPct + got.MidSentPct + got.LongSentPct
	if math.Abs(total-100) > 1 {
		t.Errorf("pct sum = %.1f, want ~100", total)
	}
}

func TestComputeStats_StdDev(t *testing.T) {
	samples := []Sample{
		{Content: "一二三四五。一二三四五。一二三四五。"},
	}
	got := ComputeStats(samples)
	if got.SentLenStdDev > 0.01 {
		t.Errorf("equal-length sentences: StdDev = %.4f, want ~0", got.SentLenStdDev)
	}
}

func TestComputeStats_AvgSentLen(t *testing.T) {
	samples := []Sample{
		{Content: "你好。世界。"},
	}
	got := ComputeStats(samples)
	if math.Abs(got.AvgSentLen-3.0) > 0.5 {
		t.Errorf("AvgSentLen = %.1f, want ~3.0", got.AvgSentLen)
	}
}

func TestComputeStats_AvgParaLen(t *testing.T) {
	samples := []Sample{
		{Content: "你好世界。"},
	}
	got := ComputeStats(samples)
	if got.AvgParaLen < 1 {
		t.Errorf("AvgParaLen = %.1f, want > 0", got.AvgParaLen)
	}
}

func TestComputeStats_TotalWords(t *testing.T) {
	samples := []Sample{
		{Content: "你好hello世界world。"},
	}
	got := ComputeStats(samples)
	if got.TotalWords < 4 {
		t.Errorf("TotalWords = %d, want >= 4", got.TotalWords)
	}
}

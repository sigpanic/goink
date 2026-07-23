//go:build cgo && e2e

package e2e

import (
	"context"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/sigpanic/goink/internal/rag"
)

// TestOnnxConcurrent_Embed launches 10 goroutines each calling Embed()
// with different Chinese text, verifying all return valid 512-dim L2-normalized vectors.
func TestOnnxConcurrent_Embed(t *testing.T) {
	embedder, err := rag.GetEmbedder()
	if err != nil {
		t.Fatalf("GetEmbedder() failed: %v", err)
	}

	texts := []string{
		"春风拂过江南水乡的小桥流水",
		"远处的山峦在晨雾中若隐若现",
		"他在深夜的图书馆里翻阅古籍",
		"一道闪电划破了漆黑的夜空",
		"少女在樱花树下轻轻吟唱",
		"古老的咒语在空气中回荡",
		"战鼓擂动，千军万马奔腾而来",
		"月光洒落在寂静的湖面上",
		"密林深处传来阵阵鸟鸣声",
		"命运的齿轮开始缓缓转动",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	var mu sync.Mutex
	results := make([][]float32, len(texts))
	var firstErr error

	for i, text := range texts {
		i, text := i, text
		wg.Add(1)
		go func() {
			defer wg.Done()
			vec, err := embedder.Embed(ctx, text)
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("goroutine %d: %w", i, err)
				}
				mu.Unlock()
				return
			}

			if len(vec) != 512 {
				t.Errorf("goroutine %d: expected 512 dims, got %d", i, len(vec))
			}

			var norm float64
			for _, v := range vec {
				norm += float64(v) * float64(v)
			}
			norm = math.Sqrt(norm)
			if math.Abs(norm-1.0) > 0.01 {
				t.Errorf("goroutine %d: L2 norm = %f, expected ≈1.0", i, norm)
			}

			mu.Lock()
			results[i] = vec
			mu.Unlock()
		}()
	}

	wg.Wait()

	if firstErr != nil {
		t.Fatalf("concurrent Embed() failed: %v", firstErr)
	}

	// Verify all goroutines produced results
	for i, vec := range results {
		if vec == nil {
			t.Errorf("goroutine %d: result is nil", i)
		}
	}

	t.Logf("Concurrent Embed OK: %d goroutines completed", len(texts))
}

// TestOnnxConcurrent_EmbedBatch launches 5 goroutines each calling EmbedBatch()
// with a batch of 3 texts, verifying all return correct results.
func TestOnnxConcurrent_EmbedBatch(t *testing.T) {
	embedder, err := rag.GetEmbedder()
	if err != nil {
		t.Fatalf("GetEmbedder() failed: %v", err)
	}

	batches := [][]string{
		{"主角踏入了未知的领域", "暗影中传来低沉的笑声", "命运的丝线交织在一起"},
		{"大雪纷飞的夜晚格外安静", "远方的灯塔闪烁着微光", "脚印延伸到森林深处"},
		{"古老的卷轴上记载着秘密", "星辰排列成神秘的图案", "钥匙藏在最不可能的地方"},
		{"海浪拍打着礁石发出轰鸣", "船只在风暴中摇摆不定", "水手紧握着舵轮不放"},
		{"花园里盛开着各色鲜花", "蝴蝶在花丛间翩翩起舞", "微风送来阵阵花香"},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	var mu sync.Mutex
	allResults := make([][][]float32, len(batches))
	var firstErr error

	for i, batch := range batches {
		i, batch := i, batch
		wg.Add(1)
		go func() {
			defer wg.Done()
			vecs, err := embedder.EmbedBatch(ctx, batch)
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("batch %d: %w", i, err)
				}
				mu.Unlock()
				return
			}

			if len(vecs) != len(batch) {
				t.Errorf("batch %d: expected %d vectors, got %d", i, len(batch), len(vecs))
			}

			for j, vec := range vecs {
				if len(vec) != 512 {
					t.Errorf("batch %d vec[%d]: expected 512 dims, got %d", i, j, len(vec))
					continue
				}

				var norm float64
				for _, v := range vec {
					norm += float64(v) * float64(v)
				}
				norm = math.Sqrt(norm)
				if math.Abs(norm-1.0) > 0.01 {
					t.Errorf("batch %d vec[%d]: L2 norm = %f, expected ≈1.0", i, j, norm)
				}
			}

			mu.Lock()
			allResults[i] = vecs
			mu.Unlock()
		}()
	}

	wg.Wait()

	if firstErr != nil {
		t.Fatalf("concurrent EmbedBatch() failed: %v", firstErr)
	}

	for i, vecs := range allResults {
		if vecs == nil {
			t.Errorf("batch %d: result is nil", i)
		}
	}

	t.Logf("Concurrent EmbedBatch OK: %d batches completed", len(batches))
}

// TestOnnxConcurrent_MixedEmbedAndBatch launches goroutines that mix
// single Embed and EmbedBatch calls concurrently, verifying no panics.
func TestOnnxConcurrent_MixedEmbedAndBatch(t *testing.T) {
	embedder, err := rag.GetEmbedder()
	if err != nil {
		t.Fatalf("GetEmbedder() failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error

	// 6 single Embed calls
	for i := 0; i < 6; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			vec, err := embedder.Embed(ctx, fmt.Sprintf("并发混合测试的单条嵌入调用编号%d", i))
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("single embed %d: %w", i, err)
				}
				mu.Unlock()
				return
			}
			if len(vec) != 512 {
				t.Errorf("single embed %d: expected 512 dims, got %d", i, len(vec))
			}
		}()
	}

	// 4 EmbedBatch calls
	for i := 0; i < 4; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			vecs, err := embedder.EmbedBatch(ctx, []string{
				"混合并发批次测试文本一",
				"混合并发批次测试文本二",
			})
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("batch %d: %w", i, err)
				}
				mu.Unlock()
				return
			}
			if len(vecs) != 2 {
				t.Errorf("batch %d: expected 2 vectors, got %d", i, len(vecs))
			}
			for j, vec := range vecs {
				if len(vec) != 512 {
					t.Errorf("batch %d vec[%d]: expected 512 dims, got %d", i, j, len(vec))
				}
			}
		}()
	}

	wg.Wait()

	if firstErr != nil {
		t.Fatalf("concurrent mixed Embed/EmbedBatch failed: %v", firstErr)
	}

	t.Log("Concurrent mixed Embed + EmbedBatch OK: no panics or errors")
}

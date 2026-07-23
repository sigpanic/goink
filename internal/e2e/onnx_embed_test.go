//go:build cgo && e2e

package e2e

import (
	"context"
	"math"
	"os"
	"testing"

	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/rag"
)

func TestOnnxEmbedder_Init(t *testing.T) {
	embedder, err := rag.GetEmbedder()
	if err != nil {
		t.Fatalf("GetEmbedder() failed: %v", err)
	}
	if embedder == nil {
		t.Fatal("GetEmbedder() returned nil")
	}

	// Verify model files exist (sanity check for test environment)
	modelsDir := config.ModelsDir()
	if _, err := os.Stat(modelsDir); err != nil {
		t.Fatalf("models dir not accessible: %v", err)
	}

	t.Log("ONNX embedder initialized successfully")
}

func TestOnnxEmbedder_SingleEmbed(t *testing.T) {
	embedder, err := rag.GetEmbedder()
	if err != nil {
		t.Fatalf("GetEmbedder() failed: %v", err)
	}

	ctx := context.Background()
	vec, err := embedder.Embed(ctx, "这是一段测试文本，用于验证ONNX嵌入器是否正常工作。")
	if err != nil {
		t.Fatalf("Embed() failed: %v", err)
	}

	// Verify dimension
	if len(vec) != 512 {
		t.Fatalf("expected 512 dimensions, got %d", len(vec))
	}

	// Verify L2 norm ≈ 1.0 (BGE model uses L2 normalization)
	var norm float64
	for _, v := range vec {
		norm += float64(v) * float64(v)
	}
	norm = math.Sqrt(norm)
	if math.Abs(norm-1.0) > 0.01 {
		t.Errorf("expected L2 norm ≈ 1.0, got %f", norm)
	}

	// Verify vector is not all zeros
	allZero := true
	for _, v := range vec {
		if v != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Error("embedding vector is all zeros")
	}

	t.Logf("Embed OK: dim=%d, L2 norm=%.4f", len(vec), norm)
}

func TestOnnxEmbedder_BatchEmbed(t *testing.T) {
	embedder, err := rag.GetEmbedder()
	if err != nil {
		t.Fatalf("GetEmbedder() failed: %v", err)
	}

	ctx := context.Background()
	texts := []string{
		"主角走进了古老的城堡",
		"敌人埋伏在远处的山坡上",
		"她在月光下弹奏了一首曲子",
	}

	vecs, err := embedder.EmbedBatch(ctx, texts)
	if err != nil {
		t.Fatalf("EmbedBatch() failed: %v", err)
	}

	if len(vecs) != len(texts) {
		t.Fatalf("expected %d vectors, got %d", len(texts), len(vecs))
	}

	for i, vec := range vecs {
		if len(vec) != 512 {
			t.Errorf("vec[%d]: expected 512 dims, got %d", i, len(vec))
		}

		var norm float64
		for _, v := range vec {
			norm += float64(v) * float64(v)
		}
		norm = math.Sqrt(norm)
		if math.Abs(norm-1.0) > 0.01 {
			t.Errorf("vec[%d]: L2 norm = %f, expected ≈1.0", i, norm)
		}
	}

	t.Logf("EmbedBatch OK: %d vectors, all 512-dim, L2 normalized", len(vecs))
}

func TestOnnxEmbedder_Consistency(t *testing.T) {
	embedder, err := rag.GetEmbedder()
	if err != nil {
		t.Fatalf("GetEmbedder() failed: %v", err)
	}

	ctx := context.Background()
	text := "一致性测试：相同的文本应该产生相同的嵌入向量"

	vec1, err := embedder.Embed(ctx, text)
	if err != nil {
		t.Fatalf("first Embed() failed: %v", err)
	}

	vec2, err := embedder.Embed(ctx, text)
	if err != nil {
		t.Fatalf("second Embed() failed: %v", err)
	}

	// Verify identical vectors
	var maxDiff float32
	for i := range vec1 {
		diff := vec1[i] - vec2[i]
		if diff < 0 {
			diff = -diff
		}
		if diff > maxDiff {
			maxDiff = diff
		}
	}
	if maxDiff > 1e-5 {
		t.Errorf("same text produced different embeddings, max diff = %e", maxDiff)
	}

	t.Logf("Consistency OK: max diff = %e", maxDiff)
}

func TestOnnxEmbedder_DifferentTexts(t *testing.T) {
	embedder, err := rag.GetEmbedder()
	if err != nil {
		t.Fatalf("GetEmbedder() failed: %v", err)
	}

	ctx := context.Background()
	vec1, _ := embedder.Embed(ctx, "一只小猫在草地上玩耍")
	vec2, _ := embedder.Embed(ctx, "量子计算是未来科技的重要方向")

	if vec1 == nil || vec2 == nil {
		t.Fatal("Embed() returned nil")
	}

	// Compute cosine similarity
	var dot, norm1, norm2 float64
	for i := range vec1 {
		dot += float64(vec1[i]) * float64(vec2[i])
		norm1 += float64(vec1[i]) * float64(vec1[i])
		norm2 += float64(vec2[i]) * float64(vec2[i])
	}
	cosSim := dot / (math.Sqrt(norm1) * math.Sqrt(norm2))

	// Very different texts should have low similarity (but > 0 for BGE)
	t.Logf("Cosine similarity between unrelated texts: %.4f", cosSim)
	if cosSim > 0.99 {
		t.Errorf("unrelated texts have very high similarity: %.4f", cosSim)
	}
}

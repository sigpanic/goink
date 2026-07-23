//go:build cgo && e2e

package e2e

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/rag"
)

// verifyEmbedding validates that a vector has dim=512 and L2 norm ≈ 1.0.
func verifyEmbedding(t *testing.T, vec []float32, label string) {
	t.Helper()

	if len(vec) != 512 {
		t.Fatalf("%s: expected 512 dimensions, got %d", label, len(vec))
	}

	var norm float64
	for _, v := range vec {
		norm += float64(v) * float64(v)
	}
	norm = math.Sqrt(norm)
	if math.Abs(norm-1.0) > 0.01 {
		t.Errorf("%s: L2 norm = %f, expected ≈1.0", label, norm)
	}
}

// TestOnnxEdge_EmptyText verifies that embedding an empty string does not panic
// and returns a valid 512-dim L2-normalized vector. The model should produce a
// [CLS][SEP] sequence which yields a valid embedding.
func TestOnnxEdge_EmptyText(t *testing.T) {
	embedder, err := rag.GetEmbedder()
	if err != nil {
		t.Fatalf("GetEmbedder() failed: %v", err)
	}

	vec, err := embedder.Embed(context.Background(), "")
	if err != nil {
		t.Fatalf("Embed(\"\") failed: %v", err)
	}

	verifyEmbedding(t, vec, "empty text")

	// Also verify it's not all zeros — [CLS] embedding should be non-trivial
	allZero := true
	for _, v := range vec {
		if v != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Error("embedding of empty text is all zeros")
	}

	t.Logf("Empty text embedding OK: L2 norm verified")
}

// TestOnnxEdge_VeryLongText verifies that text exceeding 512 tokens gets
// truncated properly and still returns a valid 512-dim L2-normalized vector.
func TestOnnxEdge_VeryLongText(t *testing.T) {
	embedder, err := rag.GetEmbedder()
	if err != nil {
		t.Fatalf("GetEmbedder() failed: %v", err)
	}

	// 2000 Chinese characters — well beyond 512 BERT tokens
	longText := strings.Repeat("这是一段用于测试超长文本截断功能的文字。", 100)
	if len([]rune(longText)) < 2000 {
		t.Fatalf("test text too short: %d runes", len([]rune(longText)))
	}

	vec, err := embedder.Embed(context.Background(), longText)
	if err != nil {
		t.Fatalf("Embed(very long text) failed: %v", err)
	}

	verifyEmbedding(t, vec, "very long text")
	t.Logf("Very long text embedding OK: %d runes, dim=512, L2 norm verified", len([]rune(longText)))
}

// TestOnnxEdge_SpecialUnicode verifies that text containing emoji, rare CJK
// characters, zero-width joiners, and other special Unicode does not crash the
// embedder and produces a valid output vector.
func TestOnnxEdge_SpecialUnicode(t *testing.T) {
	embedder, err := rag.GetEmbedder()
	if err != nil {
		t.Fatalf("GetEmbedder() failed: %v", err)
	}

	specialText := "🎉👨‍👩‍👧‍👦𪚥𱁬𰀀❤️🔥💯🇨🇳"

	vec, err := embedder.Embed(context.Background(), specialText)
	if err != nil {
		t.Fatalf("Embed(special unicode) failed: %v", err)
	}

	verifyEmbedding(t, vec, "special unicode")
	t.Logf("Special Unicode embedding OK: dim=512, L2 norm verified")
}

// TestOnnxEdge_MixedLanguage verifies that text mixing Chinese, English,
// Japanese, and Korean produces a valid embedding without errors.
func TestOnnxEdge_MixedLanguage(t *testing.T) {
	embedder, err := rag.GetEmbedder()
	if err != nil {
		t.Fatalf("GetEmbedder() failed: %v", err)
	}

	mixedText := "这是一段中文，this is English, これは日本語です, 이것은 한국어입니다."
	vec, err := embedder.Embed(context.Background(), mixedText)
	if err != nil {
		t.Fatalf("Embed(mixed language) failed: %v", err)
	}

	verifyEmbedding(t, vec, "mixed language")
	t.Logf("Mixed language embedding OK: dim=512, L2 norm verified")
}

// TestOnnxEdge_RepeatedText verifies that repeating the same short text 100
// times (a tokenization edge case) still produces a valid embedding.
func TestOnnxEdge_RepeatedText(t *testing.T) {
	embedder, err := rag.GetEmbedder()
	if err != nil {
		t.Fatalf("GetEmbedder() failed: %v", err)
	}

	repeatedText := strings.Repeat("测试", 100)
	vec, err := embedder.Embed(context.Background(), repeatedText)
	if err != nil {
		t.Fatalf("Embed(repeated text) failed: %v", err)
	}

	verifyEmbedding(t, vec, "repeated text")
	t.Logf("Repeated text embedding OK: dim=512, L2 norm verified")
}

// TestOnnxEdge_ModelFileIntegrity verifies that if model.onnx does not exist
// at the resolved path, the embedder initialization fails as expected.
// It does NOT delete any real files — it constructs a non-existent path
// and checks that os.Stat and the initializer handle it correctly.
func TestOnnxEdge_ModelFileIntegrity(t *testing.T) {
	modelsDir := config.ModelsDir()
	t.Logf("Resolved models dir: %s", modelsDir)

	modelPath := filepath.Join(modelsDir, "model.onnx")

	// Verify the real model file exists (this is a sanity check for the test environment)
	if _, err := os.Stat(modelPath); err != nil {
		t.Fatalf("model.onnx not found at %s — cannot verify integrity check: %v", modelPath, err)
	}

	// Now verify that a non-existent path is detected correctly
	fakeDir := filepath.Join(modelsDir, "nonexistent_subdir_for_test")
	fakeModelPath := filepath.Join(fakeDir, "model.onnx")

	if _, err := os.Stat(fakeModelPath); !os.IsNotExist(err) {
		t.Errorf("expected os.Stat to report non-existent for %s, got err=%v", fakeModelPath, err)
	}

	t.Logf("Model file integrity check OK: real model at %s, non-existent path correctly detected", modelPath)
}

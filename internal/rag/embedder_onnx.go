//go:build cgo

package rag

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"path/filepath"
	"runtime"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
)

const (
	bertMaxLen   = 512 // BERT 模型最大输入 token 数（含特殊 token）
	maxBatchSize = 8   // 单次 ONNX Run 最大样本数：批越大，注意力张量的峰值内存越高

	minEmbedThreads = 2 // 推理线程数下限：核少的机器也要能用
	maxEmbedThreads = 4 // 推理线程数上限：核多的机器不占满 CPU
)

// BGE 模型查询时需要加指令前缀，文档不加。
const queryInstruction = "为这个句子生成表示以用于检索相关文章："

const (
	memoryArenaShrinkKey   = "memory.enable_memory_arena_shrinkage"
	memoryArenaShrinkValue = "cpu:0"
	compactProbeText       = "内存整理"
)

var (
	instance  *OnnxEmbedder
	tokenizer *Tokenizer
	initOnce  sync.Once
	ready     = make(chan struct{})
	initErr   error
)

// OnnxEmbedder 使用 ONNX Runtime 加载 bge-small-zh-v1.5 (int8) 模型生成 embedding。
// 全局仅一个实例，通过 InitEmbedder 异步初始化。
type OnnxEmbedder struct {
	session   *ort.DynamicAdvancedSession
	tokenizer *Tokenizer
	clsID     int
	sepID     int
	mu        sync.Mutex
	log       *slog.Logger
}

// InitEmbedder 加载 tokenizer（同步）后异步加载 ONNX 模型，不阻塞调用方。
// main 启动时尽早调用，模型在后台加载，GUI 可先行渲染。
// 调用后 GetTokenizer() 即可使用，无需等待模型就绪。
func InitEmbedder(modelsDir string, log *slog.Logger) {
	initOnce.Do(func() {
		t, err := NewTokenizer(filepath.Join(modelsDir, "vocab.txt"))
		if err != nil {
			initErr = err
			close(ready)
			return
		}
		tokenizer = t

		go func() {
			defer close(ready)
			e, err := newOnnxEmbedder(modelsDir, t, log)
			if err != nil {
				initErr = err
				return
			}
			instance = e
		}()
	})
}

// GetEmbedder 返回全局 embedder 实例。若尚未加载完成则阻塞等待。
func GetEmbedder() (*OnnxEmbedder, error) {
	<-ready
	if initErr != nil {
		return nil, initErr
	}
	return instance, nil
}

// TryGetEmbedder 非阻塞获取全局 embedder 实例。
// 若尚未初始化完成则返回 nil，不会阻塞。适用于 shutdown 等不能阻塞的场景。
func TryGetEmbedder() *OnnxEmbedder {
	select {
	case <-ready:
		if initErr != nil {
			return nil
		}
		return instance
	default:
		return nil
	}
}

// GetTokenizer 返回全局 tokenizer 实例。InitEmbedder 调用后即可使用，
// 未调用 InitEmbedder 时返回 nil。
func GetTokenizer() *Tokenizer {
	return tokenizer
}

func (e *OnnxEmbedder) Dim() int { return 512 }

// embedThreads 返回批量推理使用的 ONNX 线程数：按 CPU 核数动态收缩，
// 避免核多的机器被推理占满，也避免核少的机器线程数过低而不可用。
func embedThreads() int {
	threads := runtime.NumCPU() / 4
	if threads < minEmbedThreads {
		threads = minEmbedThreads
	}
	if threads > maxEmbedThreads {
		threads = maxEmbedThreads
	}
	return threads
}

// newOnnxEmbedder 同步初始化 ONNX Runtime 并加载模型。仅由 InitEmbedder 调用。
func newOnnxEmbedder(modelsDir string, t *Tokenizer, log *slog.Logger) (*OnnxEmbedder, error) {
	modelPath := filepath.Join(modelsDir, "model.onnx")

	clsID, ok1 := t.vocab["[CLS]"]
	sepID, ok2 := t.vocab["[SEP]"]
	if !ok1 || !ok2 {
		return nil, fmt.Errorf("rag: vocab missing [CLS] or [SEP]")
	}

	if err := ort.InitializeEnvironment(); err != nil {
		return nil, fmt.Errorf("rag: init onnx environment: %w", err)
	}

	options, err := ort.NewSessionOptions()
	if err != nil {
		return nil, fmt.Errorf("rag: create session options: %w", err)
	}
	defer options.Destroy()

	// 保持 CPU arena 开启，使连续推理复用稳定的高水位内存；全量重建结束后由
	// Compact 显式 shrink。关闭 arena 会把释放行为交给 native allocator，Linux
	// 多线程推理下反而可能持续抬高 RSS。
	if err := options.SetCpuMemArena(true); err != nil {
		return nil, fmt.Errorf("rag: enable cpu mem arena: %w", err)
	}
	if err := options.SetMemPattern(false); err != nil {
		return nil, fmt.Errorf("rag: disable mem pattern: %w", err)
	}
	// 线程数按机器规模动态收缩；Run 已由 e.mu 串行化，inter-op 并行无意义。
	if err := options.SetIntraOpNumThreads(embedThreads()); err != nil {
		return nil, fmt.Errorf("rag: set intra op threads: %w", err)
	}
	if err := options.SetInterOpNumThreads(1); err != nil {
		return nil, fmt.Errorf("rag: set inter op threads: %w", err)
	}

	session, err := ort.NewDynamicAdvancedSession(modelPath,
		[]string{"input_ids", "attention_mask", "token_type_ids"},
		[]string{"last_hidden_state"}, options)
	if err != nil {
		return nil, fmt.Errorf("rag: load model: %w", err)
	}

	log.Info("ONNX embedder 已初始化", "model", modelPath)
	return &OnnxEmbedder{
		session:   session,
		tokenizer: t,
		clsID:     clsID,
		sepID:     sepID,
		log:       log,
	}, nil
}

// ── 模型适配层 ────────────────────────────────────────────

// prepare 对原始 token 先截断再加 [CLS]/[SEP]，保证总长度 ≤ bertMaxLen。
func (e *OnnxEmbedder) prepare(text string) []int {
	raw := e.tokenizer.Tokenize(text)
	limit := bertMaxLen - 2
	if len(raw) > limit {
		e.log.Warn("token 序列超过模型上限，已截断", "len", len(raw), "max", limit)
		raw = raw[:limit]
	}
	ids := make([]int, len(raw)+2)
	ids[0] = e.clsID
	copy(ids[1:], raw)
	ids[len(ids)-1] = e.sepID
	return ids
}

// ── Embedding ────────────────────────────────────────────

func (e *OnnxEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	// 查询加 BGE 指令前缀
	ids := e.prepare(queryInstruction + text)

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	seqLen := int64(len(ids))
	inputIDs := make([]int64, seqLen)
	attentionMask := make([]int64, seqLen)
	tokenTypeIDs := make([]int64, seqLen)
	for i, id := range ids {
		inputIDs[i] = int64(id)
		attentionMask[i] = 1
	}

	inputTensor, err := ort.NewTensor(ort.NewShape(1, seqLen), inputIDs)
	if err != nil {
		return nil, fmt.Errorf("rag: create input tensor: %w", err)
	}
	defer inputTensor.Destroy()

	maskTensor, err := ort.NewTensor(ort.NewShape(1, seqLen), attentionMask)
	if err != nil {
		return nil, fmt.Errorf("rag: create mask tensor: %w", err)
	}
	defer maskTensor.Destroy()

	typeIDsTensor, err := ort.NewTensor(ort.NewShape(1, seqLen), tokenTypeIDs)
	if err != nil {
		return nil, fmt.Errorf("rag: create type_ids tensor: %w", err)
	}
	defer typeIDsTensor.Destroy()

	outputTensor, err := ort.NewEmptyTensor[float32](ort.NewShape(1, seqLen, 512))
	if err != nil {
		return nil, fmt.Errorf("rag: create output tensor: %w", err)
	}
	defer outputTensor.Destroy()

	e.mu.Lock()
	defer e.mu.Unlock()
	err = e.session.Run(
		[]ort.Value{inputTensor, maskTensor, typeIDsTensor},
		[]ort.Value{outputTensor},
	)

	if err != nil {
		return nil, fmt.Errorf("rag: onnx run: %w", err)
	}

	hidden := outputTensor.GetData()
	return clsPool(hidden, 512), nil
}

func (e *OnnxEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	if len(texts) <= maxBatchSize {
		return e.embedBatch(ctx, texts, nil)
	}

	results := make([][]float32, 0, len(texts))
	for i := 0; i < len(texts); i += maxBatchSize {
		end := i + maxBatchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch, err := e.embedBatch(ctx, texts[i:end], nil)
		if err != nil {
			return nil, fmt.Errorf("rag: batch [%d:%d]: %w", i, end, err)
		}
		results = append(results, batch...)
	}
	return results, nil
}

// embedBatch 执行单次 batch ONNX 推理，调用方保证 len(texts) ≤ maxBatchSize。
func (e *OnnxEmbedder) embedBatch(ctx context.Context, texts []string, runOptions *ort.RunOptions) ([][]float32, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// 1. Tokenize all texts, find max length.
	tokenized := make([][]int, len(texts))
	maxLen := 0
	for i, text := range texts {
		tokenized[i] = e.prepare(text)
		if len(tokenized[i]) > maxLen {
			maxLen = len(tokenized[i])
		}
	}
	if maxLen == 0 {
		results := make([][]float32, len(texts))
		for i := range results {
			results[i] = make([]float32, 512)
		}
		return results, nil
	}

	batchSize := int64(len(texts))
	seqLen := int64(maxLen)

	// 2. Build padded tensors [N, maxLen].
	inputIDs := make([]int64, batchSize*seqLen)
	attentionMask := make([]int64, batchSize*seqLen)
	tokenTypeIDs := make([]int64, batchSize*seqLen)

	for i := int64(0); i < batchSize; i++ {
		ids := tokenized[i]
		base := i * seqLen
		for j, id := range ids {
			inputIDs[base+int64(j)] = int64(id)
			attentionMask[base+int64(j)] = 1
		}
	}

	inputTensor, err := ort.NewTensor(ort.NewShape(batchSize, seqLen), inputIDs)
	if err != nil {
		return nil, fmt.Errorf("rag: create batch input tensor: %w", err)
	}
	defer inputTensor.Destroy()

	maskTensor, err := ort.NewTensor(ort.NewShape(batchSize, seqLen), attentionMask)
	if err != nil {
		return nil, fmt.Errorf("rag: create batch mask tensor: %w", err)
	}
	defer maskTensor.Destroy()

	typeIDsTensor, err := ort.NewTensor(ort.NewShape(batchSize, seqLen), tokenTypeIDs)
	if err != nil {
		return nil, fmt.Errorf("rag: create batch type_ids tensor: %w", err)
	}
	defer typeIDsTensor.Destroy()

	outputTensor, err := ort.NewEmptyTensor[float32](ort.NewShape(batchSize, seqLen, 512))
	if err != nil {
		return nil, fmt.Errorf("rag: create batch output tensor: %w", err)
	}
	defer outputTensor.Destroy()

	// 3. Single ONNX Run.
	e.mu.Lock()
	defer e.mu.Unlock()
	err = e.session.RunWithOptions(
		[]ort.Value{inputTensor, maskTensor, typeIDsTensor},
		[]ort.Value{outputTensor},
		runOptions,
	)
	if err != nil {
		return nil, fmt.Errorf("rag: batch onnx run: %w", err)
	}

	// 4. CLS pool each sample.
	hidden := outputTensor.GetData()
	results := make([][]float32, batchSize)
	sampleSize := int(seqLen) * 512
	for i := int64(0); i < batchSize; i++ {
		start := int(i) * sampleSize
		results[i] = clsPool(hidden[start:start+sampleSize], 512)
	}

	return results, nil
}

// Compact 在一次极小推理结束时触发 ONNX Runtime CPU arena shrink。
// 用于完整索引重建后的任务级收尾；普通查询和增量刷新不应逐次调用。
func (e *OnnxEmbedder) Compact(ctx context.Context) error {
	options, err := ort.NewRunOptions()
	if err != nil {
		return fmt.Errorf("rag: create compact run options: %w", err)
	}
	defer options.Destroy()
	if err := options.AddRunConfigEntry(memoryArenaShrinkKey, memoryArenaShrinkValue); err != nil {
		return fmt.Errorf("rag: configure memory arena shrink: %w", err)
	}
	if _, err := e.embedBatch(ctx, []string{compactProbeText}, options); err != nil {
		return fmt.Errorf("rag: compact memory arena: %w", err)
	}
	e.log.Info("ONNX CPU 内存池已整理")
	return nil
}

func (e *OnnxEmbedder) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.session != nil {
		e.session.Destroy()
		e.session = nil
	}
	return nil
}

// ── Pooling ──────────────────────────────────────────────

// clsPool 取 [CLS] token 的 embedding 并做 L2 归一化。
// BGE 模型使用 CLS 池化 + L2 归一化，优于均值池化。
func clsPool(hidden []float32, dim int) []float32 {
	vec := make([]float32, dim)
	copy(vec, hidden[:dim])
	var norm float64
	for _, v := range vec {
		norm += float64(v) * float64(v)
	}
	norm = math.Sqrt(norm)
	if norm > 0 {
		for i := range vec {
			vec[i] /= float32(norm)
		}
	}
	return vec
}

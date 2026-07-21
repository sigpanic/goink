//go:build cgo

package rag

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"novel/internal/chapter"
	"novel/internal/git"
	"novel/internal/novel"
)

// RefreshTask 是一次向量刷新任务。
type RefreshTask struct {
	NovelID       int64
	ChapterNumber int
	Content       string
}

// RefreshQueue 异步管理向量刷新，支持去重和限速。
type RefreshQueue struct {
	vs         *VectorStore
	chStore    *chapter.Store
	novelStore *novel.Store
	logger     *slog.Logger

	ch      chan RefreshTask
	pending map[string]RefreshTask
	mu      sync.Mutex

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// ── 全局单例 ──────────────────────────────────────────────

var (
	rqOnce sync.Once
	rq     *RefreshQueue
)

// InitRefreshQueue 初始化全局 RefreshQueue。多次调用只生效一次。
func InitRefreshQueue(vs *VectorStore, chStore *chapter.Store, novelStore *novel.Store, logger *slog.Logger) {
	rqOnce.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		rq = &RefreshQueue{
			vs:         vs,
			chStore:    chStore,
			novelStore: novelStore,
			logger:     logger,
			ch:         make(chan RefreshTask, 256),
			pending:    make(map[string]RefreshTask),
			ctx:        ctx,
			cancel:     cancel,
		}
	})
}

// GetRefreshQueue 返回全局 RefreshQueue，未初始化时返回 nil。
func GetRefreshQueue() *RefreshQueue {
	return rq
}

// SubmitRefresh 提交异步向量刷新任务。若 RefreshQueue 未初始化则静默跳过。
func SubmitRefresh(novelID int64, chapterNumber int, content string) {
	if rq == nil {
		return
	}
	rq.Submit(RefreshTask{NovelID: novelID, ChapterNumber: chapterNumber, Content: content})
}

// ── 实例方法 ──────────────────────────────────────────────

// Start 启动后台消费者 goroutine。
func (q *RefreshQueue) Start() {
	q.wg.Add(1)
	go q.consumer()
}

// Stop 取消后台消费者并等待完成。
func (q *RefreshQueue) Stop() {
	q.cancel()
	q.wg.Wait()
}

// Submit 非阻塞提交刷新任务。队列满时丢弃并记警告。
func (q *RefreshQueue) Submit(task RefreshTask) {
	select {
	case q.ch <- task:
	default:
		q.logger.Warn("向量刷新队列已满，丢弃任务", "chapter_number", task.ChapterNumber)
	}
}

func pendingKey(task RefreshTask) string {
	return fmt.Sprintf("%d:%d", task.NovelID, task.ChapterNumber)
}

// consumer 是后台消费者，500ms 内同一章节的重复提交合并为一次。
func (q *RefreshQueue) consumer() {
	defer q.wg.Done()

	timer := time.NewTimer(0)
	if !timer.Stop() {
		<-timer.C
	}
	timerActive := false

	for {
		select {
		case <-q.ctx.Done():
			// 退出前清空 pending（使用独立 context，不受 cancel 影响）
			q.mu.Lock()
			pending := q.pending
			q.pending = make(map[string]RefreshTask)
			q.mu.Unlock()
			drainCtx, drainCancel := context.WithTimeout(context.Background(), 30*time.Second)
			for _, task := range pending {
				q.doRefreshWithCtx(drainCtx, task)
			}
			drainCancel()
			return

		case task := <-q.ch:
			q.mu.Lock()
			q.pending[pendingKey(task)] = task
			if !timerActive {
				timer.Reset(500 * time.Millisecond)
				timerActive = true
			}
			q.mu.Unlock()

		case <-timer.C:
			q.mu.Lock()
			timerActive = false
			tasks := q.pending
			q.pending = make(map[string]RefreshTask)
			q.mu.Unlock()

			for _, task := range tasks {
				q.doRefresh(task)
			}
		}
	}
}

func (q *RefreshQueue) doRefresh(task RefreshTask) {
	ctx, cancel := context.WithTimeout(q.ctx, 30*time.Second)
	defer cancel()
	q.doRefreshWithCtx(ctx, task)
}

func (q *RefreshQueue) doRefreshWithCtx(ctx context.Context, task RefreshTask) {

	ch, err := q.chStore.GetByNovelAndNumber(ctx, task.NovelID, task.ChapterNumber)
	if err != nil {
		q.logger.Warn("查章节失败，跳过向量刷新", "novel_id", task.NovelID, "chapter_number", task.ChapterNumber, "err", err)
		return
	}

	if err := q.vs.DeleteChapterChunks(ctx, task.NovelID, task.ChapterNumber); err != nil {
		q.logger.Warn("删除章节旧向量失败", "chapter_number", task.ChapterNumber, "err", err)
	}

	params := ChapterChunkParams{
		ChapterNumber: task.ChapterNumber,
		ChapterTitle:  ch.Title,
		Content:       task.Content,
		Summary:       ch.Summary,
	}
	chunks := BuildChapterChunks(params, GetTokenizer())
	if len(chunks) == 0 {
		return
	}

	if err := q.vs.IndexChunks(ctx, task.NovelID, chunks); err != nil {
		q.logger.Error("索引章节向量失败", "chapter_number", task.ChapterNumber, "err", err)
	}
}

// RebuildNovel 无条件全量重建一部小说的向量索引。
func (q *RefreshQueue) RebuildNovel(ctx context.Context, novelID int64) error {
	chapters, err := q.chStore.ListAllByNovel(ctx, novelID)
	if err != nil {
		return fmt.Errorf("rag: list chapters for rebuild: %w", err)
	}

	if len(chapters) == 0 {
		return nil
	}

	if err := q.vs.DeleteNovel(ctx, novelID); err != nil {
		return fmt.Errorf("rag: rebuild: delete old vectors: %w", err)
	}

	var batch []Chunk
	batchCount := 0
	totalChunks := 0

	for _, ch := range chapters {
		content, err := git.ReadFile(novelID, ch.FilePath)
		if err != nil {
			q.logger.Warn("读取章节文件失败，跳过", "chapter_id", ch.ID, "path", ch.FilePath, "err", err)
			continue
		}

		params := ChapterChunkParams{
			ChapterNumber: ch.ChapterNumber,
			ChapterTitle:  ch.Title,
			Content:       content,
			Summary:       ch.Summary,
		}
		chunks := BuildChapterChunks(params, GetTokenizer())
		batch = append(batch, chunks...)

		if len(batch) >= maxBatchSize {
			if err := q.vs.IndexChunks(ctx, novelID, batch); err != nil {
				return fmt.Errorf("rag: index batch: %w", err)
			}
			totalChunks += len(batch)
			batch = batch[:0]
			batchCount++
			if batchCount%4 == 0 {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(50 * time.Millisecond):
				}
			}
		}
	}

	if len(batch) > 0 {
		if err := q.vs.IndexChunks(ctx, novelID, batch); err != nil {
			return fmt.Errorf("rag: index final batch: %w", err)
		}
		totalChunks += len(batch)
	}

	q.logger.Info("全量向量重建完成", "novel_id", novelID, "chapters", len(chapters), "chunks", totalChunks)
	return nil
}

// RebuildAll 遍历全部小说，对尚无向量索引的小说执行首次全量重建。
func (q *RefreshQueue) RebuildAll(ctx context.Context) error {
	var novels []novel.Novel
	if err := q.novelStore.DB.WithContext(ctx).Find(&novels).Error; err != nil {
		return fmt.Errorf("rag: list novels: %w", err)
	}

	for _, n := range novels {
		count, err := q.vs.CountChunks(ctx, n.ID)
		if err != nil {
			q.logger.Warn("检查向量行数失败，跳过", "novel_id", n.ID, "err", err)
			continue
		}
		if count > 0 {
			q.logger.Info("向量已存在，跳过重建", "novel_id", n.ID, "count", count)
			continue
		}

		q.logger.Info("开始首次向量索引", "novel_id", n.ID, "title", n.Title)
		if err := q.RebuildNovel(ctx, n.ID); err != nil {
			q.logger.Error("小说向量重建失败", "novel_id", n.ID, "err", err)
			continue
		}
	}
	return nil
}

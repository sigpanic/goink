//go:build cgo

package rag

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/sigpanic/goink/internal/chapter"
	"github.com/sigpanic/goink/internal/git"
	"github.com/sigpanic/goink/internal/novel"
)

// RefreshTask 是一次向量刷新任务。
// 调用方按章节号语义提交（ChapterNumber），consumer 反查 ch.ID 后填充 ChapterID，
// 后续删除/索引一律用 ChapterID（chapters.id 外键）。
//
// TODO(v1.5.0): 反查是过渡设计——v1.5.0 支持删除/重排后章节号实时计算且不再存 DB，
// 反查会失效并引入竞态（去重窗口内章节号过期）。commit 3.1 将调用方（rw_tools/content）
// 路径解析改为 id 后，SubmitRefresh 改按 chapterID 提交，删除此反查与 ChapterNumber 字段。
type RefreshTask struct {
	NovelID       int64
	ChapterNumber int
	ChapterID     int64 // 反查 ch.ID 后填充（过渡期）
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
	// TODO(v1.5.0): 反查 ch.ID 是过渡实现（见 RefreshTask 注释），commit 3.1 改按 id 提交后删除。
	// 当前阶段（章节号稳定）无竞态；此后删除/索引一律用 chapter_id 外键。
	task.ChapterID = ch.ID

	if err := q.vs.DeleteChapterChunks(ctx, task.NovelID, task.ChapterID); err != nil {
		q.logger.Warn("删除章节旧向量失败", "chapter_id", task.ChapterID, "err", err)
	}

	params := ChapterChunkParams{
		ChapterNumber: task.ChapterNumber,
		ChapterID:     task.ChapterID,
		ChapterTitle:  ch.Title,
		Content:       task.Content,
		Summary:       ch.Summary,
	}
	chunks := BuildChapterChunks(params, GetTokenizer())
	if len(chunks) == 0 {
		return
	}

	if err := q.vs.IndexChunks(ctx, task.NovelID, chunks); err != nil {
		q.logger.Error("索引章节向量失败", "chapter_id", task.ChapterID, "err", err)
	}
}

// RebuildNovel 无条件全量重建一部小说的向量索引。
func (q *RefreshQueue) RebuildNovel(ctx context.Context, novelID int64) error {
	chapters, err := q.chStore.ListAllByNovel(ctx, novelID)
	if err != nil {
		return fmt.Errorf("rag: list chapters for rebuild: %w", err)
	}

	if len(chapters) == 0 {
		// 无章节：若有孤儿向量残留（如全章节被删），清空整表
		if err := q.vs.DeleteOrphanChunks(ctx, novelID, nil); err != nil {
			q.logger.Warn("清理孤儿向量失败", "novel_id", novelID, "err", err)
		}
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
			ChapterID:     ch.ID,
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

// RebuildAll 遍历全部小说，保证每部小说的向量索引完整：
//   - vec 表全空 → RebuildNovel 首次全量重建
//   - 有缺失章节 → 对缺失章节逐个增量补建（已索引章节不动）
//   - 有孤儿 chunk（章节已删除的残留 / chapter_id=0）→ 批量清理
//
// 覆盖度依据：vec 表 DISTINCT chapter_id 集合 vs chapters 表实际 id 集合。
// 幂等且可中断续跑：重建到一半被关闭，下次启动覆盖度检查发现缺失章节继续补建。
func (q *RefreshQueue) RebuildAll(ctx context.Context) error {
	var novels []novel.Novel
	if err := q.novelStore.DB.WithContext(ctx).Find(&novels).Error; err != nil {
		return fmt.Errorf("rag: list novels: %w", err)
	}

	for _, n := range novels {
		if err := q.rebuildNovelIfIncomplete(ctx, n.ID); err != nil {
			q.logger.Warn("向量完整性检查失败，跳过", "novel_id", n.ID, "err", err)
			continue
		}
	}
	return nil
}

// rebuildNovelIfIncomplete 对一部小说做覆盖度检查并按需重建/补建/清理。
func (q *RefreshQueue) rebuildNovelIfIncomplete(ctx context.Context, novelID int64) error {
	chapters, err := q.chStore.ListAllByNovel(ctx, novelID)
	if err != nil {
		return fmt.Errorf("rag: list chapters: %w", err)
	}
	if len(chapters) == 0 {
		return nil
	}

	indexedIDs, err := q.vs.DistinctChapterIDs(ctx, novelID)
	if err != nil {
		return err
	}
	// 全空：首次全量重建（批量路径，比逐章补建少 N 次 delete/embed 调用）
	if len(indexedIDs) == 0 {
		q.logger.Info("向量为空，首次全量索引", "novel_id", novelID, "chapters", len(chapters))
		return q.RebuildNovel(ctx, novelID)
	}

	validSet := make(map[int64]struct{}, len(chapters))
	for _, ch := range chapters {
		validSet[ch.ID] = struct{}{}
	}
	indexedSet := make(map[int64]struct{}, len(indexedIDs))
	for _, id := range indexedIDs {
		indexedSet[id] = struct{}{}
	}

	// 孤儿清理：vec 有但 chapters 没有（含 chapter_id=0 未知归属）
	validIDs := make([]int64, 0, len(chapters))
	orphanCount := 0
	for _, id := range indexedIDs {
		if _, ok := validSet[id]; !ok {
			orphanCount++
			continue
		}
		validIDs = append(validIDs, id)
	}
	if orphanCount > 0 {
		q.logger.Info("清理孤儿向量", "novel_id", novelID, "orphans", orphanCount)
		if err := q.vs.DeleteOrphanChunks(ctx, novelID, validIDs); err != nil {
			q.logger.Warn("清理孤儿向量失败", "novel_id", novelID, "err", err)
		}
	}

	// 缺失补建：chapters 有但 vec 没有
	var missing []chapter.Chapter
	for _, ch := range chapters {
		if _, ok := indexedSet[ch.ID]; !ok {
			missing = append(missing, ch)
		}
	}
	if len(missing) > 0 {
		q.logger.Info("检测到缺失章节，增量补建", "novel_id", novelID, "missing", len(missing))
		for _, ch := range missing {
			if err := q.rebuildChapter(ctx, novelID, ch); err != nil {
				q.logger.Warn("增量补建章节失败", "novel_id", novelID, "chapter_id", ch.ID, "err", err)
			}
		}
	}
	return nil
}

// rebuildChapter 增量重建单个章节的向量：删除旧 chunks 后重新索引该章。
// 已索引的其他章节不受影响，是 RebuildAll 覆盖度检查的补建路径。
func (q *RefreshQueue) rebuildChapter(ctx context.Context, novelID int64, ch chapter.Chapter) error {
	content, err := git.ReadFile(novelID, ch.FilePath)
	if err != nil {
		return fmt.Errorf("rag: read chapter file: %w", err)
	}
	params := ChapterChunkParams{
		ChapterNumber: ch.ChapterNumber,
		ChapterID:     ch.ID,
		ChapterTitle:  ch.Title,
		Content:       content,
		Summary:       ch.Summary,
	}
	chunks := BuildChapterChunks(params, GetTokenizer())
	if len(chunks) == 0 {
		return nil
	}
	if err := q.vs.DeleteChapterChunks(ctx, novelID, ch.ID); err != nil {
		return err
	}
	return q.vs.IndexChunks(ctx, novelID, chunks)
}

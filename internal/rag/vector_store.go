//go:build cgo

package rag

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/mattn/go-sqlite3"
)

// VectorStore 使用 sqlite-vec 管理向量索引，每部小说一张 vec0 虚拟表。
type VectorStore struct {
	db          *sql.DB
	embedder    Embedder
	log         *slog.Logger
	ensuredOnce sync.Map // map[int64]bool，避免重复 CREATE TABLE IF NOT EXISTS
}

// NewVectorStore 创建向量存储。db 应为已启用 sqlite-vec 扩展的 SQLite 连接。
func NewVectorStore(db *sql.DB, embedder Embedder, log *slog.Logger) *VectorStore {
	return &VectorStore{db: db, embedder: embedder, log: log}
}

func (s *VectorStore) tableName(novelID int64) string {
	return fmt.Sprintf("vec_novel_%d", novelID)
}

// ensureTable 确保指定小说的向量表存在，不存在则创建。结果缓存在 ensuredOnce 中。
func (s *VectorStore) ensureTable(ctx context.Context, novelID int64) error {
	if _, ok := s.ensuredOnce.Load(novelID); ok {
		return nil
	}

	tableName := s.tableName(novelID)
	// 迁移：旧表无 start_position 列时重建。列不存在和查询出错需区分处理。
	hasCol, colErr := tableHasColumn(ctx, s.db, tableName, "start_position")
	if colErr != nil {
		s.log.Warn("rag: 检查列失败，跳过迁移", "table", tableName, "err", colErr)
	} else if !hasCol {
		s.log.Info("rag: 旧表缺少 start_position 列，重建中", "table", tableName)
		if _, err := s.db.ExecContext(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName)); err != nil {
			return fmt.Errorf("rag: drop old vec table %s: %w", tableName, err)
		}
	}
	// v1.5.0 迁移：旧表仍用 chapter_number 列（迁移前 schema）时 DROP 重建。
	// vec0 虚拟表不支持改列名，只能重建为新 schema（chapter_id 列）。
	// 向量是派生索引，丢失后由 RebuildAll 覆盖度检查自动重建，无需数据搬迁。
	if hasOld, _ := tableHasColumn(ctx, s.db, tableName, "chapter_number"); hasOld {
		s.log.Warn("rag: 旧表仍含 chapter_number 列，重建为新 schema", "table", tableName)
		if _, err := s.db.ExecContext(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName)); err != nil {
			return fmt.Errorf("rag: drop legacy vec table %s: %w", tableName, err)
		}
	}
	sql := fmt.Sprintf(`CREATE VIRTUAL TABLE IF NOT EXISTS %s USING vec0(
		embedding float[512] distance_metric=cosine,
		chunk_id text,
		content text,
		chunk_type text,
		chapter_id integer,
		chunk_index integer,
		start_position integer
	)`, tableName)

	_, err := s.db.ExecContext(ctx, sql)
	if err != nil {
		return fmt.Errorf("rag: create vec table for novel %d: %w", novelID, err)
	}
	s.ensuredOnce.Store(novelID, true)
	return nil
}

// IndexChunks 将文本块批量生成 embedding 并在事务中写入向量表。
func (s *VectorStore) IndexChunks(ctx context.Context, novelID int64, chunks []Chunk) error {
	if len(chunks) == 0 {
		return nil
	}

	if err := s.ensureTable(ctx, novelID); err != nil {
		return err
	}

	// 批量生成 embedding，一次 ONNX Run。
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Content
	}
	embs, err := s.embedder.EmbedBatch(ctx, texts)
	if err != nil {
		return fmt.Errorf("rag: batch embed: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("rag: begin tx: %w", err)
	}
	defer tx.Rollback()

	tableName := s.tableName(novelID)
	for i, chunk := range chunks {
		v, err := sqlite_vec.SerializeFloat32(embs[i])
		if err != nil {
			return fmt.Errorf("rag: serialize chunk %s: %w", chunk.ID, err)
		}

		_, err = tx.ExecContext(ctx,
			fmt.Sprintf(`INSERT INTO %s (embedding, chunk_id, content, chunk_type, chapter_id, chunk_index, start_position) VALUES (?, ?, ?, ?, ?, ?, ?)`, tableName),
			v, chunk.ID, chunk.Content, chunk.ChunkType, chunk.ChapterID, chunk.ChunkIndex, chunk.StartRunePos,
		)
		if err != nil {
			return fmt.Errorf("rag: insert chunk %s: %w", chunk.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("rag: commit tx: %w", err)
	}

	s.log.Info("向量索引完成", "novel_id", novelID, "chunks", len(chunks))
	return nil
}

// Search 在指定小说的向量索引中执行语义检索。
func (s *VectorStore) Search(ctx context.Context, novelID int64, query string, topK int, filter *SearchFilter) ([]SearchResult, error) {
	if err := s.ensureTable(ctx, novelID); err != nil {
		return nil, err
	}

	queryEmb, err := s.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("rag: embed query: %w", err)
	}

	q, err := sqlite_vec.SerializeFloat32(queryEmb)
	if err != nil {
		return nil, fmt.Errorf("rag: serialize query: %w", err)
	}

	tableName := s.tableName(novelID)
	whereClauses := []string{}
	args := []any{q}

	if filter != nil {
		if len(filter.ChapterIDs) > 0 {
			placeholders := make([]string, len(filter.ChapterIDs))
			for i, id := range filter.ChapterIDs {
				placeholders[i] = "?"
				args = append(args, id)
			}
			whereClauses = append(whereClauses,
				fmt.Sprintf("chapter_id IN (%s)", strings.Join(placeholders, ",")))
		}
		if len(filter.ChapterNumbers) > 0 {
			// v1.5.0 后 vec 表已无 chapter_number 列，此过滤不再可用。
			// 调用方应改用 ChapterIDs（commit 4.3 迁移）。
			s.log.Warn("rag: Search filter.ChapterNumbers deprecated, use ChapterIDs", "values", filter.ChapterNumbers)
		}
		if len(filter.ChunkTypes) > 0 {
			placeholders := make([]string, len(filter.ChunkTypes))
			for i, t := range filter.ChunkTypes {
				placeholders[i] = "?"
				args = append(args, t)
			}
			whereClauses = append(whereClauses,
				fmt.Sprintf("chunk_type IN (%s)", strings.Join(placeholders, ",")))
		}
	}

	whereSQL := ""
	if len(whereClauses) > 0 {
		whereSQL = " AND " + strings.Join(whereClauses, " AND ")
	}

	querySQL := fmt.Sprintf(
		`SELECT chunk_id, content, chunk_type, chapter_id, distance, start_position, embedding FROM %s WHERE embedding MATCH ?%s ORDER BY distance LIMIT ?`,
		tableName, whereSQL,
	)
	args = append(args, topK)

	rows, err := s.db.QueryContext(ctx, querySQL, args...)
	if err != nil {
		return nil, fmt.Errorf("rag: search query: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var chunkID, content, chunkType string
		var chapterID int64
		var startRunePos int
		var distance float64
		var embBlob []byte
		if err := rows.Scan(&chunkID, &content, &chunkType, &chapterID, &distance, &startRunePos, &embBlob); err != nil {
			return nil, fmt.Errorf("rag: scan result: %w", err)
		}
		relevance := 1.0 - distance
		if relevance < 0 {
			relevance = 0
		}
		results = append(results, SearchResult{
			ChunkID:      chunkID,
			Content:      content,
			SourceType:   chunkType,
			ChapterID:    chapterID,
			StartRunePos: startRunePos,
			Distance:     distance,
			Relevance:    relevance,
			Embedding:    deserializeFloat32(embBlob),
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rag: iterate results: %w", err)
	}

	s.log.Info("向量检索完成", "novel_id", novelID, "query_len", len([]rune(query)), "results", len(results))
	return results, nil
}

// DeleteChapterChunks 删除指定章节的所有向量块。
// 参数 chapterID 为 chapters.id。
func (s *VectorStore) DeleteChapterChunks(ctx context.Context, novelID int64, chapterID int64) error {
	tableName := s.tableName(novelID)
	_, err := s.db.ExecContext(ctx,
		fmt.Sprintf(`DELETE FROM %s WHERE chapter_id = ?`, tableName),
		chapterID,
	)
	if err != nil {
		return fmt.Errorf("rag: delete chapter %d chunks: %w", chapterID, err)
	}
	s.log.Info("已删除章节向量", "novel_id", novelID, "chapter_id", chapterID)
	return nil
}

// CountChunks 返回指定小说的向量块总数。表不存在时自动创建后返回 0。
func (s *VectorStore) CountChunks(ctx context.Context, novelID int64) (int, error) {
	if err := s.ensureTable(ctx, novelID); err != nil {
		return 0, err
	}
	var count int
	err := s.db.QueryRowContext(ctx,
		fmt.Sprintf("SELECT COUNT(*) FROM %s", s.tableName(novelID))).Scan(&count)
	return count, err
}

// DistinctChapterIDs 返回向量表中已索引的去重章节 id 集合。
// 供 RebuildAll 覆盖度检查使用：与 chapters 表实际 id 对比，
// 找出缺失章节（增量补建）和孤儿 chunk（清理）。
func (s *VectorStore) DistinctChapterIDs(ctx context.Context, novelID int64) ([]int64, error) {
	if err := s.ensureTable(ctx, novelID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx,
		fmt.Sprintf("SELECT DISTINCT chapter_id FROM %s", s.tableName(novelID)))
	if err != nil {
		return nil, fmt.Errorf("rag: distinct chapter_id: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("rag: scan distinct chapter_id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rag: iterate distinct chapter_id: %w", err)
	}
	return ids, nil
}

// DeleteOrphanChunks 删除向量表中 chapter_id 不在有效列表内的行（孤儿 chunk）。
// 典型场景：章节被删除后其向量残留，或迁移期写入的 chapter_id=0 未知归属行。
// 不触发 DROP/CREATE，直接 DELETE；validChapterIDs 为空时清空整表。
func (s *VectorStore) DeleteOrphanChunks(ctx context.Context, novelID int64, validChapterIDs []int64) error {
	tableName := s.tableName(novelID)
	if len(validChapterIDs) == 0 {
		_, err := s.db.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s", tableName))
		if err != nil {
			return fmt.Errorf("rag: clear orphan chunks for novel %d: %w", novelID, err)
		}
		return nil
	}

	placeholders := make([]string, len(validChapterIDs))
	args := make([]any, len(validChapterIDs))
	for i, id := range validChapterIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	sqlStr := fmt.Sprintf(
		"DELETE FROM %s WHERE chapter_id IS NULL OR chapter_id NOT IN (%s)",
		tableName, strings.Join(placeholders, ","))
	if _, err := s.db.ExecContext(ctx, sqlStr, args...); err != nil {
		return fmt.Errorf("rag: delete orphan chunks for novel %d: %w", novelID, err)
	}
	return nil
}

// DeleteNovel 删除整部小说的向量表。
func (s *VectorStore) DeleteNovel(ctx context.Context, novelID int64) error {
	tableName := s.tableName(novelID)
	_, err := s.db.ExecContext(ctx,
		fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName),
	)
	if err != nil {
		return fmt.Errorf("rag: drop table for novel %d: %w", novelID, err)
	}
	s.ensuredOnce.Delete(novelID)
	s.log.Info("已删除小说向量表", "novel_id", novelID)
	return nil
}

// ── 全局单例 ──────────────────────────────────────────────

var (
	globalVSOnce sync.Once
	globalVS     *VectorStore
)

// InitVectorStore 初始化全局 VectorStore，多次调用只生效一次。
func InitVectorStore(db *sql.DB, embedder Embedder, log *slog.Logger) {
	globalVSOnce.Do(func() {
		globalVS = NewVectorStore(db, embedder, log)
	})
}

// tableHasColumn 检查虚拟表中是否存在指定列。vec0 不支持 PRAGMA，
// 所以通过查询后 Columns() 判断。返回 (exists, error)。
func tableHasColumn(ctx context.Context, db *sql.DB, tableName, col string) (bool, error) {
	rows, err := db.QueryContext(ctx, fmt.Sprintf("SELECT * FROM %s LIMIT 0", tableName))
	if err != nil {
		return false, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return false, err
	}
	for _, c := range cols {
		if strings.EqualFold(c, col) {
			return true, nil
		}
	}
	return false, nil
}

// GetVectorStore 返回全局 VectorStore，未初始化时返回 nil。
func GetVectorStore() *VectorStore {
	return globalVS
}

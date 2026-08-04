package imp

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/chapter"
	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/git"
	"github.com/sigpanic/goink/internal/novel"
	"github.com/sigpanic/goink/internal/text"
)

// ImportResult 是完整导入流程的结果。
type ImportResult struct {
	NovelID         int64            `json:"novel_id"`
	Title           string           `json:"title"`
	ChapterCount    int              `json:"chapter_count"`
	SkippedCount    int              `json:"skipped_count"`
	SkippedChapters []SkippedChapter `json:"skipped_chapters"`
	NeedsLLM        bool             `json:"needs_llm"`           // 正则分割结果不合理，提示前端可调用 LLM 分析
	FilePath        string           `json:"file_path,omitempty"` // NeedsLLM 时回传文件路径，供前端调 ImportWithLLM
}

// ProgressCallback 是进度回调函数，app 层用它来推送前端事件。
type ProgressCallback func(stage, message string, current, total, percent int, novelID int64)

// Import 执行完整的小说导入流程：解析文件 → 创建 Novel → 写入章节 → git 提交。
func Import(ctx context.Context, logger *slog.Logger, db *gorm.DB, filePath string, gitName, gitEmail string, onProgress ProgressCallback) (*ImportResult, error) {
	onProgress("parse", "正在解析文件", 0, 0, 0, 0)

	result, err := Parse(filePath, logger)
	if err != nil {
		onProgress("error", "导入解析失败，已停止导入", 0, 0, 0, 0)
		return nil, fmt.Errorf("导入解析失败: %w", err)
	}

	// 正则分割结果不合理，直接返回 NeedsLLM 标记，不创建 Novel
	// FilePath 回传给前端，供其调用 ImportWithLLM
	if result.NeedsLLM {
		return &ImportResult{
			Title:    result.Title,
			NeedsLLM: true,
			FilePath: filePath,
		}, nil
	}

	title := result.Title
	if title == "" {
		title = "未命名"
	}

	return doImport(ctx, logger, db, result, title, filePath, gitName, gitEmail, onProgress)
}

// ImportWithResult 使用已有的解析结果执行完整导入流程。
// 用于 LLM 兜底场景：先 AnalyzeWithLLM → ImportWithResult。
func ImportWithResult(ctx context.Context, logger *slog.Logger, db *gorm.DB, result *Result, filePath string, gitName, gitEmail string, onProgress ProgressCallback) (*ImportResult, error) {
	title := result.Title
	if title == "" {
		title = inferTitle(filePath)
		if title == "" {
			title = "未命名"
		}
	}

	return doImport(ctx, logger, db, result, title, filePath, gitName, gitEmail, onProgress)
}

// doImport 执行导入的核心流程：创建 Novel → 写入章节 → git add → DB 事务 → git commit。
// Import 和 ImportWithResult 共享此逻辑。
//
// 事务策略（参考 RollbackBeforeTurn 三步模式）：
//  1. 创建 Novel + 写文件 + git add（可逆操作，不 commit）
//  2. DB 事务写入 Chapters（原子操作）
//  3. git commit（不可逆但极低失败率，放在 DB 成功之后）
func doImport(ctx context.Context, logger *slog.Logger, db *gorm.DB, result *Result, title, filePath, gitName, gitEmail string, onProgress ProgressCallback) (*ImportResult, error) {
	chapterCount := len(result.Chapters)
	onProgress("create_novel", "正在创建作品", 0, chapterCount, 10, 0)

	n := novel.Novel{
		Title:       title,
		Description: fmt.Sprintf("从 %s 导入，共 %d 章", filePath, chapterCount),
	}

	// ── Step 1: 创建 Novel 记录 + 文件操作 + git add（不 commit） ──
	// Novel 必须先创建以获取 ID（文件路径依赖此 ID）。
	// 如果后续步骤失败，cleanupImport 会删除 Novel 记录和文件。

	if err := db.WithContext(ctx).Create(&n).Error; err != nil {
		onProgress("error", "创建小说失败", 0, chapterCount, 0, 0)
		return nil, fmt.Errorf("创建小说失败: %w", err)
	}

	repo, err := git.New(n.ID, gitName, gitEmail, logger)
	if err != nil {
		cleanupImport(db, ctx, logger, n.ID)
		onProgress("error", "初始化小说仓库失败", 0, chapterCount, 0, n.ID)
		return nil, fmt.Errorf("初始化小说仓库失败: %w", err)
	}

	if err := git.WriteFile(n.ID, git.GoinkPath(), ""); err != nil {
		cleanupImport(db, ctx, logger, n.ID)
		onProgress("error", "创建 goink.md 失败", 0, chapterCount, 0, n.ID)
		return nil, fmt.Errorf("创建 goink.md 失败: %w", err)
	}

	onProgress("write_chapters", "正在写入章节", 0, chapterCount, 15, n.ID)
	for i, ch := range result.Chapters {
		chapNum := i + 1
		if err := git.WriteFile(n.ID, git.ChapterPath(chapNum), ch.Content); err != nil {
			cleanupImport(db, ctx, logger, n.ID)
			onProgress("error", fmt.Sprintf("写入第%d章失败", chapNum), chapNum, chapterCount, 0, n.ID)
			return nil, fmt.Errorf("写入第%d章失败: %w", chapNum, err)
		}
		onProgress(
			"write_chapters",
			fmt.Sprintf("正在写入第 %d/%d 章", chapNum, chapterCount),
			chapNum,
			chapterCount,
			importProgressPercent(chapNum, chapterCount),
			n.ID,
		)
	}

	onProgress("commit", "正在暂存导入文件", chapterCount, chapterCount, 90, n.ID)
	if err := repo.StageAll(); err != nil {
		cleanupImport(db, ctx, logger, n.ID)
		onProgress("error", "暂存导入文件失败", 0, chapterCount, 0, n.ID)
		return nil, fmt.Errorf("暂存导入文件失败: %w", err)
	}

	// ── Step 2: DB 事务写入 Chapters（原子操作） ──
	onProgress("saving_metadata", "正在保存章节元数据", chapterCount, chapterCount, 93, n.ID)
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for i, ch := range result.Chapters {
			chapNum := i + 1
			chap := chapter.Chapter{
				NovelID:       n.ID,
				ChapterNumber: chapNum,
				Title:         ch.Title,
				WordCount:     text.ComputeStats(ch.Content).WordCount,
			}
			if err := tx.Create(&chap).Error; err != nil {
				return fmt.Errorf("创建第%d章元数据失败: %w", chapNum, err)
			}
		}
		return nil
	}); err != nil {
		// DB 写入失败，完整清理：删除 Novel 记录 + 文件目录
		cleanupImport(db, ctx, logger, n.ID)
		onProgress("error", "导入失败，已撤销本次导入产生的数据和文件", 0, chapterCount, 0, n.ID)
		return nil, err
	}

	// ── Step 3: git commit（不可逆但极低失败率） ──
	// DB 已提交，Chapters 已创建。git commit 失败时数据完整，只是缺版本记录。
	if _, err := repo.Commit(fmt.Sprintf("import novel: %s", title)); err != nil {
		// 不回滚 DB：数据完整，只是缺 git 初始提交
		// 下次任何文件变更都会触发新的 commit，数据不会丢失
		logger.Error("导入提交 git commit 失败，数据已保存但缺少版本记录，下次编辑将自动提交",
			"novel_id", n.ID, "err", err)
		onProgress("done", fmt.Sprintf("已导入《%s》，共 %d 章（版本记录待下次编辑自动提交）", title, chapterCount), chapterCount, chapterCount, 100, n.ID)
	} else {
		onProgress("done", fmt.Sprintf("已导入《%s》，共 %d 章", title, chapterCount), chapterCount, chapterCount, 100, n.ID)
	}

	return &ImportResult{
		NovelID:         n.ID,
		Title:           title,
		ChapterCount:    chapterCount,
		SkippedCount:    len(result.SkippedChapters),
		SkippedChapters: result.SkippedChapters,
	}, nil
}

// cleanupImport 清理导入失败后的残留：删除 Novel 记录和文件目录。
func cleanupImport(db *gorm.DB, ctx context.Context, logger *slog.Logger, novelID int64) {
	// 删除文件目录（含 .git 仓库）
	if err := os.RemoveAll(config.NovelDirPath(novelID)); err != nil {
		logger.Error("清理导入失败后的小说目录失败", "novel_id", novelID, "err", err)
	}
	// 删除 Novel 记录
	if err := db.WithContext(ctx).Delete(&novel.Novel{}, novelID).Error; err != nil {
		logger.Error("清理导入失败后的 Novel 记录失败", "novel_id", novelID, "err", err)
	}
}

func importProgressPercent(current, total int) int {
	if total <= 0 {
		return 15
	}
	percent := 15 + current*75/total
	if percent > 90 {
		return 90
	}
	return percent
}

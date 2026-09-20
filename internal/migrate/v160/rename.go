package v160

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"syscall"

	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/git"
)

// migrateRenameFiles（commit 1.6）：
//  1. chapter.sort_order 初始化 = chapter_number（保留原顺序）
//  2. 每个 novel 仓库建 volumes/ 目录 + .gitkeep（卷纲 volumes/{id}.md 落位）
//  3. 章节文件 rename：chapters/{num:03d}.md → chapters/id_{id}.md、outlines/{num:03d}.md → outlines/id_{id}.md
//  4. 按 novel 各自 git commit
//
// 依赖 chapter_number 列仍存在（commit 1.7 才删），因此必须排在 drop-legacy 之前。
//
// 幂等可重入：
//   - sort_order = 0 才初始化（已初始化的跳过）
//   - volumes/ 已存在跳过
//   - rename：目标文件已存在 → 跳过（已迁移）；源文件不存在 → 跳过（该章节无正文/大纲）
//   - git commit：工作区无变更（HasUncommitted）→ 跳过
//
// 单个 novel 失败只告警不阻塞（7.5），step 整体幂等，下次启动重跑剩余部分。
func migrateRenameFiles(db *gorm.DB, log *slog.Logger) error {
	// 新装库已由当前 model 直接建成 id 化 schema，没有旧文件或 chapter_number 可迁移。
	// 迁移状态丢失时也必须安全跳过，不能查询已经删除的旧列。
	if !db.Migrator().HasTable("chapters") || !db.Migrator().HasColumn("chapters", "chapter_number") {
		return nil
	}

	// 1. sort_order 初始化 = chapter_number
	if err := db.Exec("UPDATE chapters SET sort_order = chapter_number WHERE sort_order = 0").Error; err != nil {
		return fmt.Errorf("migrate v160: 初始化 sort_order: %w", err)
	}

	// 遍历全部 novel（novels 表仅存元数据，目录即 config.NovelDirPath(novelID)）
	var novelIDs []int64
	if err := db.Table("novels").Order("id").Pluck("id", &novelIDs).Error; err != nil {
		return fmt.Errorf("migrate v160: 读取 novels: %w", err)
	}
	for _, novelID := range novelIDs {
		if err := migrateNovelFiles(db, log, novelID); err != nil {
			log.Warn("migrate v160: 该 novel 文件迁移失败（继续其他 novel）", "novel_id", novelID, "err", err)
		}
	}
	return nil
}

// migrateNovelFiles 处理单个 novel：建 volumes/ + 章节/大纲文件 rename + git commit。
func migrateNovelFiles(db *gorm.DB, log *slog.Logger, novelID int64) error {
	dir := config.NovelDirPath(novelID)

	// 2. volumes/ 目录 + .gitkeep（幂等：已存在跳过）
	volumesDir := filepath.Join(dir, "volumes")
	if _, err := os.Stat(volumesDir); err != nil {
		if err := os.MkdirAll(volumesDir, 0o755); err != nil {
			return fmt.Errorf("migrate v160: 创建 volumes/ 目录: %w", err)
		}
		if err := os.WriteFile(filepath.Join(volumesDir, ".gitkeep"), nil, 0o644); err != nil {
			return fmt.Errorf("migrate v160: 写入 volumes/.gitkeep: %w", err)
		}
	}

// 3. 章节/大纲文件 rename：{num:03d} → id_{id}（源按旧 num 命名，目标按新 id 命名）
	type chRow struct {
		ID            int64
		ChapterNumber int
	}
	var rows []chRow
	if err := db.Table("chapters").Select("id, chapter_number").
		Where("novel_id = ?", novelID).Order("id").Scan(&rows).Error; err != nil {
		return fmt.Errorf("migrate v160: 读取 chapters: %w", err)
	}
	for _, r := range rows {
		// 正文：chapters/001.md → chapters/id_1.md
		if err := renameChapterFile(dir, chapterNumPath(r.ChapterNumber), chapterIDPath(r.ID)); err != nil {
			log.Warn("migrate v160: 章节文件 rename 失败", "novel_id", novelID, "id", r.ID, "err", err)
		}
		// 大纲：outlines/001.md → outlines/id_1.md
		if err := renameChapterFile(dir, outlineNumPath(r.ChapterNumber), outlineIDPath(r.ID)); err != nil {
			log.Warn("migrate v160: 大纲文件 rename 失败", "novel_id", novelID, "id", r.ID, "err", err)
		}
	}

	// 4. 按 novel git commit（工作区有变更才提交）
	repo, err := git.New(novelID, "", "", log)
	if err != nil {
		return fmt.Errorf("migrate v160: 打开 novel git 仓库: %w", err)
	}
	uncommitted, err := repo.HasUncommitted()
	if err != nil {
		return fmt.Errorf("migrate v160: 检查 git 状态: %w", err)
	}
	if !uncommitted {
		return nil
	}
	if err := repo.StageAll(); err != nil {
		return fmt.Errorf("migrate v160: git stage: %w", err)
	}
	if _, err := repo.Commit("migrate: rename chapter files to id-based names"); err != nil {
		return fmt.Errorf("migrate v160: git commit: %w", err)
	}
	log.Info("migrate v160: 章节文件 rename 完成", "novel_id", novelID)
	return nil
}

// chapterNumPath / outlineNumPath 是历史 num 命名的源路径（仅迁移内部使用，
// 等价旧 git.ChapterPath/OutlinePath 的 num 版本；commit 2 起 git 包只提供 id 版本）。
func chapterNumPath(num int) string { return fmt.Sprintf("chapters/%03d.md", num) }
func outlineNumPath(num int) string { return fmt.Sprintf("outlines/%03d.md", num) }

// chapterIDPath / outlineIDPath 是迁移后的 id 命名路径（id_ 前缀）。
// 前缀将 id 命名空间与旧 num 的纯数字命名空间（chapters/001.md）完全隔离：
// 任何 num 值都不可能撞名，目标存在即本文件已完成迁移，rename 判断结构性可靠。
// 与代码层 commit 2 的 ChapterPath/OutlinePath 改 id 后的格式一致（chapters/id_\d+\.md）。
func chapterIDPath(id int64) string { return fmt.Sprintf("chapters/id_%d.md", id) }
func outlineIDPath(id int64) string { return fmt.Sprintf("outlines/id_%d.md", id) }

// renameChapterFile 将源相对路径 rename 到目标相对路径。
//
// 幂等：目标已存在 → 跳过（已迁移，中断恢复）；源不存在 → 跳过（该章节无此文件）。
// 跨设备（EXDEV，如仓库在别的挂载点）退化：拷贝 + 删除源。
func renameChapterFile(dir, srcRel, dstRel string) error {
	src := filepath.Join(dir, srcRel)
	dst := filepath.Join(dir, dstRel)
	if _, err := os.Stat(dst); err == nil {
		return nil // 目标已存在 → 已迁移
	}
	if _, err := os.Stat(src); err != nil {
		return nil // 源不存在 → 该章节无正文/大纲文件
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("migrate v160: 创建目标目录: %w", err)
	}
	if err := os.Rename(src, dst); err != nil {
		if errors.Is(err, syscall.EXDEV) {
			if cerr := copyFileX(src, dst); cerr != nil {
				return fmt.Errorf("migrate v160: 跨设备拷贝 %s: %w", srcRel, cerr)
			}
			if rerr := os.Remove(src); rerr != nil {
				return fmt.Errorf("migrate v160: 跨设备删除源 %s: %w", srcRel, rerr)
			}
			return nil
		}
		return fmt.Errorf("migrate v160: rename %s → %s: %w", srcRel, dstRel, err)
	}
	return nil
}

// copyFileX 拷贝单文件（EXDEV 退化的 cp）。
func copyFileX(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

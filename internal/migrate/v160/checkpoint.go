package v160

import (
	"errors"
	"fmt"
	"log/slog"

	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/git"
)

const checkpointCommitMessage = "checkpoint: save changes before chapter ID migration"

// migrateCheckpointNovelRepositories 将迁移开始前已有的小说仓库改动单独提交。
// 它必须在任何 schema 或文件变更前运行：完成后写入独立 step 状态，后续文件迁移
// 中断重试时不会把迁移自身留下的工作区改动误提交为 checkpoint。
func migrateCheckpointNovelRepositories(db *gorm.DB, log *slog.Logger) error {
	// 已完成 v1.6.0 的旧开发库可能缺少后来新增的 checkpoint step。此时不能因
	// 补状态而提交用户当前改动；chapter_number 仍存在才说明文件迁移尚未开始。
	if !db.Migrator().HasTable("chapters") || !db.Migrator().HasColumn("chapters", "chapter_number") {
		return nil
	}

	var novelIDs []int64
	if err := db.Table("novels").Order("id").Pluck("id", &novelIDs).Error; err != nil {
		return fmt.Errorf("migrate v160: 读取 novels: %w", err)
	}

	var errs []error
	for _, novelID := range novelIDs {
		repo, err := git.New(novelID, "", "", log)
		if err != nil {
			errs = append(errs, fmt.Errorf("novel %d: 打开 git 仓库: %w", novelID, err))
			continue
		}
		dirty, err := repo.HasUncommitted()
		if err != nil {
			errs = append(errs, fmt.Errorf("novel %d: 检查 git 状态: %w", novelID, err))
			continue
		}
		if !dirty {
			continue
		}
		if err := repo.StageAll(); err != nil {
			errs = append(errs, fmt.Errorf("novel %d: 暂存 checkpoint: %w", novelID, err))
			continue
		}
		if _, err := repo.Commit(checkpointCommitMessage); err != nil {
			errs = append(errs, fmt.Errorf("novel %d: 提交 checkpoint: %w", novelID, err))
			continue
		}
		log.Info("migrate v160: 已提交迁移前小说仓库改动", "novel_id", novelID)
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("migrate v160: 小说仓库 checkpoint 未全部完成: %w", err)
	}
	return nil
}

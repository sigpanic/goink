package migrate

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/platform"
)

// backupBeforeMigrate 在破坏性迁移前备份数据目录，提供恢复兜底。migration 为本次迁移标识
// （如 "v1.6.0-chapter-id-refactor"），未来新迁移传新标识即可复用：目录名/组判断/临时目录均按它隔离。
//
// 仅在"迁移需要执行"时备份一次，迁移完成后不再备份：
//   - migrate_state 中本迁移组（migration = migration）有记录且全部 done → 跳过
//   - 备份目录 backups/{migration}/ 已存在（完整备份）→ 跳过
//   - DB 文件不存在（无数据库文件）→ 跳过
//   - 无 chapters 表（全新库，无历史数据）→ 跳过，避免备份空库
//     （注意：SQLite 连接即建文件，GlobalDBPath() 在新库时也已存在，"DB 文件不存在"守卫挡不住新库）
//
// 备份内容（全量拷贝）：novel-agent.db + novels/（含每个 novel 的 git 仓库）。
// 备份目录 = backups/{migration}/，与 migrate_state 的 migration 标识对应，未来迁移各占一目录互不覆盖。
//
// 原子性：先拷贝到临时目录 backups/.tmp-{migration}-{pid}/，成功后 os.Rename 到正式目录。
// 正式目录只存在"完整"或"不存在"两种状态，无半截备份；失败删临时目录，下次启动重来；
// 崩溃残留的临时目录下次启动清理。备份失败只告警不阻塞迁移（防御措施，不当硬前置）。
func backupBeforeMigrate(db *gorm.DB, log *slog.Logger, migration string) error {
	// 本迁移组（migration）有记录且全部 done → 迁移已完成，无需再备份
	var total, done int64
	db.Model(&MigrateState{}).Where("migration = ?", migration).Count(&total)
	db.Model(&MigrateState{}).Where("migration = ? AND status = ?", migration, "done").Count(&done)
	if total > 0 && total == done {
		log.Info("迁移已完成，跳过备份", "migration", migration)
		return nil
	}

	// DB 不存在（无数据库文件）→ 无需备份
	if _, err := os.Stat(config.GlobalDBPath()); err != nil {
		log.Info("无数据库文件，跳过备份")
		return nil
	}

	// 全新库（无 chapters 表，无历史数据）→ 无需备份，避免备份空库
	if !db.Migrator().HasTable("chapters") {
		log.Info("新库，跳过备份")
		return nil
	}

	backupRoot := filepath.Join(platform.DataDir(), "backups")
	dest := filepath.Join(backupRoot, migration)

	// 完整备份已存在 → 跳过（迁移名目录天然幂等）
	if _, err := os.Stat(dest); err == nil {
		log.Info("备份已存在，跳过", "backup", dest)
		return nil
	}

	// 清理残留临时目录（上次备份崩溃遗留）
	cleanupStaleTempBackups(backupRoot, migration, log)

	// 拷贝到临时目录，成功后原子 rename 到正式目录
	tmp := filepath.Join(backupRoot, fmt.Sprintf(".tmp-%s-%d", migration, os.Getpid()))
	if err := os.RemoveAll(tmp); err != nil {
		return fmt.Errorf("backup: clear tmp %s: %w", tmp, err)
	}
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return fmt.Errorf("backup: mkdir tmp %s: %w", tmp, err)
	}

	// 1. 拷贝数据库
	if err := copyFile(config.GlobalDBPath(), filepath.Join(tmp, "novel-agent.db")); err != nil {
		removeTmp(log, tmp)
		return fmt.Errorf("backup db: %w", err)
	}

	// 2. 拷贝 novels/ 目录（含每个 novel 的 git 仓库）
	novelsSrc := filepath.Join(platform.DataDir(), "novels")
	if _, err := os.Stat(novelsSrc); err == nil {
		if err := copyDir(novelsSrc, filepath.Join(tmp, "novels")); err != nil {
			removeTmp(log, tmp)
			return fmt.Errorf("backup novels: %w", err)
		}
	}

	// 3. 原子 rename：正式目录只出现完整备份
	if err := os.Rename(tmp, dest); err != nil {
		removeTmp(log, tmp)
		return fmt.Errorf("backup: rename to %s: %w", dest, err)
	}

	log.Info("迁移前备份完成", "backup", dest)
	return nil
}

// removeTmp 删除临时备份目录，失败仅告警（残留会被下次启动 cleanupStaleTempBackups 清理）。
func removeTmp(log *slog.Logger, tmp string) {
	if err := os.RemoveAll(tmp); err != nil {
		log.Warn("清理临时备份目录失败（残留下次启动清理）", "path", tmp, "err", err)
	}
}

// cleanupStaleTempBackups 清理 backups/ 下本迁移残留的临时备份目录（上次备份崩溃遗留）。
func cleanupStaleTempBackups(backupRoot, name string, log *slog.Logger) {
	entries, err := os.ReadDir(backupRoot)
	if err != nil {
		return
	}
	prefix := ".tmp-" + name + "-"
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), prefix) {
			p := filepath.Join(backupRoot, e.Name())
			if err := os.RemoveAll(p); err != nil {
				log.Warn("清理残留临时备份失败", "path", p, "err", err)
				continue
			}
			log.Info("清理残留临时备份", "path", p)
		}
	}
}

// copyFile 拷贝单个文件，保留原权限位。
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// copyDir 递归拷贝目录。符号链接原样重建（不跟随），避免跨设备或循环链接问题。
func copyDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, e := range entries {
		s := filepath.Join(src, e.Name())
		d := filepath.Join(dst, e.Name())
		if e.IsDir() {
			if err := copyDir(s, d); err != nil {
				return err
			}
			continue
		}
		if e.Type()&os.ModeSymlink != 0 {
			target, err := os.Readlink(s)
			if err != nil {
				return err
			}
			if err := os.Symlink(target, d); err != nil {
				return err
			}
			continue
		}
		if err := copyFile(s, d); err != nil {
			return err
		}
	}
	return nil
}

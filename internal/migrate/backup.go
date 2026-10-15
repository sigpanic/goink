package migrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mattn/go-sqlite3"
	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/config"
	"github.com/sigpanic/goink/internal/migrate/engine"
	"github.com/sigpanic/goink/internal/platform"
)

const (
	backupBusyRetryLimit = 500
	backupBusyRetryDelay = 10 * time.Millisecond
)

// prepareUpgradeBackup 仅预检备份需求，不固定后续逐组执行的计划或登记完成状态。
// 从首个待执行组开始，只要后续存在破坏性组就保守提前备份，不再区分后续组的计划状态。
// 起点使用首个待执行组之前最后完成的迁移版本；缺少记录时使用 legacy。
func prepareUpgradeBackup(db *gorm.DB, log *slog.Logger, isNewDatabase bool, registry []engine.Migration) error {
	if isNewDatabase {
		return nil
	}
	plans, err := engine.BuildPlans(db, log, false, registry)
	if err != nil {
		return fmt.Errorf("inspect upgrade backup requirements: %w", err)
	}
	from := "legacy"
	mayRun, needsBackup := false, false
	for _, plan := range plans {
		if !mayRun && plan.Action == engine.PlanSkip {
			from = migrationVersion(plan.Migration.Name)
		}
		mayRun = mayRun || plan.Action == engine.PlanRun
		if mayRun && plan.Migration.Destructive {
			needsBackup = true
			break
		}
	}
	if !needsBackup {
		return nil
	}
	to := migrationVersion(registry[len(registry)-1].Name)
	name, err := findUpgradeBackup(filepath.Join(platform.DataDir(), "backups"), to)
	if err != nil {
		return err
	}
	if name == "" {
		name = from + "-upgrade-to-" + to
	}
	return backupBeforeUpgrade(db, log, name)
}

func migrationVersion(name string) string {
	version, _, _ := strings.Cut(name, "-")
	return version
}

// findUpgradeBackup 按目标复用完整备份，避免重启时起点推进造成重复备份。
// 只识别本轮命名，既有按迁移组命名的备份保持不变；不跨不同目标接管备份。
func findUpgradeBackup(root, to string) (string, error) {
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read upgrade backups: %w", err)
	}
	var found string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".tmp-") || !strings.HasSuffix(entry.Name(), "-upgrade-to-"+to) {
			continue
		}
		if !entry.IsDir() {
			return "", fmt.Errorf("upgrade backup is not a directory: %s", entry.Name())
		}
		if found != "" {
			return "", fmt.Errorf("multiple upgrade backups for target %s", to)
		}
		found = entry.Name()
	}
	return found, nil
}

// backupBeforeUpgrade 生成一份整轮升级的数据库和小说目录起始快照，不再自行判断迁移状态。
// name 由备份预检确定，如 v1.6.1-upgrade-to-v1.8.0；完整目录已存在时直接复用。
// 备份内容：通过 SQLite Online Backup API 取得的 novel-agent.db 一致性快照 + novels/（含每个 novel 的 git 仓库）。
// 原子性：先拷贝到临时目录 backups/.tmp-{name}-{pid}/，成功后 os.Rename 到正式目录。
// 正式目录只存在"完整"或"不存在"两种状态，无半截备份；失败删临时目录，下次启动重来；
// 崩溃残留的临时目录下次启动清理。备份是破坏性迁移的硬前置：失败时迁移不会开始。
func backupBeforeUpgrade(db *gorm.DB, log *slog.Logger, name string) error {
	// DB 不存在（无数据库文件）→ 无需备份
	if _, err := os.Stat(config.GlobalDBPath()); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("stat upgrade database: %w", err)
		}
		log.Info("无数据库文件，跳过备份")
		return nil
	}

	backupRoot := filepath.Join(platform.DataDir(), "backups")
	dest := filepath.Join(backupRoot, name)

	// 完整备份已存在 → 跳过，保留本轮首次备份的起始状态。
	if info, err := os.Stat(dest); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("upgrade backup is not a directory: %s", dest)
		}
		log.Info("备份已存在，跳过", "backup", dest)
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat upgrade backup: %w", err)
	}

	// 清理残留临时目录（上次备份崩溃遗留）
	cleanupStaleTempBackups(backupRoot, name, log)

	// 拷贝到临时目录，成功后原子 rename 到正式目录
	tmp := filepath.Join(backupRoot, fmt.Sprintf(".tmp-%s-%d", name, os.Getpid()))
	if err := os.RemoveAll(tmp); err != nil {
		return fmt.Errorf("backup: clear tmp %s: %w", tmp, err)
	}
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return fmt.Errorf("backup: mkdir tmp %s: %w", tmp, err)
	}

	// 1. 从已打开的 SQLite 连接生成一致性快照。不能直接复制主数据库文件：
	// WAL 中的已提交事务可能尚未 checkpoint 到该文件。
	if err := backupSQLiteDatabase(context.Background(), db, filepath.Join(tmp, "novel-agent.db")); err != nil {
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

// backupSQLiteDatabase 使用 SQLite Online Backup API 创建 source 的一致性快照。
// 目标文件由 API 直接写入，已包含源库 WAL 中的已提交数据，不应另行复制 -wal 或 -shm 文件。
func backupSQLiteDatabase(ctx context.Context, source *gorm.DB, targetPath string) error {
	sourceDB, err := source.DB()
	if err != nil {
		return fmt.Errorf("get source sql.DB: %w", err)
	}
	sourceConn, err := sourceDB.Conn(ctx)
	if err != nil {
		return fmt.Errorf("get source connection: %w", err)
	}
	defer sourceConn.Close()

	targetDB, err := sql.Open("sqlite3", targetPath)
	if err != nil {
		return fmt.Errorf("open target database: %w", err)
	}
	defer targetDB.Close()
	targetConn, err := targetDB.Conn(ctx)
	if err != nil {
		return fmt.Errorf("get target connection: %w", err)
	}
	defer targetConn.Close()

	return targetConn.Raw(func(targetDriverConn any) error {
		targetSQLiteConn, ok := targetDriverConn.(*sqlite3.SQLiteConn)
		if !ok {
			return fmt.Errorf("unexpected target SQLite driver connection %T", targetDriverConn)
		}
		return sourceConn.Raw(func(sourceDriverConn any) (err error) {
			sourceSQLiteConn, ok := sourceDriverConn.(*sqlite3.SQLiteConn)
			if !ok {
				return fmt.Errorf("unexpected source SQLite driver connection %T", sourceDriverConn)
			}

			backup, err := initializeSQLiteBackup(ctx, targetSQLiteConn, sourceSQLiteConn)
			if err != nil {
				return err
			}
			defer func() {
				if finishErr := backup.Finish(); finishErr != nil && err == nil {
					err = fmt.Errorf("finish SQLite backup: %w", finishErr)
				}
			}()

			for attempt := 0; ; attempt++ {
				done, stepErr := backup.Step(-1)
				if stepErr != nil {
					return fmt.Errorf("copy SQLite backup: %w", stepErr)
				}
				if done {
					return nil
				}
				// go-sqlite3 对 SQLITE_BUSY/SQLITE_LOCKED 返回 (false, nil)。
				// 迁移发生在启动期，通常不会重试；限制次数避免占用锁时无限等待。
				if retryErr := waitForSQLiteBackupRetry(ctx, attempt); retryErr != nil {
					return fmt.Errorf("copy SQLite backup: %w", retryErr)
				}
			}
		})
	})
}

// initializeSQLiteBackup 对短暂的 SQLite 锁冲突有限重试；其他错误立即返回。
func initializeSQLiteBackup(ctx context.Context, target, source *sqlite3.SQLiteConn) (*sqlite3.SQLiteBackup, error) {
	for attempt := 0; ; attempt++ {
		backup, err := target.Backup("main", source, "main")
		if err == nil {
			return backup, nil
		}
		if !isSQLiteBusy(err) {
			return nil, fmt.Errorf("initialize SQLite backup: %w", err)
		}
		if retryErr := waitForSQLiteBackupRetry(ctx, attempt); retryErr != nil {
			return nil, fmt.Errorf("initialize SQLite backup: %w", retryErr)
		}
	}
}

func isSQLiteBusy(err error) bool {
	var sqliteErr sqlite3.Error
	return errors.As(err, &sqliteErr) && (sqliteErr.Code == sqlite3.ErrBusy || sqliteErr.Code == sqlite3.ErrLocked)
}

// waitForSQLiteBackupRetry 等待下一次锁冲突重试。它只限制锁等待时间，不限制一次合法的大库复制耗时。
func waitForSQLiteBackupRetry(ctx context.Context, attempt int) error {
	if attempt >= backupBusyRetryLimit {
		return fmt.Errorf("SQLite backup remained busy after %d retries", backupBusyRetryLimit)
	}
	select {
	case <-ctx.Done():
		return fmt.Errorf("wait for SQLite backup: %w", ctx.Err())
	case <-time.After(backupBusyRetryDelay):
		return nil
	}
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

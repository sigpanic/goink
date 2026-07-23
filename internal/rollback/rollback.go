package rollback

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/sigpanic/goink/internal/git"
	"github.com/sigpanic/goink/internal/storage"
)

// RollbackBeforeTurn 回退到 targetTurn 开始之前的状态。
// 撤销 [targetTurn, lastTurn] 区间所有变更：DB 元数据逆向 + git revert 章节文件。
//
// 三步策略（最小化不一致窗口）：
//  1. git revert --no-commit 暂存（可 revert 部分提前检测，不碰 DB）
//  2. DB 事务 commit（失败则 git revert --abort）
//  3. git commit（DB 已提交，这一步是纯 index→commit，极低失败概率）
func RollbackBeforeTurn(ctx context.Context, db *gorm.DB, sessionID string, targetTurn int,
	repo *git.Repo, tcStore *Store) error {

	// 1. 查询当前最新 turn
	var lastTurn int
	if err := db.WithContext(ctx).
		Raw("SELECT last_turn_id FROM sessions WHERE session_id = ?", sessionID).
		Scan(&lastTurn).Error; err != nil {
		return err
	}
	if lastTurn < targetTurn {
		return nil
	}

	// 2. 查询待 revert 的 git commit hash（时间正序，Revert 内部逆序处理）
	commits, err := tcStore.ListForRollback(ctx, sessionID, targetTurn, lastTurn)
	if err != nil {
		return err
	}
	hashes := make([]string, len(commits))
	for i, c := range commits {
		hashes[i] = c.CommitHash
	}

	// 3. git revert --no-commit（仅暂存，不提交）
	if len(hashes) > 0 {
		if err := repo.RevertNoCommit(hashes); err != nil {
			return err // RevertNoCommit 失败时已自动 abort
		}
	}

	// 4. DB 事务
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := storage.RollbackInTx(context.Background(), tx, sessionID, targetTurn, lastTurn); err != nil {
			return err
		}
		return cleanupTurnCommits(tx, sessionID, targetTurn, lastTurn)
	}); err != nil {
		// DB 失败 → 撤销 git 暂存，两边都回到初始状态
		if len(hashes) > 0 {
			_ = repo.RevertAbort()
		}
		return err
	}

	// 5. git commit（此时 DB 已提交，git 里只有已暂存的 revert 内容）
	if len(hashes) > 0 {
		msg := fmt.Sprintf("revert turn %d-%d\n\nSession: %s", targetTurn, lastTurn, sessionID)
		if _, err := repo.Commit(msg); err != nil {
			return fmt.Errorf("rollback: git commit after revert: %w (revert staged but not committed, run 'git commit' manually)", err)
		}
	}

	return nil
}

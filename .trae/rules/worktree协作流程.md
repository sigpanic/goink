# Worktree 双工作区协作流程

本项目使用 git worktree 实现两个 agent 并行开发。两个工作区共享同一份 `.git`，一方的 commit 对另一方立即可见，同步无需 fetch/push。

## 工作区布局

| 工作区 | 目录 | 分支 | 用途 |
|---|---|---|---|
| 主工作区 | `/home/nianhe/projects/todo` | `master` | agent 1 直接在 master 上开发（或临时切小开发分支） |
| 副工作区 | `/home/nianhe/projects/goink` | `dev`（从 master 切出） | agent 2 在该开发分支上开发 |

- 同一分支不能被两个 worktree 同时 checkout，因此两边分支必须不同。
- 两个工作区对全量文件都有读写权限，不冲突靠派活时挑选不相干模块保证，不靠 worktree 隔离。

## 同步机制（master → dev，不走 PR）

master 前进后（master 上有 dev 没有的新 commit），副工作区 dev 用 `git merge` 拉取 master 的最新内容（**不走 PR**，PR 只用于 dev → master 合并）：

```
cd /home/nianhe/projects/goink
git fetch origin
git merge origin/master              # ff 或产生 sync merge commit（git 自动选择）
git push origin dev                   # 无需 force
```

- `git merge origin/master` 的行为：
  - dev 是 master 祖先时（dev 无独有 commit）：自动 ff，不产生 merge commit，不重写 hash/date。
  - 双向分歧时（dev 有独有 commit + master 有新 commit）：产生 sync merge commit，不重写 dev 独有 commit 的 hash/date，dev 历史非线性。
- **PR 前建议先 sync**：在副工作区 dev 上先 `git merge origin/master` 解决冲突，再 push dev、走 PR，可以让 PR 看起来干净（无冲突），代价是 dev 上多一个 sync merge commit。
- 不要用 `git rebase origin/master`：会重写 dev 独有 commit 的 hash + committer date，破坏历史稳定性，还需要 force push。

## 合并回 master（dev → master，走 GitHub PR）

dev 干完后通过 GitHub PR 合并回 master，**产生 merge commit**（不再用本地 `git merge --ff-only`）：

```
# 副工作区推 dev
cd /home/nianhe/projects/goink
git push origin dev

# 主工作区建 PR 并合并（--merge 产生 merge commit）
cd /home/nianhe/projects/todo
gh pr create --base master --head dev --title "<英文标题>" --body "<英文描述>"
gh pr merge dev --merge               # 不删 dev 分支，dev 永久存在
```

- `--merge` 产生 merge commit，保留 dev 上的完整提交历史与合并节点。
- 合并后 master 前进，副工作区需要走“同步机制”用 `git merge origin/master` 拉取最新。
- 不要用 `--rebase` 或 `--squash`：前者会让 dev 历史在 master 上被重写失去合并节点；后者会丢失 dev 上每个 commit 的细粒度信息。

## 环境说明

- **Go 依赖**：模块缓存全局共享（`~/go/pkg/mod`），副工作区无需重装。
- **前端 node_modules**：各工作区独立，副工作区需在 `frontend/` 跑 `npm install`。
- **ONNX runtime**：副工作区 `build/runtime/` 下的 `.so` / `git/` / `.pc` 软链到主工作区共享；`models/vocab.txt` 是 git 跟踪的真实文件，勿动。`make clean` 只删软链不删目标，安全。
- **运行时数据 `~/Goink/`**：两个工作区跑应用时共享同一 SQLite DB 与 novel git 仓库。避免两个 agent 同时 `wails dev` 跑应用（DB 锁冲突）；仅编辑/构建/lint 不受影响。

## 红线（呼应《编码规则.md》Git 写操作禁令）

- 所有 `git rebase` / `git merge` / `git commit` / `git stash` / `git push` 等写操作，必须经用户明确授权，禁止擅自执行。
- 写完代码先发询问等用户 review，用户明确说 commit 才可提交。

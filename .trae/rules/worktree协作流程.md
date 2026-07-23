# Worktree 双工作区协作流程

本项目使用 git worktree 实现两个 agent 并行开发。两个工作区共享同一份 `.git`，一方的 commit 对另一方立即可见，同步无需 fetch/push。

## 工作区布局

| 工作区 | 目录 | 分支 | 用途 |
|---|---|---|---|
| 主工作区 | `/home/nianhe/projects/todo` | `master` | agent 1 直接在 master 上开发 |
| 副工作区 | `/home/nianhe/projects/goink` | `feat/goink-wt`（从 master 切出） | agent 2 在该 feature 分支上开发 |

- 同一分支不能被两个 worktree 同时 checkout，因此两边分支必须不同。
- 两个工作区对全量文件都有读写权限，不冲突靠派活时挑选不相干模块保证，不靠 worktree 隔离。

## 同步机制（master → feat）

master 前进后，feat 分支要跟上 master 的新 commit：

```
cd /home/nianhe/projects/goink
git rebase master
```

- rebase 把 feat 自己的 commit 重新摞到 master 最新之上，本质是“把 master 的 commit 拿过来”。
- rebase 会重写 feat 的 commit hash（feat 是本地未 push 分支，安全）。
- rebase 前确保 feat 工作区干净（已 commit 或已 stash），否则会中断报冲突。

## 合并回 master（feat → master）

feat 干完后并回 master，保持线性历史、不产生 merge commit：

```
cd /home/nianhe/projects/todo
git merge --ff-only feat/goink-wt
```

- 因 feat 已 rebase、严格领先 master，快进合并，无 merge commit。
- 若 master 又动了导致无法快进：先回 goink/ 再 `git rebase master` 一次，即可 ff-only。
- 若确实想留功能合并标记，可用 `git merge --no-ff feat/goink-wt` 强制产生 merge commit（需用户授权）。

## 环境说明

- **Go 依赖**：模块缓存全局共享（`~/go/pkg/mod`），副工作区无需重装。
- **前端 node_modules**：各工作区独立，副工作区需在 `frontend/` 跑 `npm install`。
- **ONNX runtime**：副工作区 `build/runtime/` 下的 `.so` / `git/` / `.pc` 软链到主工作区共享；`models/vocab.txt` 是 git 跟踪的真实文件，勿动。`make clean` 只删软链不删目标，安全。
- **运行时数据 `~/Goink/`**：两个工作区跑应用时共享同一 SQLite DB 与 novel git 仓库。避免两个 agent 同时 `wails dev` 跑应用（DB 锁冲突）；仅编辑/构建/lint 不受影响。

## 红线（呼应《编码规则.md》Git 写操作禁令）

- 所有 `git rebase` / `git merge` / `git commit` / `git stash` 等写操作，必须经用户明确授权，禁止擅自执行。
- 禁止 `git push`。
- 写完代码先发询问等用户 review，用户明确说 commit 才可提交。

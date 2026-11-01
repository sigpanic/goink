---
name: release
description: 发布新版本：手动建中文 release（gh 一步完成建 tag + release + 触发 CI）
---

# Release 发布流程

按以下步骤执行，每步完成后向用户报告结果。

## 1. 确认当前状态

- 确保工作区干净（`git status`）
- 确定发版分支：若有待合并的发布分支（head，由用户指定）领先 `master`，走第 2 步 PR 合并流程；若直接在 `master` 开发，跳到第 3 步
- 列出待发布提交：`git log <上一个 tag>..master --oneline`
- 向用户展示提交列表，确认是否全部应发布
- 确认版本号：参照上一个 tag 格式递增（例 `v1.1.1` → `v1.2.0`）
- 确认 `origin/master` 已包含所有待发布提交
- 编写 release notes 前，用 `git log <上一个 tag>..master --format=full` 读取每个提交的完整 body，并查看各提交 `Refs #NN` 指向的 issue（`gh issue view NN --comments`）作为上下文，据此区分「修复上个版本已存在的问题」与「版本内迭代」，不要只看 oneline 标题

## 2. 提 PR 并合并（仅当发布分支领先 master）

- 走 `pr` skill 的完整流程：读取 `git log origin/master..HEAD --format=full` 的全部 commit body，用**中文**标题和描述起草，给用户 review 通过后再创建
- 分支不对等：head 为发布分支（由用户指定），base 为 `master`，不要自行套用其他分支
- 创建：`gh pr create --base master --head <发布分支> --title "<中文标题>" --body "<中文描述>"`
- 合并：保留完整历史，**不 squash、不 rebase**，用 `gh pr merge <发布分支> --merge`，产生 merge commit
- 发布分支**不删除**
- 合并后 main 工作区 `git pull`，副工作区按 worktree 规则 `git merge origin/master` 同步

## 3. 本地打 tag 并推送

- 查看当前最新 tag：`git tag --sort=-v:refname | head -5`
- 打 annotated tag，push 到远端（push 同时触发 CI）
- tag message：英文，和发版说明一致即可
- 命令：
  ```
  git tag -a vX.Y.Z -m "<英文说明>"
  git push origin vX.Y.Z
  ```

## 4. 手动建 release

- 第 3 步已 push tag，gh 直接用它创建 release（**不要加 `--target`**，会在远端另建类型不同的 tag 对象，导致本地与远端不一致）
- release job 要等三个构建完成后才跑，手动 create 秒级完成，必然先存在；CI 只上传产物、不覆盖 body；若 CI 抢建了空 body，用 `gh release edit` 覆盖
- Release 信息**用中文手写**，不要用 `--generate-notes`
- 基于第 1 步读取的完整 commit body 与 issue 上下文撰写，不要只依赖 oneline
- 格式分块：「新增功能」「改进」「修复」等
- 命令：
  ```
  gh release create vX.Y.Z --title "vX.Y.Z" --notes "中文 release notes"
  ```

## 5. 校验

- CI 构建完成后，确认 release 已附加三平台安装包 + `sha256sums.txt`
- 确认 release body 仍为手写中文（CI 只上传产物，不覆盖 body）
- 确认 CI 运行状态：`gh run list --limit 3`（Triggered via push，job 为 build-linux / build-macos / build-windows）

## 6. 切回发布分支（若适用）

- 需要回到发布分支时执行 `git checkout <发布分支>`（切换分支属 git 写操作，先向用户确认）
- 若该分支曾领先，确认 `git log master..<发布分支> --oneline` 为空（已合并）

## 注意事项

- **Release notes**：仅手写中文。`release.yml` 的 release job 不再 `generate_release_notes`，只把产物上传到第 4 步已建的 release，不覆盖 body。
- **禁止 force push 已发布的 tag**：会重新触发 release workflow（匹配 `v*`），用不可复现的新构建覆盖现有 release 产物并重置下载计数。若确有必要动旧 tag，必须先临时禁用 Release workflow（GitHub → Settings → Actions），操作完再启用。
- **辨识版本内 fix**：区分「修复上个版本已存在的问题」（应写入 notes）与「开发过程中先 feat 后 fix 的新问题」（属本版本内部迭代，不算与上个版本的差异，不应写入 notes）。只记录面向用户的、相对上个版本的真实变化。务必结合 `Refs #NN` 的 issue 讨论确认每个 fix 到底修的是什么。
- **Commit 规范**：英文、具体描述、无 emoji、无 Co-Authored-By
- **PR 规范**：中文标题和描述，流程见 `pr` skill
- **Release notes 规范**：中文

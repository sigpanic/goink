---
name: release
description: 发布新版本：手动建中文 release（gh 一步完成建 tag + release + 触发 CI）
---

# Release 发布流程

按以下步骤执行，每步完成后向用户报告结果。

## 1. 确认当前状态

- 确保工作区干净（`git status`）
- 确定发版分支：若 `dev` 领先 `master`，走第 2 步 PR 合并流程；若直接在 `master` 开发，跳到第 3 步
- 列出待发布提交：`git log <上一个 tag>..master --oneline`
- 向用户展示提交列表，确认是否全部应发布
- 确认版本号：参照上一个 tag 格式递增（例 `v1.1.1` → `v1.2.0`）
- 确认 `origin/master` 已包含所有待发布提交
- 编写 release notes 前，用 `git log <上一个 tag>..master --format=full` 读取每个提交的完整 body，并查看各提交 `Refs #NN` 指向的 issue（`gh issue view NN --comments`）作为上下文，据此区分「修复上个版本已存在的问题」与「版本内迭代」，不要只看 oneline 标题

## 2. 提 PR 并合并（仅当 dev 领先 master）

- 标题：英文，简洁，60 字符以内
- 描述：英文，分 Summary 和 Test plan 两段
- **必须先用 `git log master..dev --format=full` 读取每个提交的完整 body**，基于所有详细的改动信息来撰写 PR 描述，不要只看 oneline 标题
- 合并：保留完整历史，**不 squash、不 rebase**，使用 `--no-ff`
- merge commit 信息：英文，格式 `merge: <简短主题>`，后接主要改动点列表（`- ` 开头）
- dev 分支**不删除**
- 命令示例：
  ```
  git checkout master
  git merge --no-ff dev -m "merge: <subject>

  - point one
  - point two"
  git push origin master
  ```

## 3. 本地打 tag

- 查看当前最新 tag：`git tag --sort=-v:refname | head -5`
- 本地打 tag 仅作本地记录，**不要 push**（见第 4 步）
- tag message：英文，和发版说明一致即可
- 命令：
  ```
  git tag -a vX.Y.Z -m "<英文说明>"
  ```

## 4. 手动建 release（gh ≥ 2.98，一步完成建 tag + release + 触发 CI）

- **gh ≥ 2.98 行为变化**：`gh release create vX.Y.Z` 对未推送的本地 tag 会直接报错
  `tag exists locally but has not been pushed ... specify the --target flag`，必须加 `--target <commit-sha>`
- `--target <commit-sha>` 会通过 GitHub API 同时：创建远程 tag（指向该 commit）→ 触发 CI（release.yml 检测到 tag）→ 创建 release。因此**无需再手动 push tag**，且 release body 就是手写中文，CI 只上传产物、不会抢建空 body
- commit-sha 取待发布分支的 HEAD（必须是已包含全部待发布提交的 commit）：`git rev-parse HEAD`
- Release 信息**用中文手写**，不要用 `--generate-notes`
- 基于第 1 步读取的完整 commit body 与 issue 上下文撰写，不要只依赖 oneline
- 格式分块：「新增功能」「改进」「修复」等
- 命令：
  ```
  gh release create vX.Y.Z --target <commit-sha> --title "vX.Y.Z" --notes "中文 release notes"
  ```

## 5. 校验

- CI 构建完成后，确认 release 已附加三平台安装包 + `sha256sums.txt`
- 确认 release body 仍为手写中文（CI 只上传产物，不覆盖 body）
- 确认 CI 运行状态：`gh run list --limit 3`（Triggered via push，job 为 build-linux / build-macos / build-windows）

## 6. 切回 dev（若适用）

- `git checkout dev`
- 若 dev 曾领先，确认 `git log master..dev --oneline` 为空（已合并）

## 注意事项

- **Release notes**：仅手写中文。`release.yml` 的 release job 不再 `generate_release_notes`，只把产物上传到第 4 步已建的 release，不覆盖 body。
- **辨识版本内 fix**：区分「修复上个版本已存在的问题」（应写入 notes）与「开发过程中先 feat 后 fix 的新问题」（属本版本内部迭代，不算与上个版本的差异，不应写入 notes）。只记录面向用户的、相对上个版本的真实变化。务必结合 `Refs #NN` 的 issue 讨论确认每个 fix 到底修的是什么。
- **本地 tag 与远程 tag**：本地 `git tag -a` 是 annotated tag 对象；gh `--target` 在远程创建的是 lightweight tag（指向同一 commit）。两者指向相同 commit 但对象不同，**不要再用 `git push origin vX.Y.Z` 推本地 tag**（远程 ref 已存在，push 会报 non-fast-forward）。
- **Commit 规范**：英文、具体描述、无 emoji、无 Co-Authored-By
- **PR 规范**：英文标题和描述
- **Release notes 规范**：中文

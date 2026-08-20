---
name: release
description: 发布新版本：本地打 tag，手动建中文 release，CI 构建并上传安装包
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
- 编写 release notes 前，用 `git log <上一个 tag>..master --format=full` 读取每个提交的完整 body，基于详细改动信息撰写，不要只看 oneline 标题

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
- tag message：英文，和发版说明一致即可
- 命令：
  ```
  git tag -a vX.Y.Z -m "<英文说明>"
  ```

## 4. 手动建 release（必须在第 5 步之前）

- **必须在推 tag 之前执行**，否则 CI 的 release job 会抢先创建一个空 body 的 release
- Release 信息**用中文手写**，不要用 `--generate-notes`
- 基于第 1 步读取的完整 commit body 撰写，不要只依赖 oneline
- 格式分块：「新增功能」「改进」「修复」等
- 命令：
  ```
  gh release create vX.Y.Z --title "vX.Y.Z" --notes "中文 release notes"
  ```

## 5. 推 tag 触发 CI

- 推送 tag 到 `origin`，CI（`release.yml`）检测到 tag push 后构建三平台安装包
- 命令：
  ```
  git push origin vX.Y.Z
  ```

## 6. 校验

- CI 构建完成后，确认 release 已附加三平台安装包 + `sha256sums.txt`
- 确认 release body 仍为手写中文（CI 只上传产物，不覆盖 body）

## 7. 切回 dev（若适用）

- `git checkout dev`
- 若 dev 曾领先，确认 `git log master..dev --oneline` 为空（已合并）

## 注意事项

- **Release notes**：仅手写中文。`release.yml` 的 release job 不再 `generate_release_notes`，只把产物上传到第 4 步已建的 release，不覆盖 body。
- **辨识版本内 fix**：区分「修复上个版本已存在的问题」（应写入 notes）与「开发过程中先 feat 后 fix 的新问题」（属本版本内部迭代，不算与上个版本的差异，不应写入 notes）。只记录面向用户的、相对上个版本的真实变化。
- **顺序关键**：第 4 步（手动建 release）必须在第 5 步（push tag 触发 CI）之前。
- **Commit 规范**：英文、具体描述、无 emoji、无 Co-Authored-By
- **PR 规范**：英文标题和描述
- **Release notes 规范**：中文

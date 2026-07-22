---
name: release
description: 发布新版本：检查 dev 分支待合并提交，提 PR（英文），合并到 master，打 tag，创建中文 release notes
---

# Release 发布流程

按以下步骤执行，每步完成后向用户报告结果。

## 1. 确认当前状态

- 确保当前在 dev 分支，工作区干净（`git status`）
- 列出 dev 领先 master 的提交：`git log master..dev --oneline`
- 向用户展示提交列表，确认是否全部应发布

## 2. 提 PR

- 标题：英文，简洁，60 字符以内
- 描述：英文，分 Summary 和 Test plan 两段
- **必须先用 `git log master..dev --format=full` 读取每个提交的完整 body**，基于所有详细的改动信息来撰写 PR 描述，不要只看 oneline 标题
- 不要加 Claude Code 相关说明

## 3. 合并 PR

- 保留完整历史：**不 squash、不 rebase**
- dev 分支**不删除**
- 使用 `--no-ff` 合并，写 merge commit 信息：
  - 英文，格式 `merge: <简短主题>`
  - 然后列出主要改动点（用 `- ` 列表）
- 命令示例：
  ```
  git checkout master
  git merge --no-ff dev -m "merge: <subject>
  
  - point one
  - point two"
  git push origin master
  ```

## 4. 打 tag 并推送

- 查看当前最新 tag：`git tag --sort=-v:refname | head -5`
- 确认版本号：参照上一个 tag 的格式（有无后缀），递增版本号。例：上一个 `v0.5.0` → 新 `v0.6.0`
- tag message：英文，和 merge commit 一致即可
- 命令：
  ```
  git tag -a vX.Y.Z -m "<英文说明>"
  git push origin vX.Y.Z
  ```

## 5. 创建 GitHub Release

- Release 信息**用中文**写，由操作者手写，不要用 `--generate-notes`
- 格式分两块：「新增功能」和「问题修复」
- CI workflow（`release.yml`）检测到 tag push 后会自动构建各平台安装包并附加到此 release
- 因为 release 由 CI 自动关联，手写 notes 不会被覆盖（已验证）
- 命令：
  ```
  gh release create vX.Y.Z -R sigpanic/goink --title "vX.Y.Z" --notes "中文 release notes"
  ```

## 6. 切回 dev

- `git checkout dev`
- 确认 `git log master..dev --oneline` 为空（已合并）

## 注意事项

- **Commit 规范**：英文、具体描述、无 emoji、无 Co-Authored-By
- **PR 规范**：英文标题和描述
- **Release notes 规范**：中文
- **GitHub Release**：手写中文 notes，不依赖 GitHub 自动生成

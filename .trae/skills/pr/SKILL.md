---
name: pr
description: 创建中文 PR，读取全部领先 commit 的完整 body 起草描述，分支由用户指定，确认后再用 gh 创建。用户要求提 PR、开 Pull Request 时使用，不用于发版或合并已有 PR。
---

# PR 创建流程

PR 标题与描述一律中文，commit message 保持英文不动。来源分支（head）和目标分支（base）由用户指定，不要预设 dev → master，也不要自行猜测。

按顺序执行，每步完成后向用户报告结果。任何 git 写操作（fetch / merge / push / gh pr create）执行前都要先说明具体命令。

## 1. 确认分支与工作区

- `pwd` + `git rev-parse --abbrev-ref HEAD`，确认在本次任务的工作区根目录
- 分支以用户指定为准：用户只给了一个分支时，确认它是 head 还是 base，另一个分支要问清，不要套用默认值
- head 与 base 相同则停止
- `git status`：有未提交改动就先停下，让用户决定是否先处理，不要自行 stash / commit
- `gh pr list --base <base> --head <head> --state open`：已存在打开的 PR 就报告并停止，不要重复创建

## 2. 同步远端（需授权）

- 先说明再执行 `git fetch origin`，避免用过期 ref 作为对比基线
- `git log --oneline HEAD..origin/<base>` 非空说明 head 分支落后，需要 `git merge origin/<base>`（冲突交给用户确认后再解决）
- 禁止 `git rebase`、禁止 force push

## 3. 读取全部领先提交

- `git log origin/<base>..HEAD --format=full`，逐个读取每个提交的完整 body，禁止只看 oneline 标题下结论
- `git diff --stat origin/<base>...HEAD` 辅助确认实际改动范围
- 每个 `Refs #NN` 都用 `gh issue view NN --comments` 读上下文，弄清这个提交到底修了什么
- 先向用户汇报：提交数量、每条 subject 与 body 要点，确认没有遗漏或被误判的提交

## 4. 起草 PR（中文）

标题：中文，具体，60 字以内；不使用 `fixes` / `closes` / `resolves` 等自动关闭关键词。

正文模板：

```
## 概述
（1-3 句：这个 PR 解决了什么问题，为什么现在做）

## 主要改动
- （按模块或主题归类，每条结论必须能追溯到某个 commit body 或 diff）

## 验证
- （实际跑过的 build / test / lint / 手工验证；没跑就如实写「未运行」，不要编造）

## 备注
- （已知限制、风险、有意保留的取舍、后续计划；没有可省略）

Refs #NN
```

写作要求：

- 把多个提交归纳成主题，不要逐条复述 commit 列表；但设计取舍、限制、风险要取自 commit body
- 区分「修复 base 分支上已存在的问题」与「head 分支开发过程中的内部迭代」，后者通常不该作为 PR 亮点
- 只写 commit body 与 diff 能支撑的事实，不臆造动机或验证结果

## 5. 等用户 review

- 在对话中贴出完整草稿（标题 + 正文 + 将要执行的分支与命令），然后停下等待明确指令
- 用户提出修改意见时，改完重新贴一遍再确认；「起草」「看看」「改一下」都不等于授权创建
- 只有用户明确说「创建 / 提交 / 可以提 PR」后才进入第 6 步

## 6. 创建 PR

- `git push origin <head>`（不加 `-f`；若远端分支被他人改写，停下来向用户说明）
- 用 heredoc 传正文，避免换行丢失：

  ```
  gh pr create --base <base> --head <head> --title "<中文标题>" --body "$(cat <<'EOF'
  <正文>
  EOF
  )"
  ```

- 除非用户要求，不要加 `--draft`
- 创建完成后报告 PR URL，并说明尚未合并

## 禁止事项

- 不 squash、不 rebase、不 force push，不删除分支
- 不擅自 `gh pr merge`；用户要求合并时按 `.trae/rules/worktree协作流程.md` 用 `gh pr merge <head> --merge`，并在合并前再次确认
- 不擅自发布 PR 评论、review 或修改 PR 内容
- 为写 PR 描述不修改任何代码

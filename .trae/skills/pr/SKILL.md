---
name: pr
description: 创建 PR，读取全部领先 commit 的完整 body 起草英文标题与中文描述，分支由用户指定，确认后再用 gh 创建。用户要求提 PR、开 Pull Request 时使用，不用于发版或合并已有 PR。
---

# PR 创建流程

PR 标题用英文，描述用中文；commit message 保持英文不动。来源分支（head）和目标分支（base）由用户指定，不要预设 dev → master，也不要自行猜测。

按顺序执行，每步完成后向用户报告结果。任何 git 写操作（fetch / merge / push / gh pr create）执行前都要先说明具体命令。

## 1. 确认分支与工作区

- `pwd` + `git rev-parse --abbrev-ref HEAD`，确认在本次任务的工作区根目录
- 分支以用户指定为准：用户只给了一个分支时，确认它是 head 还是 base，另一个分支要问清，不要套用默认值
- head 与 base 相同则停止
- `git status`：有未提交改动就先停下，让用户决定是否先处理，不要自行 stash / commit
- `gh pr list --base <base> --head <head> --state open`：已存在打开的 PR 就报告并停止，不要重复创建

## 2. 同步远端（命令交给用户执行）

- 用只读方式确认基线：`git ls-remote origin refs/heads/<base> refs/heads/<head>`，必要时再说明后执行 `git fetch origin`
- `git log --oneline HEAD..origin/<base>` 非空说明 head 落后 base。此时**不要自行执行 `git merge`**，把具体命令交给用户自行执行：

  ```
  cd <当前工作区根目录> && git fetch origin && git merge origin/<base>
  ```

- 把落后情况（领先/落后提交数、是否会因三点 diff 出现无关删除行）在**会话中**告知用户，等待用户确认已合并完成后再继续
- 禁止 `git rebase`、禁止 force push

## 3. 读取全部领先提交

- `git log origin/<base>..HEAD --format=full`，逐个读取每个提交的完整 body，禁止只看 oneline 标题下结论
- `git diff --stat origin/<base>...HEAD` 辅助确认实际改动范围
- 每个 `Refs #NN` 都用 `gh issue view NN --comments` 读上下文，弄清这个提交到底修了什么
- 先向用户汇报：提交数量、每条 subject 与 body 要点，确认没有遗漏或被误判的提交

## 4. 起草 PR（英文标题 + 中文正文）

标题：英文，遵循 pre-commit hook 校验的 Conventional Commits 规范，格式 `<type>(<scope>): <description>`。type 取 `feat` `fix` `docs` `style` `refactor` `perf` `test` `build` `ci` `chore` `revert` 之一（小写）；scope 可选，写主要影响模块；description 小写开头、不加句号、控制在 72 字符以内，不使用 `fixes` / `closes` / `resolves` 等自动关闭关键词。

正文：中文，按下方模板组织。

```
## 概述
（1-3 句：这个 PR 解决了什么问题，为什么现在做）

## 主要改动
- （按模块或主题归类，每条结论必须能追溯到某个 commit body 或 diff）

## 验证
- （项目层面真实的验证结论，例如「新增的 mock 测试覆盖了 X」「CI 已通过」；没有任何可写的事实时整节省略，不要编造）

## 备注
- （合并后仍然存在的限制、风险、有意保留的取舍、后续计划；没有可省略）

Refs #NN
```

写作要求：

- PR 正文是**面向公众的项目文档**，只客观描述这个 PR 改了什么、为什么改，不写会话过程、工作汇报或与用户沟通的记录
- 把多个提交归纳成主题，不要逐条复述 commit 列表；但设计取舍、限制、风险要取自 commit body
- 区分「修复 base 分支上已存在的问题」与「head 分支开发过程中的内部迭代」，后者通常不该作为 PR 亮点；已被本 PR 后续提交修好的中间态也不要写进正文
- 只写 commit body 与 diff 能支撑的事实，不臆造动机或验证结果
- 严禁出现的表述（这些问题只在会话里告诉用户，绝不写进 PR）：
  - 起草过程与工具状态：「本次起草阶段未重新运行 build/test/lint」「起草时已指出」「已在实际起草时说明」
  - 分支同步状况：「head 落后 base N 个提交」「合并前需先同步 master」「三点 diff 会出现无关删除行」
  - 对用户的工作交代：「建议先…」「需要你确认…」「后续待办」等面向个人的措辞

## 5. 等用户 review

- 在对话中贴出完整草稿（标题 + 正文 + 将要执行的分支与命令），然后停下等待明确指令
- 用户提出修改意见时，改完重新贴一遍再确认；「起草」「看看」「改一下」都不等于授权创建
- 只有用户明确说「创建 / 提交 / 可以提 PR」后才进入第 6 步

## 6. 创建 PR

- `git push origin <head>`（不加 `-f`；若远端分支被他人改写，停下来向用户说明）
- 用 heredoc 传正文，避免换行丢失：

  ```
  gh pr create --base <base> --head <head> --title "<英文标题>" --body "$(cat <<'EOF'
  <正文>
  EOF
  )"
  ```

- 除非用户要求，不要加 `--draft`
- 创建完成后报告 PR URL，并说明尚未合并

## 禁止事项

- 不 squash、不 rebase、不 force push，不删除分支；不自行执行 `git merge origin/<base>`，同步命令交给用户
- 不把会话过程、起草状态、分支同步情况或对用户的工作交代写进 PR 标题与正文
- 不擅自 `gh pr merge`；用户要求合并时按 `.trae/rules/worktree协作流程.md` 用 `gh pr merge <head> --merge`，并在合并前再次确认
- 不擅自发布 PR 评论、review 或修改 PR 内容
- 为写 PR 描述不修改任何代码

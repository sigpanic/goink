# Git 包设计文档

## 概述

每部小说一个 Git 仓库，章节正文以 Markdown 文件存储在 `chapters/` 目录下，故事状态文档 `goink.md` 放在小说根目录。调用系统 Git CLI 执行所有版本控制操作，文件 I/O 走标准库 `os` 包。

正文的版本历史、差异对比、回退走标准 Git 操作。章节元数据（标题、编号、字数等）存 SQLite，不在此包职责范围。

## Git 部署策略

| 平台 | 方案 |
|------|------|
| **Windows** | 便携版 MinGit（~48MB），解压到应用目录 `{app}/bin/git.exe`，静默下载安装，不写注册表、不加 PATH |
| **macOS** | 系统自带 git（Xcode CLI），`gitBin` 为空从 PATH 查找 |
| **Linux** | 系统自带 git（各发行版默认安装），同上 |

启动时检测路径 `/app/bin/git` 是否存在 → 不存在则 Windows 自动下载 MinGit zip 解压，Linux/macOS 提示用户安装系统包。国内下载走 npmmirror.com 镜像。`New(novelDir, gitBin)` 接收 git 路径，为空时用 `"git"` 从 PATH 查找。

## 文件结构

小说仓库位置由调用方传入目录路径，Git 包不关心具体规则。约定：

```
{config.DataDir}/novels/{novel_id}/
    goink.md              ← 故事状态文档（自由文本）
    chapters/
        001.md
        002.md
        ...
```

路径规则统一由 config/调用方管理，New(novelDir) 只接收完整路径。

大纲（结构化场景列表）存 SQLite，不在 Git 管理范围。用户不能随意编辑大纲文件破坏结构。

## Commit 模型

每个 turn 最多两个 commit：

```
Turn 开始
  │  HasUncommitted()? → git add -A → commit
  │  消息格式: "turn N: user manual changes\n\nSession: {session_id}"
  │  语义：用户在对话间隙手动编辑了文件
  │
  ├─ AI 提出编辑 → DiffContent 预览（不写文件）→ 用户审批
  │     ├─ 通过 → WriteChapter（写文件）
  │     └─ 拒绝 → 不写文件，对话中断
  │
  ├─ AI 继续编辑 → 重复审批...
  │
Turn 结束 → git add -A → commit
  消息格式: "turn N: AI changes\n\nSession: {session_id}\n\nCo-authored-by: {model_name}"
  始终执行，无论正常结束、用户打断、异常关闭
```

### 关键规则

- **用户手动修改只在 turn 开始时 commit**，语义是"用户在对话间隙改的"
- **Turn 结束始终 commit**，无论正常结束、打断还是异常关闭，涵盖该 turn 所有文件变更
- **审批时只做 Diff 预览，不写文件**。拒绝不产生任何文件写入，不存在脏工作区
- **AI 编辑流程**：edit_chapter MCP 工具在内存中完成 search/replace → 调用 DiffContent 生成预览 → 通过后 WriteChapter → 拒绝则不写
- **commit 消息携带 Session ID 和模型名称**（session 在前，Co-authored-by 在最后）：
  ```
  turn 3: AI changes

  Session: sess_abc123

  Co-authored-by: DeepSeek V4
  ```

> **已知限制**：`DiffContent` 对现有文件输出 `a/path b/path`（相同路径），与 `git diff` CLI 一致。Monaco Editor diff mode 等主流组件正常渲染，但如果前端自行 parse 路径做前后对比需注意。

## 自动化模式

调用方可配置自动审批模式，跳过 Diff 预览和用户确认步骤，AI 编辑直接写文件。Git 包不感知此模式——由 edit_chapter MCP 工具和 agent loop 控制：

- 手动模式：DiffContent → 等用户审批 → WriteChapter 或放弃
- 自动模式：直接 WriteChapter，不调 DiffContent

两种模式都走内存 search/replace + 全量覆写 WriteChapter。章节文件天然小（几千字），不需要文件级增量替换，也不依赖流式处理。

## 前端 Diff 可视化

DiffContent 输出 unified diff 格式（标准 `git diff` 输出），前端用 Monaco Editor diff mode 或 react-diff-viewer 组件渲染，左右对照高亮。

## Turn 回退

```
git revert <turn-commit-hash>
```

- 回退粒度 = 一个 turn，和对话一一对应
- 多 session 交错时只回退指定 session 的指定 turn，其他 session 不受影响
- 有冲突时交给用户手动处理——这正是正确的行为
- `Commit` 返回 commit hash，agent loop 在 session 元数据中记录 `turn → hash` 映射，回退时直接按 hash 定位，不依赖 commit 消息解析
- `Revert` 使用 `--no-commit` 逐个暂存，全部成功后统一 commit，保证原子性。任何一步冲突则 `--abort` 回滚所有暂存的 revert

## API

```go
package git

// New 打开已有仓库，不存在则 git init + 首次 commit。
// gitBin 为 git 可执行文件路径（便携版部署路径），为空时从 PATH 查找。
func New(novelDir, gitBin string) (*Repo, error)

// ── 文件读写 ──

func ChapterPath(id int64) string                           // "chapters/id_1.md"
func ReadFile(novelID int64, path string) (string, error)   // 读全文
func WriteFile(novelID int64, path, content string) error   // 全量覆写，章节内容天然小（几千字），无需增量写入

func (r *Repo) GoinkPath() string
func (r *Repo) ReadGoink() (string, error)
func (r *Repo) WriteGoink(content string) error

// ── Diff ──

// DiffContent 在内存中计算当前文件内容与 proposed 的 unified diff，不写文件。
// 审批预览用：edit_chapter 工具内存中做完 search/replace 后，调用此方法生成 diff 给用户看。
func (r *Repo) DiffContent(path string, proposed string) (string, error)

// ── Git 操作 ──

func (r *Repo) StageAll() error                            // git add -A
func (r *Repo) Commit(msg string) (string, error)           // git commit，返回 commit hash
func (r *Repo) HasUncommitted() (bool, error)              // working tree 是否有未提交变更
func (r *Repo) Revert(hashes []string) error               // git revert，逆序回退
func (r *Repo) Log(path string, n int) ([]CommitInfo, error)
func (r *Repo) LogDetailed(n int, afterHash string) ([]CommitInfo, error) // 带文件统计，降序返回，afterHash 非空时游标翻页
func (r *Repo) CommitFileList(hash string) (*CommitInfo, []FileEntry, error) // commit 文件列表（不含内容），懒加载第一步
func (r *Repo) ShowFile(hash, filePath string) (*FileDiff, error)            // 单个文件前后内容，懒加载第二步

type CommitInfo struct {
    Hash         string
    ShortHash    string
    Message      string
    Time         time.Time
    AuthorName   string
    AuthorEmail  string
    FilesChanged int
    Insertions   int
    Deletions    int
}

type FileDiff struct {
    Path            string
    ChangeType      string    // "added" | "modified" | "deleted"
    OriginalContent string
    ModifiedContent string
}

type FileEntry struct {
    Path       string
    ChangeType string    // "added" | "modified" | "deleted"
}
```

### 与 edit_chapter MCP 工具的关系

edit_chapter 是 LLM 编辑章节正文的工具，Git 包提供底层文件 I/O 和 Diff 能力：

```
edit_chapter 工具:
  1. ReadChapter(num)              → 取当前文件内容
  2. 内存中执行 search_replace     → 得到 proposed 内容
  3. DiffContent(path, proposed)   → 生成 diff 预览
  4. 等用户审批（自动模式跳过）
  5. 通过 → WriteChapter(num, proposed)
     拒绝 → 不写文件
```

Git 包不实现 search/replace 逻辑，不感知审批流。

## 调用方

| 调用方 | 操作 |
|--------|------|
| agent loop（turn 管理） | HasUncommitted, StageAll, Commit, Revert |
| MCP edit_chapter 工具 | ReadChapter, WriteChapter, DiffContent |
| MCP get_chapter_content 工具 | ReadChapter |
| MCP 故事状态工具 | ReadGoink, WriteGoink, DiffContent |
| 审批流 | DiffContent |
| 前端（用户手动编辑） | 编辑器 autosave 到文件（不经过此包），turn 开始时由 agent loop 检测并 commit |

## 不做

- 分支：单用户应用不需要
- git push/pull/remote：纯本地
- 章节元数据管理：SQLite chapter 表负责
- search/replace 逻辑：edit_chapter MCP 工具负责，Git 包只提供文件读写和 Diff

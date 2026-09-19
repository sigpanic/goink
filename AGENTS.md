# Goink 项目协作与编码规则

Goink 是一个使用 Wails（Go + React）的桌面 AI 网文写作助手。用户主要使用中文沟通；回复用户时使用中文。

## 工作区与 Git

- 只在当前启动的 worktree 中读取、编辑和运行命令。不要进入另一个 worktree 操作文件。
- 当前仓库使用两个 worktree：`/home/nianhe/projects/todo`（`master`）和 `/home/nianhe/projects/goink`（`dev`）。以当前工作目录为准，不要硬编码切换到另一个目录。
- Git 只允许执行只读操作：`status`、`diff`、`log`、`show`、`branch`、`tag`、`ls-files`、`blame`、`grep`、`rev-parse`、`rev-list`、`stash list` 等。
- 未经用户明确许可，不要执行任何 Git 写操作，包括 `commit`、`push`、`pull`、`fetch`、`merge`、`rebase`、`reset`、`revert`、`checkout`、`stash`、`clean`、创建/删除分支或 tag。
- 修改完成后不要自动 commit 或 push，也不要主动询问是否 commit；等待用户明确指示。
- Commit message 使用英文、具体描述、无 emoji、无 `Co-Authored-By`。Issue 引用使用 body 末尾的 `Refs #NN`，不要使用 `fixes`、`closes` 或 `resolves`，除非用户明确要求关闭 issue。

## 构建与验证

- Go 命令从仓库根目录执行：`go build ./...`、`go test ./...`。
- 前端命令必须先进入 `frontend/`：使用 `npm run build`、`npm run lint`、`npm run test`；不要在项目根目录运行 `npm install`。
- 安装前端依赖时，在 `frontend/` 中执行 `npm install <pkg> --save`。
- `.githooks/pre-commit` 会按 staged 文件范围执行验证：Go 变更触发 build/test/golangci-lint，前端变更触发 build/lint/test，文档和配置变更跳过。除非用户要求或需要诊断，通常不重复运行整套验证。
- 系统依赖（Ubuntu/Debian）：`libsqlite3-dev libgtk-3-dev libwebkit2gtk-4.1-dev gcc`。

## 代码库结构与约定

- `app/`：Wails binding 层；导出方法构成前端 API。
- `internal/agent/`、`agentcfg/`、`llm/`、`session/`：LLM 对话、系统提示、传输和会话。
- `internal/mcp_tools/`：MCP 工具；该目录的专门规则见 `internal/mcp_tools/AGENTS.md`。
- `frontend/`：React 19 + TypeScript + Tailwind 4 + shadcn/ui。
- 数据目录为 Linux/macOS 的 `~/Goink/`，Windows 为可执行文件附近；包含 `models/`、`runtime/`、数据库和小说仓库。
- 每个小说是独立 Git 仓库，章节位于 `chapters/NNN.md`，大纲位于 `outlines/NNN.md`。
- ONNX 和 sqlite-vec 代码使用 `//go:build cgo`；Windows 诊断时 cgo 相关编译失败是已知限制。
- 所有 ONNX、VectorStore、RefreshQueue 均按现有代码的全局 singleton 约定维护，不要随意引入第二个实例。
- `target_chapter` 只用于排序，不要把它当作精确过滤条件。
- 消息是 append-only，并区分 API、前端和完整审计查询路径。

## 修改安全边界

- 不要删除或修改 logger 语句、代码注释，除非用户明确要求。
- 不要手动编辑 Wails 自动生成的绑定文件（例如 `frontend/src/lib/wailsjs/go/models.ts`、`App.d.ts`）。需要通过 `wails generate module` 或 `make build` 重新生成。
- 使用编辑工具修改代码；不要用 `sed` 或 Python 脚本直接写代码文件。
- 第一次替换文本时使用从文件中精确复制的原文；如果匹配失败，再检查格式、缩进或扩大锚点，不要先格式化整个文件。
- 同一文件的多次编辑必须顺序执行，不能并行，避免后一次写入覆盖前一次修改。
- 除非用户明确要求，不主动扩大任务范围，不自动生成提交、发布或远程操作。

## 领域细节

- 角色关系是 append-only，使用 `is_current` 表示当前关系；更新会新增记录并保留历史。
- 偏好分为全局和单小说偏好，分类由 LLM 处理。
- ONNX embedder、VectorStore 和 RefreshQueue 是全局 singleton。
- ONNX 库查找顺序、模型目录和向量表命名沿用现有实现，不要在单个功能中重新定义路径规则。

## 代码审查重点

- 检查是否误改自动生成文件、日志/注释、数据 append-only 语义或 cgo build tag。
- 检查 Git 写操作是否得到用户明确授权。
- 检查 Go 与前端命令是否在正确目录执行，以及变更是否需要对应测试。

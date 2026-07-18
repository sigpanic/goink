# v1.2.0 — Skill 市场与仓库 URL 同步

## 背景与目标

v1.1.x 期间 **skills 仓库独立**为 `sigpanic/goink-skills`（main 分支）。
独立后主应用仓库内的 URL 引用未跟上，且应用内完全没有消费 `goink-skills/index.json` 的能力——
社区贡献的 skill 用户无法在应用内浏览/安装。

本次 v1.2.0 要做两件事：

1. **同步 URL**：修复主应用内残留的错误 skills 仓库引用（指向 `sigpanic/goink` 而非 `sigpanic/goink-skills`）。
2. **新增 Skill 市场**：通过 GitHub API 拉取 `goink-skills` 仓库的 `index.json` 列出所有社区 skill，
   用户可选择具体 skill 下载到本地（user 层或 novel 层），网络失败时给出明确的可操作反馈。

## 调研发现

### URL 引用不一致

| 文件 | 行 | 当前值 | 问题 |
|---|---|---|---|
| `frontend/src/components/skill/SkillContributeDialog.tsx` | 5 | `const REPO = 'sigpanic/goink'` | 应为 `sigpanic/goink-skills` |
| `frontend/src/components/skill/SkillContributeDialog.tsx` | 6 | `const BRANCH = 'master'` | 应为 `main` |

- 上述两处导致贡献按钮的 fork / template / guide 三个链接全部 404。
- 其余位置（`README.md`、`README_EN.md`、`CONTRIBUTING.md`）已正确指向 `sigpanic/goink-skills`。
- 主仓库 URL（`GitHubLink.tsx`、`internal/update/checker.go`）保持 `sigpanic/goink` 不变。

### 没有通用 GitHub API 客户端

全仓只有 `internal/update/checker.go` 一处裸 `http.Client` 调用 `api.github.com/repos/sigpanic/goink/releases/latest`，
无封装、无重试、无 rate limit 解析。本次需要新建通用客户端。

### 没有市场类 UI 组件

无 `marketplace` / `store` / `hub` 命名组件。可复用：
- `SkillList.tsx` — 现有 skill 列表样式
- `SkillPreview.tsx` — skill markdown 渲染
- `UpdateDialog.tsx` — 版本对比 UI 思路
- `SkillContributeDialog.tsx` — Dialog 弹窗模式

### `goink-skills/index.json` 当前为空

`goink-skills/skills/` 目录还没有任何社区 skill，但应用应提前支持消费 `index.json`，
等社区 PR 合并后立即可用。

---

## 决策记录

| 决策点 | 选择 | 说明 |
|---|---|---|
| 安装层级 | 用户选择 | `InstallRemoteSkill` 接受 `target` 参数（`user` / `novel`），前端安装时弹窗让用户选；novel 层需传 `novel_id` |
| 覆盖策略 | 弹确认框覆盖 | 后端不做存在性判断，前端调用前弹确认框，显示本地/远程 version 对比 +「是否覆盖」 |
| 更新检查 | 仅市场内对比 | 不做后台轮询，只在市场打开时拉一次 `index.json` 与本地 version 对比，显示「已安装 / 可更新 v{remote}」 |
| 推进方式 | 分步提交 | 严格按 6 步顺序，每步写完发询问等 review，用户说 commit 才提交 |
| GitHub API 客户端 | 抽取通用层 | 新建 `internal/githubapi/client.go`，`update/checker.go` 本次不动，`skill/remote` 基于新 client |

### 关于 403 vs 429 的说明

标准 HTTP 语义上 `429` 才是 rate limit，`403` 是 forbidden。但 **GitHub API 是特例**：
rate limit 实际返回 `403` + body 含 `"message": "API rate limit exceeded"` + header `X-RateLimit-Remaining: 0`。

因此错误分类需要区分两种 403：
- `403` + header `X-RateLimit-Remaining: 0` → rate limit
- `403` + 其他 → 真 forbidden（仓库私有等）

真正的 `429` 在 GitHub API 上几乎不出现，但为健壮性也按 rate limit 处理。

---

## 方案设计

### 网络端点选择

用 `api.github.com` 而非 `raw.githubusercontent.com`：
- `api.github.com` 国内基本可直连（区别于 `github.com`）
- 支持 `Accept: application/vnd.github.raw+json` header 直接返回原始内容
- response header 提供 rate limit 信息（`X-RateLimit-Remaining`、`X-RateLimit-Reset`）

| 操作 | 端点 |
|---|---|
| 列出所有 skill | `GET https://api.github.com/repos/sigpanic/goink-skills/contents/index.json` |
| 获取单个 skill 全文 | `GET https://api.github.com/repos/sigpanic/goink-skills/contents/skills/{name}.md` |

### 第一部分：URL 修复

仅修 `frontend/src/components/skill/SkillContributeDialog.tsx` 第 5-6 行两处常量。
**不**做集中管理 URL 的 refactor（避免扩大改动面，未来可单独做）。

### 第二部分：通用 GitHub API 客户端

新建 `internal/githubapi/client.go`：

- `Client` struct：持有 `*http.Client`（5s 超时）、`User-Agent: Goink` header
- `GetRawContent(ctx, owner, repo, branch, path) ([]byte, *RateLimit, error)` — 通用方法
  - 请求时带 `Accept: application/vnd.github.raw+json` header 直接拿原始内容
  - 解析 response header 的 `X-RateLimit-Remaining` / `X-RateLimit-Reset`
- 统一错误类型 `GitHubAPIError`：
  - `RateLimited` — 403 + `X-RateLimit-Remaining: 0`，或 429
  - `NotFound` — 404
  - `Forbidden` — 403 非 rate limit
  - `Network` — 超时、连接拒绝、DNS 失败
  - `Other` — 其他 4xx / 5xx
- 匿名调用即可（受 60 req/h 限制），暂不支持 token

单测用 `httptest` mock GitHub API，覆盖 200 / 304 / 403-rate-limit / 403-forbidden / 404 / 429 / 5xx / 超时。

**`internal/update/checker.go` 本次不动**，等市场功能稳定后单独 refactor 迁移。

### 第三部分：Skill 远程模块

新建 `internal/skill/remote/` 包，基于 `githubapi.Client` 实现：

- `ListRemoteSkills(ctx) ([]RemoteSkillMeta, error)`
  - 调 `GetRawContent(ctx, "sigpanic", "goink-skills", "main", "index.json")`
  - 反序列化 JSON（结构对齐 `goink-skills/scripts/generate-index.py` 的输出）
  - 本地缓存到 `~/.goink/skill-market-cache.json`（含 `updated` 时间戳），用户点"刷新"才重新拉取
- `GetRemoteSkillContent(ctx, name) (string, error)`
  - 调 `GetRawContent(ctx, "sigpanic", "goink-skills", "main", "skills/{name}.md")`
- `InstallRemoteSkill(ctx, name, target, novelID) error`
  - 内部调 `GetRemoteSkillContent`
  - `target=user` → 写入 `~/.goink/skills/{name}.md`，触发 `skill.Store.ReloadUser()`
  - `target=novel` → 写入 `{novel_dir}/skills/{name}.md`，触发 `skill.Store.ReloadNovel(novelID)`
  - 后端不做存在性判断（前端弹确认框处理覆盖）

`RemoteSkillMeta` 字段对齐 `goink-skills/scripts/generate-index.py` 输出（与本地 `skill.SkillMeta` 基本一致：
`name` / `description` / `category` / `mode` / `author` / `version`）。

单测用 `httptest` mock，覆盖正常路径 + 各类错误传播。

### 第四部分：Wails 应用层

在 `app/skill_api.go` 新增三个方法（与现有 `ListSkills` / `DeleteSkill` 并列）：

| 方法 | 入参 | 返回 |
|---|---|---|
| `ListRemoteSkills()` | 无 | `[]RemoteSkillMeta, error` |
| `GetRemoteSkillContent(name)` | `string` | `string, error` |
| `InstallRemoteSkill(name, target, novelID)` | `string, string, string` | `error` |

改完后跑 `wails generate module` 重新生成 `App.d.ts` 和 `models.ts`
（**不手改**自动生成文件，符合 `.trae/rules/编码规则.md`）。

### 第五部分：前端 UI

新建 `frontend/src/components/skill/SkillMarketplace.tsx`：

- **顶部工具栏**：搜索框 + 分类筛选（复用现有 6 大分类）+ "刷新"按钮
- **列表区**：远程 skill 卡片
  - 显示 `name` / `description` / `category` / `author` / `version`
  - 状态标识：
    - 未安装 → 「安装」按钮
    - 已安装（本地 version ≥ 远程）→ 「已安装 v{local}」灰色标签
    - 可更新（本地 version < 远程）→ 「更新 v{remote}」按钮
- **详情面板**：点击卡片展开右侧，复用 `SkillPreview.tsx` 渲染 markdown
  - 调 `GetRemoteSkillContent(name)` 拉取全文
- **安装弹窗**（用户选择层级）：
  - 标题：「安装 skill `{name}`」
  - 单选：`user 层（所有小说共享，推荐）` / `novel 层（仅当前小说）`
  - 如果检测到本地已存在，改为「覆盖安装」确认框，显示本地/远程 version 对比
- **网络失败态**（核心需求）：
  - 超时 / 连接失败 → 红色提示条：
    > **无法连接 GitHub API**
    > `api.github.com` 国内通常可直连（区别于 `github.com`）。如长期失败，请尝试：
    > 1. 检查网络代理设置
    > 2. 手动访问 https://github.com/sigpanic/goink-skills 浏览社区 skill
    > 3. 在可访问 GitHub 的网络环境中使用此功能
    >
    > [重试]
  - 403 rate limit → 「GitHub API 请求频率超限（匿名 60 次/小时），重置时间：{X-RateLimit-Reset 格式化}，请稍后再试」
  - 403 forbidden → 「访问被拒绝，可能是仓库权限问题」
  - 404 单个 skill → 「该 skill 可能已被维护者移除，请刷新列表」
  - 其他 → 「获取 skill 失败：{具体错误}」+ [重试]

**入口位置**：在 `SkillList.tsx` 顶部工具栏加一个"浏览市场"按钮，
点击打开 `SkillMarketplace` Dialog（与现有 `SkillContributeDialog` 弹窗模式一致，不新增侧栏 tab）。

### 第六部分：i18n 文案

在 `frontend/src/i18n/locales/zh-CN.json` 和 `en.json` 的 `skill.*` 命名空间下新增 `marketplace.*` 子键：
- 按钮文案（浏览市场 / 安装 / 更新 / 刷新 / 重试）
- 状态标签（已安装 / 可更新 / 加载中）
- 安装弹窗（标题 / 层级选择 / 覆盖确认）
- 错误提示（各类网络失败 + rate limit + 404）

遵循项目记忆约束：
- 英文需要复数化的 key 拆 `_one` / `_other`
- 中文不拆

---

## 实施顺序（分步提交）

每步写完发询问等 review，用户说 commit 才提交，再进下一步。

| 步骤 | 内容 | 类型 |
|---|---|---|
| 1 | 修 `SkillContributeDialog.tsx` 的 URL bug（REPO + BRANCH 两处常量） | fix |
| 2 | 新建 `internal/githubapi/client.go` 通用客户端 + 单测 | feat |
| 3 | 新建 `internal/skill/remote` 包（基于 `githubapi.Client`）+ 单测 | feat |
| 4 | 新增 `app/skill_api.go` 三个方法 + `wails generate module` 重生成绑定 | feat |
| 5 | 新增前端 `SkillMarketplace.tsx` + 入口按钮 + 安装弹窗 + 覆盖确认 + 网络失败态 UI | feat |
| 6 | i18n 文案补充 + 全量验证（`go build` / `go test` / `npm run build` / `eslint`） | feat + test |

---

## 验证清单

每步完成后需通过：

- `go build ./...`
- `go test ./...`
- `cd frontend && npm run build`
- `cd frontend && npm run lint`（或 eslint）

最终步骤额外验证：
- 网络正常路径：能列出 `goink-skills` 仓库的 skill，能安装到 user / novel 层
- 网络失败路径：断网时显示明确提示 + 重试按钮
- rate limit 路径：mock 403 + `X-RateLimit-Remaining: 0`，显示重置时间
- 覆盖路径：已存在 skill 时弹确认框，显示 version 对比

---

## 风险与注意事项

1. **`App.d.ts` / `models.ts` 禁止手改**：第 4 步必须跑 `wails generate module` 重新生成。
2. **rate limit**：匿名 60 req/h。通过本地缓存 `index.json` + 用户主动刷新避免频繁请求。
3. **`update/checker.go` 不动**：避免扩大改动面，本次只新增 `githubapi` 包，不动现有调用方。
4. **`goink-skills/index.json` 当前为空**：第 5 步 UI 需要处理"远程列表为空"的状态，
   显示「社区还没有 skill，欢迎贡献」+ 贡献入口链接。
5. **网络失败提示文案**：必须明确告知用户 `api.github.com` 国内通常可直连，并提供手动访问仓库的备选方案。
6. **i18n 一致性**：en.json 和 zh-CN.json 的 key 结构遵循项目记忆里的 5 条语义规则（不是简单 diff）。

---

## 不做的事

- 不做集中管理 URL 的 refactor（本次只修 bug URL）
- 不动 `internal/update/checker.go`（等市场稳定后单独 refactor）
- 不做后台启动时检查更新（仅市场内对比）
- 不做仓库内嵌 webview 浏览 PR（外链 `BrowserOpenURL` 即可）
- 不做 skill 评分 / 评论 / 下载量统计（社区规模还小，过度设计）

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
| 错误码组织 | 模块前缀 + const 分区 | 错误码集中定义在 apperr，按模块分 const 区，switch 拆函数。同样 404 不同模块不同错误码（githubapi.not_found ≠ llm.not_found），前端反馈可精确区分 |

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

- `ListRemoteSkills(ctx, forceRefresh) ([]RemoteSkillMeta, error)`
  - 调 `GetRawContent(ctx, "sigpanic", "goink-skills", "main", "index.json")`
  - 反序列化 JSON（结构对齐 `goink-skills/scripts/generate-index.py` 的输出）
  - **内存缓存**（详见「缓存与线程安全」章节）：TTL 1 小时，`forceRefresh=true` 时强制刷新
- `GetRemoteSkillContent(ctx, name) (string, error)`
  - 调 `GetRawContent(ctx, "sigpanic", "goink-skills", "main", "skills/{name}.md")`
- `InstallRemoteSkill(ctx, name, target, novelID) error`
  - 内部调 `GetRemoteSkillContent`
  - `target=user` → 写入 `~/.goink/skills/{name}.md`，触发 `skill.Store.ReloadUser()`
  - `target=novel` → 写入 `{novel_dir}/skills/{name}.md`，触发 `skill.Store.ReloadNovel(novelID)`
  - 后端不做存在性判断（前端弹确认框处理覆盖）

`RemoteSkillMeta` 字段对齐 `goink-skills/scripts/generate-index.py` 输出（与本地 `skill.SkillMeta` 基本一致：
`name` / `description` / `category` / `mode` / `author` / `version`）。

**架构关系**：`App` 持有 `*remote.Service`，`remote.Service` 持有 `*skill.Store` 引用
（单向依赖，避免循环）。`remote.Service` 通过 `rawContentFetcher` / `skillReloader`
两个未导出接口注入依赖，便于测试时 mock。

**测试接口模式**：
- `rawContentFetcher` 抽象 `*githubapi.Client.GetRawContent`
- `skillReloader` 抽象 `*skill.Store.ReloadUser/ReloadNovel`
- `dirResolver` 函数字段注入目录解析逻辑，测试时用内存目录避免污染真实文件系统

单测用 `httptest` mock + mock reloader，覆盖正常路径 + 各类错误传播 + 缓存命中/过期/强制刷新 + 并发安全（`go test -race` 通过）。

### 第四部分：Wails 应用层

在 `app/skill_api.go` 新增三个方法（与现有 `ListSkills` / `DeleteSkill` 并列）。
**所有方法返回 `*apperr.Result[T]`**，统一错误码透传（详见 `error-code-system.md`）。
返回的 `Result.ErrCode` 字段值带模块前缀（如 `githubapi.not_found`），前端按模块做差异化反馈：

| 方法 | 入参 | 返回 |
|---|---|---|
| `ListRemoteSkills(input)` | `ListRemoteSkillsInput{Page, Size, Query}` | `*apperr.Result[*storage.PageResult[remote.RemoteSkillMeta]]` |
| `GetRemoteSkillContent(name)` | `string` | `*apperr.Result[string]` |
| `InstallRemoteSkill(input)` | `InstallRemoteSkillInput{Name, Target, NovelID}` | `*apperr.Result[apperr.Empty]` |

**分页与搜索**在 app 层完成（remote.Service 仍返回全量，详见「分页与搜索设计」章节）。

改完后跑 `wails generate module` 重新生成 `App.d.ts` 和 `models.ts`
（**不手改**自动生成文件，符合 `.trae/rules/编码规则.md`）。

### 第五部分：前端 UI

新建 `frontend/src/components/skill/SkillMarketplace.tsx`：

- **顶部工具栏**：搜索框（实时模糊匹配 name/description）+ 分类筛选（复用现有 6 大分类）+ "刷新"按钮
- **列表区**：远程 skill 卡片（分页展示，默认每页 20 条，详见「分页与搜索设计」章节）
  - 显示 `name` / `description` / `category` / `author` / `version`
  - 状态标识：
    - 未安装 → 「安装」按钮
    - 已安装（本地 version ≥ 远程）→ 「已安装 v{local}」灰色标签
    - 可更新（本地 version < 远程）→ 「更新 v{remote}」按钮
- **分页栏**：底部固定分页器，显示总数 / 当前页 / 总页数，支持上下页跳转
  （遵循项目记忆：禁止无限滚动，必须真分页）
- **详情面板**：点击卡片展开右侧，复用 `SkillPreview.tsx` 渲染 markdown
  - 调 `GetRemoteSkillContent(name)` 拉取全文
- **安装弹窗**（用户选择层级）：
  - 标题：「安装 skill `{name}`」
  - 单选：`user 层（所有小说共享，推荐）` / `novel 层（仅当前小说）`
  - 如果检测到本地已存在，改为「覆盖安装」确认框，显示本地/远程 version 对比
- **网络失败态**（核心需求，按 `err_code` 分类反馈，详见 `error-code-system.md`）：

  skill 市场的远程 API 全部走 githubapi 客户端，故错误码均带 `githubapi.` 前缀；前端按 err_code 分类反馈。
  - `err_code = "githubapi.network"` → 红色提示条：
    > **无法连接 GitHub API**
    > `api.github.com` 国内通常可直连（区别于 `github.com`）。如长期失败，请尝试：
    > 1. 检查网络代理设置
    > 2. 手动访问 https://github.com/sigpanic/goink-skills 浏览社区 skill
    > 3. 在可访问 GitHub 的网络环境中使用此功能
    >
    > [重试]
  - `err_code = "githubapi.rate_limited"` → 「GitHub API 请求频率超限（匿名 60 次/小时），重置时间：{X-RateLimit-Reset 格式化}，请稍后再试」
  - `err_code = "githubapi.forbidden"` → 「访问被拒绝，可能是仓库权限问题」
  - `err_code = "githubapi.not_found"` → 「该 skill 可能已被维护者移除，请刷新列表」
  - `err_code = "invalid"` → 表单错误提示（前端预校验应避免此情况）
  - 其他 → 「获取 skill 失败：{err_msg}」+ [重试]

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

## 缓存与线程安全

### 缓存策略

| 维度 | 选择 | 说明 |
|---|---|---|
| 缓存位置 | 进程内内存 | 不落盘，进程重启失效。避免文件 I/O + JSON 反序列化复杂度 |
| 缓存粒度 | `index.json` 全量 | 单个 skill 内容（`skills/{name}.md`）不缓存，每次按需拉取 |
| TTL | 1 小时 | 平衡 GitHub 匿名 60 req/h 限制与用户对「最新」的预期 |
| 强制刷新 | 支持 | `ListRemoteSkills(ctx, forceRefresh=true)` 跳过缓存直接调 API |
| 文件缓存兜底 | 不做 | 内存缓存失效就重新拉，无需文件 fallback |

**缓存字段**（`remote.Service` 内部）：

```go
cacheMu     sync.RWMutex      // 保护 cacheSkills / cacheAt 的并发读写
cacheSkills []RemoteSkillMeta // nil 表示无缓存
cacheAt     time.Time         // 缓存写入时间
```

### 线程安全设计

Wails 暴露的方法可能被前端并发调用（用户疯狂点刷新 + 同时点安装等场景），
`remote.Service` 必须线程安全。

**并发模型**：

```go
func (s *Service) ListRemoteSkills(ctx context.Context, forceRefresh bool) ([]RemoteSkillMeta, error) {
    // 1. 读锁检查缓存命中：非强制刷新 + 有缓存 + 未过期
    s.cacheMu.RLock()
    if !forceRefresh && s.cacheSkills != nil && time.Since(s.cacheAt) < cacheTTL {
        skills := s.cacheSkills
        s.cacheMu.RUnlock()
        return skills, nil
    }
    s.cacheMu.RUnlock()

    // 2. 调 API 时不持锁，避免网络 I/O 阻塞其他读
    body, _, err := s.client.GetRawContent(ctx, repoOwner, repoName, branch, indexPath)
    // ...

    // 3. 写锁更新缓存
    s.cacheMu.Lock()
    s.cacheSkills = idx.Skills
    s.cacheAt = time.Now()
    s.cacheMu.Unlock()

    return idx.Skills, nil
}
```

**关键决策**：

- 用 `sync.RWMutex` 而非 `sync.Mutex`：读多写少，读锁并发性更好
- 调 API 时**不持锁**：网络 I/O 可能耗时数秒，持锁会阻塞所有并发读
- 缓存更新时**允许并发重复请求**：两个并发 miss 都会调 API，但最后一次写入胜出，
  语义可接受（不引入 singleflight 复杂度）
- `GetRemoteSkillContent` / `InstallRemoteSkill` 不涉及缓存，无需加锁

**测试验证**：`go test -race ./internal/skill/remote/` 全部通过，race detector 无告警。

---

## 分页与搜索设计

### 为什么需要分页

- 社区 skill 数量可能增长到几十甚至上百个，全量返回会让前端渲染卡顿
- 项目记忆硬约束：**禁止无限滚动，必须真分页**，且页面大小合理（不能用 size=100 假分页）
- 与项目其他列表 API（如 `GetSessions`）保持一致的分页形态

### 分页位置：app 层

`remote.Service` 仍返回**全量**列表（来自 `index.json`，本身规模有限，约几十到几百条）。
**分页与搜索在 `app/skill_api.go` 层完成**：

```go
func (a *App) ListRemoteSkills(input ListRemoteSkillsInput) *apperr.Result[*storage.PageResult[remote.RemoteSkillMeta]] {
    all, err := a.remoteService.ListRemoteSkills(a.ctx, false)
    if err != nil {
        return apperr.Err[*storage.PageResult[remote.RemoteSkillMeta]](err)
    }

    // 1. 搜索过滤（name/description 模糊匹配，大小写不敏感）
    filtered := filterByQuery(all, input.Query)

    // 2. 分页（复用 storage.PageParams.Normalize + storage.NewPageResult）
    p := (&storage.PageParams{Page: input.Page, Size: input.Size}).Normalize()
    start := (p.Page - 1) * p.Size
    end := start + p.Size
    if start > len(filtered) {
        start = len(filtered)
    }
    if end > len(filtered) {
        end = len(filtered)
    }
    page := storage.NewPageResult(filtered[start:end], int64(len(filtered)), p.Page, p.Size)

    return apperr.Ok(page)
}
```

### 入参结构

```go
type ListRemoteSkillsInput struct {
    Page  int    `json:"page"`  // 1-based，< 1 归一化为 1
    Size  int    `json:"size"`  // 默认 20，上限 100（storage.PageParams 约束）
    Query string `json:"query"` // 模糊匹配 name + description，空串表示不过滤
}
```

### 搜索语义

- **匹配字段**：`name` + `description`（不匹配 author/category，避免命中过广）
- **匹配方式**：子串包含，大小写不敏感（`strings.Contains(strings.ToLower(...))`）
- **空 query**：返回全量（分页后）
- **分类筛选**：前端在拿到分页结果后**不**做二次筛选（会破坏分页语义），
  而是把分类作为 query 的一部分传给后端，或单独加 `Category` 字段

> 当前实现先用 `Query` 单字段模糊匹配；若后续需要按 category 独立筛选，
> 再扩展 `ListRemoteSkillsInput` 增加 `Category string` 字段，后端做精确匹配。

### 前端分页器

- 底部固定分页栏：`共 {total} 条 · 第 {page}/{total_pages} 页`
- 上一页 / 下一页按钮，禁用边界
- 每页大小切换：10 / 20 / 50（默认 20）
- 翻页时调 `ListRemoteSkills({page, size, query})` 重新请求

---

## 实施顺序（分步提交）

每步写完发询问等 review，用户说 commit 才提交，再进下一步。
步骤 1-2 已完成（commit b20e141 / 502127a），步骤 3 的 remote 包已实现完毕等待审核。

| 步骤 | 内容 | 类型 | 状态 |
|---|---|---|---|
| 1 | 修 `SkillContributeDialog.tsx` 的 URL bug（REPO + BRANCH 两处常量） | fix | ✅ 已 commit (b20e141) |
| 2 | 新建 `internal/githubapi/client.go` 通用客户端 + 单测 | feat | ✅ 已 commit (502127a) |
| 3a | 新建 `internal/skill/remote` 包（含内存缓存 + RWMutex）+ 单测 | feat | ⏳ 等待用户审核 commit |
| 3b | `goink-skills` 仓库新增 `emotional-arc.md` 真实 skill + `generate-index.py` 加 version 字段 + 重生成 index.json | feat | ⏳ 等待与 3a 一起 commit |
| 3c | 新增 `docs/feat/v1.2.0/error-code-system.md` + 更新 `skill-marketplace.md` 补充缓存/线程安全/分页设计 | docs | ⏳ 等待与 3a 一起 commit |
| 4 | 新建 `internal/apperr/` 包（`Result[T]` + `Code` + `CodeFromError`）+ 单测 | feat | 待做 |
| 5 | 新增 `app/skill_api.go` 三个方法（返回 `*apperr.Result[T]`）+ `wails generate module` 重生成绑定 | feat | 待做 |
| 6 | 新增前端 `SkillMarketplace.tsx` + 入口按钮 + 安装弹窗 + 覆盖确认 + 分页器 + 网络失败态 UI | feat | 待做 |
| 7 | i18n 文案补充 + 全量验证（`go build` / `go test -race` / `npm run build` / `eslint`） | feat + test | 待做 |

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
- 不做仓库内嵌 web
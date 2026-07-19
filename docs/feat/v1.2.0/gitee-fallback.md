# v1.2.0 — gitee 镜像与 fallback

## 背景与目标

GitHub API（`api.github.com`）国内访问虽比 `github.com` 稳定，但仍存在偶发不可达、被限流的情况。
v1.2.0 skill 市场功能依赖 `api.github.com` 拉取 `goink-skills` 仓库的 `index.json` 和 skill 全文，
一旦 GitHub 不可用，市场功能完全瘫痪。

本次目标：建立 `gitee.com/sigpanic/goink-skills` 镜像仓库，GitHub push 自动同步到 gitee；
应用层在 GitHub 失败时自动 fallback 到 gitee，用户无感知。

## 整体架构

```
goink-skills (GitHub main)  ──push──▶  GitHub Actions  ──mirror──▶  gitee goink-skills (main)
                                                                        │
                                                                        │
应用 SkillMarketplace                                                     │
  └─ gitapi.Client.GetRawContent                                          │
       ├─ 1. GitHubProvider ──▶ api.github.com  ──✓──▶ 返回              │
       │                          └─✗ (network/rate_limited)──┐          │
       │                                                       ▼          │
       └─ 2. GiteeProvider  ──▶ gitee.com/api/v5  ──✓──▶ 返回 ◀──────────┘
                                   └─✗ ──▶ 返回最后一个错误
```

fallback 触发条件：GitHub 返回 `KindNetwork` / `KindRateLimited`。
不触发 fallback：`KindNotFound` / `KindForbidden` / `KindOther`（这些说明仓库本身有问题，gitee 镜像也一样有问题）。

## 第一部分：gitee 仓库与自动同步

### gitee 仓库建立（手动）

1. 在 gitee 注册账号，用户名建议与 GitHub 一致（`sigpanic`）
2. 建空仓库 `sigpanic/goink-skills`（**不**初始化 README，避免首次 push 冲突）
3. 生成 gitee 私人令牌（个人设置 → 私人令牌，勾选 `projects` 权限），用于 GitHub Actions 推送

### GitHub → gitee 自动同步

在 `goink-skills` 仓库加 `.github/workflows/sync-to-gitee.yml`：

```yaml
name: Sync to Gitee
on:
  push:
    branches: [main]
  workflow_dispatch:

jobs:
  sync:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - name: Mirror to Gitee
        run: |
          git remote add gitee https://${{ secrets.GITEE_USERNAME }}:${{ secrets.GITEE_TOKEN }}@gitee.com/sigpanic/goink-skills.git
          git push --force gitee main
```

**密钥配置**（Repository secrets，不是 Environment secrets——sync 操作不需要环境保护）：
- `GITEE_USERNAME`：gitee 用户名
- `GITEE_TOKEN`：gitee 私人令牌

配置位置：goink-skills 仓库 Settings → Secrets and variables → Actions → New repository secret。

**为什么用 `--force`**：gitee 镜像只读，永远以 GitHub 为准，force push 避免历史分叉冲突。

## 第二部分：应用 fallback 设计

### githubapi 包处理方式

**rename `internal/githubapi/` 为 `internal/gitapi/`**，基于原包扩展。

理由：
- 避免双层封装（保留 githubapi 再让 gitapi 调用会多一层间接）
- rename 后 `gitapi` 包内并列 `github.go` 和 `gitee.go`，结构清晰
- `internal/update/checker.go` 目前裸调 `api.github.com` 不用 githubapi 包，不受影响
- 调用方只有 `internal/skill/remote/service.go`，rename 影响面小

rename 操作：
1. `git mv internal/githubapi internal/gitapi`
2. 改 package 名 `githubapi` → `gitapi`
3. 改 `internal/skill/remote/service.go` 的 import 和引用
4. 改 `app/skill_api.go` 的错误码前缀引用（如果有）

### gitapi 包结构

```
internal/gitapi/
  client.go       Provider 接口 + Client struct + GetRawContent fallback 逻辑
  github.go       GitHubProvider（从原 githubapi.Client 迁移）
  gitee.go        GiteeProvider（新写）
  errors.go       错误类型 + isRetryable 判断
  client_test.go  httptest mock 测试
```

### Provider 接口设计

```go
type Provider interface {
    GetRawContent(ctx context.Context, owner, repo, branch, path string) ([]byte, *RateLimit, error)
}
```

`Client` 持有 `[]Provider`，按优先级 fallback：

```go
type Client struct {
    providers []Provider  // [GitHubProvider, GiteeProvider]
}

func (c *Client) GetRawContent(ctx, owner, repo, branch, path string) ([]byte, *RateLimit, error) {
    var lastErr error
    for _, p := range c.providers {
        content, rl, err := p.GetRawContent(ctx, owner, repo, branch, path)
        if err == nil {
            return content, rl, nil
        }
        lastErr = err
        if !isRetryable(err) {
            return nil, rl, err  // not_found/forbidden 直接返回，不 fallback
        }
        // network/rate_limited → 继续下一个 provider
    }
    return nil, nil, lastErr
}
```

`isRetryable(err)` 判断：`KindNetwork` / `KindRateLimited` 返回 true，其他返回 false。

### GitHubProvider

从现有 `githubapi.Client` 迁移，逻辑不变：
- 端点：`https://api.github.com/repos/{owner}/{repo}/contents/{path}?ref={branch}`
- Header：`Accept: application/vnd.github.raw+json`（直接返回原文）
- 错误分类：`KindNetwork` / `KindRateLimited` / `KindNotFound` / `KindForbidden` / `KindOther`
- rate limit 解析：`X-RateLimit-Remaining` / `X-RateLimit-Reset` header

### GiteeProvider

新写，与 GitHubProvider 的关键差异：

| 项 | GitHub | gitee |
|---|---|---|
| 端点 | `api.github.com/repos/{owner}/{repo}/contents/{path}?ref={branch}` | `gitee.com/api/v5/repos/{owner}/{repo}/contents/{path}?ref={branch}` |
| 返回格式 | `Accept: raw` header 直接返回原文 | JSON 含 `content` 字段（base64 编码）+ `encoding` 字段，需 base64 解码 |
| 鉴权 | 匿名 | 匿名访问公开仓库（应用内不内置 token） |
| rate limit | 403 + `X-RateLimit-Remaining: 0` | 未文档化，暂按 HTTP 状态码分类（429 → rate_limited，403 → forbidden，404 → not_found） |
| User-Agent | `Goink` | `Goink`（gitee 也建议带） |

gitee API 返回 JSON 结构：
```json
{
  "content": "LS0tCm5hbWU6...",  // base64 编码的文件内容
  "encoding": "base64",
  "name": "index.json",
  "path": "index.json",
  ...
}
```

GiteeProvider 需要反序列化 JSON + base64 解码 `content` 字段。

### 错误码设计

错误码按 provider 分前缀，**分开**（用户要求）：

| 错误码 | 含义 |
|---|---|
| `githubapi.network` | GitHub 网络失败 |
| `githubapi.rate_limited` | GitHub 限流 |
| `githubapi.not_found` | GitHub 404 |
| `githubapi.forbidden` | GitHub 403 |
| `githubapi.other` | GitHub 其他错误 |
| `gitee.network` | gitee 网络失败 |
| `gitee.rate_limited` | gitee 限流 |
| `gitee.not_found` | gitee 404 |
| `gitee.forbidden` | gitee 403 |
| `gitee.other` | gitee 其他错误 |

**fallback 后的错误返回**：
- GitHub 失败 + gitee 成功 → 返回成功（用户无感知）
- GitHub 失败 + gitee 也失败 → 返回 **gitee 的错误**（最后一个 provider 的错误），错误码是 `gitee.*`

前端 [classifyError](file:///home/nianhe/projects/todo/frontend/src/components/skill/SkillMarketplace.tsx) 已用 `endsWith` 匹配短码：
- `code.endsWith('network')` → 匹配 `githubapi.network` 和 `gitee.network`
- `code.endsWith('rate_limited')` → 匹配 `githubapi.rate_limited` 和 `gitee.rate_limited`

所以**前端无需改动**，classifyError 自动兼容 gitee 错误码。

### 缓存策略

`internal/skill/remote/service.go` 的内存缓存（TTL 1 小时）**共享**，不区分 provider：
- 缓存 key：`index.json`（内容相同，GitHub 和 gitee 返回的是同一份内容）
- GitHub 成功 → 缓存命中，不调 gitee
- GitHub 失败 + gitee 成功 → 缓存 gitee 返回的内容
- 下次请求 → 缓存命中，不调任何 provider

### 错误反馈

| 场景 | 用户看到 |
|---|---|
| GitHub 成功 | 正常列表（无感知） |
| GitHub 网络失败 + gitee 成功 | 正常列表（无感知） |
| GitHub rate limit + gitee 成功 | 正常列表（无感知） |
| GitHub 失败 + gitee 也网络失败 | 「无法连接 GitHub（api.github.com）...」（gitee.network 错误，classifyError 匹配 network 分支） |
| GitHub 失败 + gitee 也 rate limit | 「GitHub API 限流...」（gitee.rate_limited，classifyError 匹配 rate_limited 分支） |
| GitHub 失败 + gitee 404 | 「未找到该 skill...」（gitee.not_found） |

前端 errorNetwork 文案提到 `api.github.com`，fallback 后 gitee 也失败时文案会不太准确（只提了 GitHub 没提 gitee）。
**可选改进**：errorNetwork 文案改为「无法连接 GitHub 与 gitee 镜像，请检查网络...」，但这会让纯 GitHub 失败的提示也带上 gitee 字样，需要权衡。
**推荐**：暂不改文案，保持「无法连接 GitHub」即可——因为用户不知道有 gitee fallback，提示 GitHub 足够引导排查。

## 文件改动清单

### 新建

| 文件 | 内容 |
|---|---|
| `internal/gitapi/client.go` | Provider 接口 + Client + GetRawContent fallback 逻辑 |
| `internal/gitapi/github.go` | GitHubProvider（从 githubapi 迁移） |
| `internal/gitapi/gitee.go` | GiteeProvider（新写，base64 解码） |
| `internal/gitapi/errors.go` | 错误类型 + isRetryable |
| `internal/gitapi/client_test.go` | httptest mock 测试（GitHub 成功 / GitHub 失败 gitee 成功 / 都失败 / not_found 不 fallback） |
| `.github/workflows/sync-to-gitee.yml`（goink-skills 仓库） | GitHub Actions 同步 |

### Rename / 迁移

| 操作 | 说明 |
|---|---|
| `git mv internal/githubapi internal/gitapi` | rename 包目录 |
| package `githubapi` → `gitapi` | 改 package 声明 |

### 改引用

| 文件 | 改动 |
|---|---|
| `internal/skill/remote/service.go` | import `githubapi` → `gitapi`，`*githubapi.Client` → `*gitapi.Client` |
| `internal/skill/remote/service_test.go` | mock 接口改为 `gitapi.Provider` |
| `app/skill_api.go` | 错误码前缀引用（如果有） |
| `app/handler.go` | 同上 |

### 文档

| 文件 | 改动 |
|---|---|
| `docs/feat/v1.2.0/skill-marketplace.md` | 加「gitee 镜像与 fallback」章节，链接到本文档 |
| `docs/error-code-system.md` | 加 `gitee.*` 错误码前缀说明 |

## 测试计划

### 单元测试（gitapi 包）

| 用例 | 预期 |
|---|---|
| GitHub 成功 | 返回 GitHub 内容，不调 gitee |
| GitHub network 失败 + gitee 成功 | 返回 gitee 内容 |
| GitHub rate_limited + gitee 成功 | 返回 gitee 内容 |
| GitHub not_found | 返回 githubapi.not_found，**不 fallback** |
| GitHub forbidden | 返回 githubapi.forbidden，**不 fallback** |
| GitHub network + gitee network | 返回 gitee.network（最后一个错误） |
| GitHub network + gitee rate_limited | 返回 gitee.rate_limited |
| gitee 返回 JSON base64 | 正确解码为原文 |

### 集成测试（remote/service）

| 用例 | 预期 |
|---|---|
| GitHub 成功 + 缓存未过期 | 返回 GitHub 内容，缓存命中 |
| GitHub 失败 + gitee 成功 | 返回 gitee 内容，写入缓存 |
| 缓存命中 | 不调任何 provider |

### 端到端测试（手动）

| 场景 | 操作 |
|---|---|
| GitHub 正常 | 打开市场，正常列表 |
| GitHub 不可达 | 断网或 hosts 屏蔽 api.github.com，打开市场，gitee 镜像正常返回 |
| 双失败 | hosts 同时屏蔽 api.github.com 和 gitee.com，打开市场，显示网络错误 |

## 工作量与优先级

| 任务 | 工作量 | 优先级 |
|---|---|---|
| gitee 仓库建立 | 10 分钟（手动） | 高 |
| GitHub Actions 同步 | 5 分钟 | 高 |
| gitapi 包 + GitHubProvider 迁移 | 1 小时 | 中 |
| GiteeProvider | 1 小时 | 中 |
| service / app 改引用 | 30 分钟 | 中 |
| 测试 | 1.5 小时 | 中 |
| 文档更新 | 30 分钟 | 低 |
| **合计** | **约 5 小时** | |

**优先级说明**：v1.2.0 核心功能（市场 + 安装 + 可更新状态）已实现，gitee fallback 属于网络容错增强，
不影响核心功能可用性。如果 v1.2.0 时间紧，可拆为 v1.2.1 单独发布。

## 开放问题

1. **gitee rate limit 机制**：gitee 官方未文档化匿名访问的频率限制，需实测确认。如果限制过严，考虑加配置项让用户填 gitee token。
2. **gitee API 返回大文件**：gitee API 对大文件可能直接返回 4xx 而非 base64（与 GitHub 类似），GiteeProvider 需处理这种情况。
3. **gitee 镜像延迟**：GitHub Actions 同步有 1-2 分钟延迟，用户在 GitHub push 后立即打开市场可能拉到旧内容。可接受（skill 仓库不频繁更新）。
4. **errorNetwork 文案**：fallback 后双失败时，文案只提 GitHub 不提 gitee，是否需要改？推荐暂不改（保持用户认知简单
# v1.2.0 — 错误码体系（apperr）

## 背景与动机

v1.2.0 之前，应用层 Wails 方法对前端的错误传递完全依赖 `error` 接口。
前端拿到的字符串形如 `"githubapi: request failed: ..."` / `"[429] rate limit"`，
**只能用字符串匹配或正则来区分错误类型**，这导致：

1. 前端无法稳定地针对网络失败 / rate limit / 404 / forbidden 给出差异化提示
2. 错误信息文案改一行，前端匹配就跟着炸
3. 后端错误结构（`githubapi.Error` / `llm.APIError`）没有统一透传出口

v1.2.0 新增 skill 市场功能恰好是首个对**错误分类反馈**有硬性需求的场景
（用户要求网络失败时给出明确可操作的提示），借此机会引入轻量错误码体系。

## 设计目标

- **作为基础设施**：对标 `storage.PageResult[T]`，提供 `apperr.Result[T]` 泛型返回包装，
  让所有需要分类错误反馈的 Wails API 有统一形态
- **零侵入**：不强制重构现有 API，只在新增 API 上启用；旧 API 保持原签名
- **可扩展**：错误码用字符串而非 int 枚举，便于后续新增场景直接加值
- **后端语义保留**：`CodeFromError` 通过 `errors.As` 提取底层 `*githubapi.Error` / `*llm.APIError`，
  不丢失原始 Kind/StatusCode 信息

## 设计概览

```
┌──────────────────────────────────────────────────────────┐
│  apperr 包（internal/apperr/apperr.go）                  │
│                                                          │
│  type Code string                                        │
│  type Result[T any] struct { Data; ErrCode; ErrMsg }     │
│  func Ok[T](data T) *Result[T]                           │
│  func Err[T](err error) *Result[T]                       │
│  func CodeFromError(err error) Code                      │
│  type Empty = struct{}                                   │
└──────────────────────────────────────────────────────────┘
            ▲                              ▲
            │ 用于映射                       │ 业务调用
            │                              │
┌───────────┴────────────┐    ┌────────────┴────────────────┐
│ githubapi.Error        │    │ app/skill_api.go (Wails)    │
│   .Kind (5 类)         │    │  ListRemoteSkills           │
│                        │    │  GetRemoteSkillContent      │
│ llm.APIError           │    │  InstallRemoteSkill         │
│   .StatusCode          │    │                             │
│   .Retryable           │    │  全部返回 *apperr.Result[T] │
└────────────────────────┘    └─────────────────────────────┘
```

## 错误码定义

```go
type Code string

// 通用错误码（跨模块共享）
const (
    CodeOK       Code = ""
    CodeInternal Code = "internal"
    CodeInvalid  Code = "invalid"
)

// githubapi 模块错误码
const (
    CodeGitHubAPINetwork     Code = "githubapi.network"
    CodeGitHubAPIRateLimited Code = "githubapi.rate_limited"
    CodeGitHubAPINotFound    Code = "githubapi.not_found"
    CodeGitHubAPIForbidden   Code = "githubapi.forbidden"
    CodeGitHubAPIOther       Code = "githubapi.other"
)

// llm 模块错误码
const (
    CodeLLMRateLimited Code = "llm.rate_limited"
    CodeLLMNotFound    Code = "llm.not_found"
    CodeLLMForbidden   Code = "llm.forbidden"
    CodeLLMServerError Code = "llm.server_error"
    CodeLLMClientError Code = "llm.client_error"
)
```

### 为什么按模块分区

- **避免 const 块无限增长**：每个模块一个 const 区，新增模块只加新区不动旧代码
- **switch 拆函数**：每个模块的映射逻辑独立，便于维护和测试
- **同样语义不同模块不同错误码**：404 在 githubapi 是「skill 文件被移除」，在 llm 是「模型不存在」，前端反馈应不同。错误码带模块前缀让前端能精确区分

**为什么用 `string` 而不是 `int`**：

- 字符串本身即文档（`"rate_limited"` 比 `4` 直观）
- 前端 TypeScript 可直接 `switch(code)` 字面量
- 扩展时不需要担心编号冲突

**为什么 `CodeOK` 是空字符串**：

- `Result[T]` 零值（`ErrCode: ""`）天然代表成功，反序列化/默认构造无歧义
- 前端 `if (result.err_code)` 自然 falsy 检查

## `Result[T]` 结构

```go
type Result[T any] struct {
    Data    T      `json:"data"`              // 业务数据（失败时为零值）
    ErrCode Code   `json:"err_code"`          // 空字符串表示成功
    ErrMsg  string `json:"err_msg,omitempty"` // 人类可读错误描述（仅失败时存在）
}
```

设计要点：

1. **始终返回 200**：Wails 方法不返回 Go `error`，而是返回 `*Result[T]`。
   传输层永远是成功序列化，业务错误体现在 `ErrCode` 字段。
2. **泛型 `T`**：可以是 `*storage.PageResult[X]`、`string`、`apperr.Empty` 等
3. **`ErrMsg` 用 `omitempty`**：成功响应 JSON 不带错误字段，前端类型定义更干净

## 构造函数

```go
// Ok 包装成功结果
func Ok[T any](data T) *Result[T] {
    return &Result[T]{Data: data}
}

// Err 包装失败结果，自动通过 CodeFromError 推断错误码
func Err[T any](err error) *Result[T] {
    return &Result[T]{
        ErrCode: CodeFromError(err),
        ErrMsg:  err.Error(),
    }
}

// Empty 用于无业务数据返回的场景（如 InstallRemoteSkill）
type Empty = struct{}
```

## `CodeFromError` 映射规则

```go
// CodeFromError 只做调度，具体映射逻辑在各模块专属函数里
func CodeFromError(err error) Code {
    if err == nil {
        return CodeOK
    }
    var ghErr *githubapi.Error
    if errors.As(err, &ghErr) {
        return codeFromGitHubAPIError(ghErr)
    }
    var llmErr *llm.APIError
    if errors.As(err, &llmErr) {
        return codeFromLLMAPIError(llmErr)
    }
    // 业务层 fmt.Errorf("remote: invalid target %q", target) 等约定
    msg := err.Error()
    if strings.Contains(msg, "invalid") || strings.Contains(msg, "requires non-zero") {
        return CodeInvalid
    }
    return CodeInternal
}

// codeFromGitHubAPIError 映射 *githubapi.Error.Kind → apperr.Code
func codeFromGitHubAPIError(err *githubapi.Error) Code {
    switch err.Kind {
    case githubapi.KindNetwork:     return CodeGitHubAPINetwork
    case githubapi.KindRateLimited: return CodeGitHubAPIRateLimited
    case githubapi.KindNotFound:    return CodeGitHubAPINotFound
    case githubapi.KindForbidden:   return CodeGitHubAPIForbidden
    default:                        return CodeGitHubAPIOther
    }
}

// codeFromLLMAPIError 映射 *llm.APIError.StatusCode → apperr.Code
func codeFromLLMAPIError(err *llm.APIError) Code {
    switch {
    case err.StatusCode == 429:  return CodeLLMRateLimited
    case err.StatusCode == 404:  return CodeLLMNotFound
    case err.StatusCode == 403:  return CodeLLMForbidden
    case err.StatusCode >= 500:  return CodeLLMServerError
    default:                     return CodeLLMClientError
    }
}
```

**为什么用 `errors.As` 而非字符串匹配**：

- `remote.Service` 用 `fmt.Errorf("remote: ...: %w", err)` 包装底层错误，保留 Unwrap 链
- `errors.As` 能穿透多层包装直达 `*githubapi.Error`，比字符串前缀匹配可靠

**为什么 LLM 错误也映射进来**：

虽然 v1.2.0 只有 skill 市场会用到，但 LLM 调用错误（429/5xx）也有相同的分类需求。
提前映射让 apperr 包真正成为「基础设施」而非 skill 专用工具。

## Wails API 应用示例

```go
// app/skill_api.go

type ListRemoteSkillsInput struct {
    Page  int    `json:"page"`
    Size  int    `json:"size"`
    Query string `json:"query"`  // 名称/描述模糊搜索
}

func (a *App) ListRemoteSkills(input ListRemoteSkillsInput) *apperr.Result[*storage.PageResult[remote.RemoteSkillMeta]] {
    all, err := a.remoteService.ListRemoteSkills(a.ctx, false)
    if err != nil {
        return apperr.Err[*storage.PageResult[remote.RemoteSkillMeta]](err)
    }
    filtered := filterAndSearch(all, input.Query)
    page := paginate(filtered, input.Page, input.Size)
    return apperr.Ok(page)
}

func (a *App) GetRemoteSkillContent(name string) *apperr.Result[string] {
    content, err := a.remoteService.GetRemoteSkillContent(a.ctx, name)
    if err != nil {
        return apperr.Err[string](err)
    }
    return apperr.Ok(content)
}

type InstallRemoteSkillInput struct {
    Name    string `json:"name"`
    Target  string `json:"target"`   // "user" or "novel"
    NovelID int64  `json:"novel_id"` // target=novel 时必填
}

func (a *App) InstallRemoteSkill(input InstallRemoteSkillInput) *apperr.Result[apperr.Empty] {
    if err := a.remoteService.InstallRemoteSkill(a.ctx, input.Name, input.Target, input.NovelID); err != nil {
        return apperr.Err[apperr.Empty](err)
    }
    return apperr.Ok(apperr.Empty{})
}
```

## 前端 TypeScript 对应

Wails 自动生成的 `App.d.ts` 会暴露 `Result<T>` 的 TS 类型。前端消费模式：

```ts
type ErrCode = '' | 'internal' | 'invalid'
  | 'githubapi.network' | 'githubapi.rate_limited' | 'githubapi.not_found' | 'githubapi.forbidden' | 'githubapi.other'
  | 'llm.rate_limited' | 'llm.not_found' | 'llm.forbidden' | 'llm.server_error' | 'llm.client_error'

interface Result<T> {
  data: T
  err_code: ErrCode
  err_msg?: string
}

async function loadMarketplace() {
  const res = await app.ListRemoteSkills({ page: 1, size: 20, query: '' })
  if (res.err_code) {
    switch (res.err_code) {
      case 'githubapi.network':
        showNetworkError(res.err_msg)         // 显示重试 + 手动访问仓库提示
        break
      case 'githubapi.rate_limited':
        showRateLimitError(res.err_msg)       // 显示重置时间
        break
      case 'githubapi.not_found':
        showSkillNotFoundError(res.err_msg)   // skill 可能已被移除
        break
      case 'llm.not_found':
        showModelNotFoundError(res.err_msg)   // 模型不存在（与 skill 404 反馈不同）
        break
      default:
        showGenericError(res.err_code, res.err_msg)
    }
    return
  }
  renderSkillList(res.data.items)
}
```

## 错误码与 UI 反馈对应表

| ErrCode | UI 反馈 | 是否可重试 |
|---|---|---|
| `""` (OK) | 正常渲染数据 | — |
| `internal` | 通用错误 + 错误详情折叠 | 是 |
| `invalid` | 表单错误提示（前端预校验应避免） | 否 |
| `githubapi.network` | 红色提示条 + 重试 + 手动访问仓库链接 | 是 |
| `githubapi.rate_limited` | 黄色提示 + 显示重置时间 | 等待后重试 |
| `githubapi.not_found` | 灰色提示 + 建议刷新列表（skill 文件被移除） | 刷新后重试 |
| `githubapi.forbidden` | 红色提示 + 联系维护者 | 否 |
| `githubapi.other` | 通用错误 + 错误详情 | 是 |
| `llm.rate_limited` | LLM 限流提示 + 稍后重试 | 等待后重试 |
| `llm.not_found` | 模型不存在，检查配置 | 否 |
| `llm.forbidden` | API Key 无权限 | 否 |
| `llm.server_error` | LLM 服务端错误，稍后重试 | 是 |
| `llm.client_error` | LLM 客户端错误，检查请求 | 否 |

## 实施计划

| 步骤 | 内容 | 类型 |
|---|---|---|
| 1 | 新建 `internal/apperr/apperr.go` + `apperr_test.go` | feat |
| 2 | `app/skill_api.go` 三个方法采用 `*apperr.Result[T]` 返回 + `wails generate module` | feat |

## 测试覆盖

`apperr_test.go` 至少覆盖：

- `Ok[T]` 构造的 Result 字段正确（ErrCode="", ErrMsg 缺省）
- `Err[T]` 包装 `*githubapi.Error` 5 个 Kind（Network/RateLimited/NotFound/Forbidden/Other）→ 对应 githubapi.* Code
- `Err[T]` 包装 `*llm.APIError` 5 个 StatusCode（429/404/403/5xx/其他 4xx）→ 对应 llm.* Code
- `Err[T]` 包装多层 `fmt.Errorf("...: %w", err)` 仍能 `errors.As` 透传到底层
- `Err[T]` 包装普通 `errors.New("invalid target")` → CodeInvalid
- `Err[T]` 包装未知错误 → CodeInternal
- `CodeFromError(nil)` → CodeOK

## 不做的事

- **不重构现有 API**：`GetSessions` / `ListSkills` 等老方法保持原签名（返回业务数据 + error），
  本版本只在新引入的 `ListRemoteSkills` / `GetRemoteSkillContent` / `InstallRemoteSkill` 上启用
- **不引入错误码注册机制**：错误码用 const 直接定义，不做 map 注册表
- **不做错误码国际化**：`ErrMsg` 直接传后端 `err.Error()` 原文，
  前端按 `ErrCode` 选择对应的 i18n 文案展示（错误码本身是稳定 ID，描述性文本交给前端 i18n）

## 风险与权衡

1. **泛型 + Wails 绑定**：`*Result[T]` 作为返回类型需要确认 Wails 的 TS 生成器能正确处理
   嵌套泛型（如 `*Result[*storage.PageResult[X]]`），第 4 步 `wails generate module` 后需校验
   `App.d.ts` / `models.ts` 的生成结果
2. **ErrMsg 暴露底层错误**：`err.Error()` 可能包含内部路径/URL，对桌面应用可接受（非 SaaS 多租户），
   但仍需注意未来若加远程上报功能要脱敏
3. **错误码字符串的稳定性**：一旦发布，错误码字符串即成为前端契约，
   后续不可改名（只能新增），需在 README/文档里强调。模块前缀错误码字符串即成为前端契约，后续不可改名（只能新增）

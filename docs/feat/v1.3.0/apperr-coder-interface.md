# v1.3.0 — apperr 演进：Coder 接口模式

## 背景与动机

v1.2.0 引入的 apperr 错误码体系（见 [v1.2.0/error-code-system.md](../v1.2.0/error-code-system.md)）采用**集中映射模式**：`CodeFromError` 依次对每个领域调 `errors.As` 提取结构化 error，再走专属映射函数。

```go
// v1.2.0 现状（internal/apperr/apperr.go）
func CodeFromError(err error) Code {
    var ghErr *githubapi.Error
    if errors.As(err, &ghErr) { return codeFromGitHubAPIError(ghErr) }
    var llmErr *llm.APIError
    if errors.As(err, &llmErr) { return codeFromLLMAPIError(llmErr) }
    // fallback：业务层 fmt.Errorf 纯字符串，只能 strings.Contains
    msg := err.Error()
    if strings.Contains(msg, "invalid") { return CodeInvalid }
    return CodeInternal
}
```

v1.2.0 code-review（[4.2.3](../v1.2.0/v1.2.0-code-review.md)）发现两个问题：

1. **apperr 耦合业务包**：每接入一个新领域，apperr 要 `import` 该包 + 加 `errors.As` 分支 + 加映射函数。集中映射要求 apperr 依赖所有领域包。
2. **fallback 字符串匹配脆弱**：业务层（如 remote service）抛 `fmt.Errorf("remote: invalid target %q", target)` 纯字符串，apperr 拿不到结构化信息只能 `strings.Contains(msg, "invalid")`，OS "invalid argument"、GORM "invalid connection" 会被误判 `CodeInvalid`。

## 设计目标

- **apperr 零业务包依赖**：apperr 只定义接口与 Code 常量，不 import 任何业务/协议包
- **新领域零改 apperr**：业务包定义自己的 error 类型并实现接口即可自动接入，apperr 不动
- **根治字符串匹配**：结构化 error 走接口断言，fallback 只剩真正未结构化的兜底
- **领域信息不丢失**：协议层 error（githubapi/llm）的领域专属字段（ResetAt/Retryable/RetryAfter）保留，供后端重试/rate limit 逻辑用

## 方案：Coder 接口 + 混合策略

### 核心接口

apperr 定义 `Coder` 接口，不依赖任何业务包：

```go
// internal/apperr/apperr.go

// Coder 由携带应用层错误码的 error 实现。
// 各领域 error 类型实现此接口，CodeFromError 用 errors.As 提取接口即可统一分发，
// apperr 无需 import 任何业务/协议包。
type Coder interface {
    Code() Code
}
```

### CodeFromError 重写

从"n 次 errors.As 逐领域提取"改为"1 次 errors.As 提取接口"：

```go
func CodeFromError(err error) Code {
    if err == nil {
        return CodeOK
    }
    var c Coder
    if errors.As(err, &c) {          // 1 次提取，遍历 Unwrap 链找实现 Coder 的 error
        return c.Code()
    }
    return CodeInternal               // 未实现 Coder 的兜底
}
```

- 复杂度从 O(n × d) 降到 O(d)（n=领域数，d=error 链深度）。实际差异纳秒级，非性能驱动；真正好处是**不用写 n 个分支 + 不 import n 个包**。
- 兜底从 `strings.Contains` 改为直接 `CodeInternal`——业务层若想要精确 Code 必须实现 Coder，杜绝字符串误判。

### 混合策略：领域 error + 通用 BusinessError

不同 error 按是否携带领域专属字段分两类处理：

#### 1. 协议/领域 error：保留结构化类型 + 实现 Code()

这类 error 有后端重试/rate limit 逻辑需要的领域字段，**不能改成统一 BusinessError 丢字段**。

```go
// internal/githubapi/client.go
type Error struct {
    Kind    Kind
    Status  int
    Message string
    ResetAt time.Time   // rate limit 重置时间，重试逻辑要用
    Cause   error
}

// 实现 apperr.Coder
func (e *Error) Code() apperr.Code {
    switch e.Kind {
    case KindNetwork:     return apperr.CodeGitHubAPINetwork
    case KindRateLimited: return apperr.CodeGitHubAPIRateLimited
    case KindNotFound:    return apperr.CodeGitHubAPINotFound
    case KindForbidden:   return apperr.CodeGitHubAPIForbidden
    default:              return apperr.CodeGitHubAPIOther
    }
}
```

```go
// internal/llm/types.go
type APIError struct {
    StatusCode  int
    Message     string
    Retryable   bool          // 重试决策要用
    RetryAfter  time.Duration // 退避计算要用
    Kind        string        // agent loop 区分路径
}

func (e *APIError) Code() apperr.Code {
    switch {
    case e.StatusCode == 429:  return apperr.CodeLLMRateLimited
    case e.StatusCode == 404:  return apperr.CodeLLMNotFound
    case e.StatusCode == 403:  return apperr.CodeLLMForbidden
    case e.StatusCode >= 500:  return apperr.CodeLLMServerError
    default:                   return apperr.CodeLLMClientError
    }
}
```

> **注意**：githubapi/llm import apperr 会引入依赖方向变化。需确认无循环依赖：apperr 不再 import githubapi/llm（改前是 apperr→githubapi/llm，改后是 githubapi/llm→apperr）。apperr 变成纯基础设施被各领域依赖，符合分层。

#### 2. 纯业务校验 error：通用 BusinessError

无领域专属字段的业务错误（如 `invalid target`、`requires non-zero novelID`）用 apperr 提供的通用结构化 error：

```go
// internal/apperr/business.go

// BusinessError 是无领域专属字段的业务错误通用载体。
// 实现 Coder 接口，供 CodeFromError 直接读取 Code。
type BusinessError struct {
    Code Code      // apperr.CodeInvalid / apperr.CodeNotFound / ...
    Msg  string
    Cause error
}

func (e *BusinessError) Error() string {
    if e.Cause != nil {
        return fmt.Sprintf("%s: %v", e.Msg, e.Cause)
    }
    return e.Msg
}
func (e *BusinessError) Unwrap() error { return e.Cause }
func (e *BusinessError) Code() Code     { return e.Code }

// 构造便利函数
func NewInvalid(msg string) *BusinessError {
    return &BusinessError{Code: CodeInvalid, Msg: msg}
}
func NewNotFound(msg string) *BusinessError {
    return &BusinessError{Code: CodeNotFound, Msg: msg}   // 需新增 CodeNotFound 常量
}
```

业务层改造（以 remote service 为例）：

```go
// 改前
return fmt.Errorf("remote: invalid target %q (want user or novel)", target)

// 改后
return apperr.NewInvalid(fmt.Sprintf("remote: invalid target %q (want user or novel)", target))
```

或保留 %w 链：

```go
return &apperr.BusinessError{
    Code: apperr.CodeInvalid,
    Msg:  fmt.Sprintf("remote: invalid target %q", target),
    Cause: err,
}
```

### 新增错误码

```go
// 通用错误码补充
const (
    CodeNotFound Code = "not_found"   // 业务资源不存在（跨模块通用）
)
```

`CodeInvalid` 已存在。`CodeNotFound` 新增，对应 BusinessError 的 not_found 场景。

## 信息边界（关键设计约束）

```
后端内部决策（重试 / rate limit / agent loop）
   → 直接 errors.As 提取领域 error 读结构化字段（ResetAt/Retryable/RetryAfter/Kind）
   → 不经 apperr，字段完整保留
   → 这要求领域 error 类型必须保留，不能统一成 BusinessError

API 出口给前端
   → apperr.Err(err) 压缩成 {ErrCode, ErrMsg}
   → 结构化字段序列化时丢失，前端只拿到粗粒度 code + 文本
   → 前端按 ErrCode 分支做 i18n 文案 / UI 行为
```

**领域字段（ResetAt/Retryable 等）是后端决策用的，不给前端。** apperr 的 Code 契约是给前端做粗粒度分类的。两者职责分离。

若未来前端需要某字段（如显示 rate limit 倒计时），扩展 `Result[T]` 加可选字段，或在 ErrMsg 拼文本——当前不做。

## 改动清单

| 步骤 | 文件 | 内容 | 类型 |
|---|---|---|---|
| 1 | `internal/apperr/apperr.go` | 加 `Coder` 接口；`CodeFromError` 重写为 1 次 `errors.As` 提取接口；删 githubapi/llm import；删 `codeFromGitHubAPIError`/`codeFromLLMAPIError`（逻辑移到各 error 的 Code() 方法） | refactor |
| 2 | `internal/apperr/business.go` | 新增 `BusinessError` 结构体 + `NewInvalid`/`NewNotFound` 构造函数；新增 `CodeNotFound` 常量 | feat |
| 3 | `internal/githubapi/client.go` | `Error` 实现 `Code() apperr.Code` 方法（映射逻辑从 apperr 搬来） | refactor |
| 4 | `internal/llm/types.go` | `APIError` 实现 `Code() apperr.Code` 方法（映射逻辑从 apperr 搬来） | refactor |
| 5 | `internal/skill/remote/service.go` | 6 处 `fmt.Errorf("remote: ...")` 业务校验错误改用 `apperr.NewInvalid` / `apperr.NewNotFound`；保留 `%w` 包装底层 githubapi.Error | refactor |
| 6 | `internal/apperr/apperr_test.go` | 更新测试：Coder 接口提取、各领域 Code()、BusinessError、fallback=CodeInternal | test |
| 7 | 其他业务 service | 逐步接入（可在后续迭代完成，非本议题阻塞） | refactor |

## 依赖方向变化

```
v1.2.0（集中映射）：
    apperr → githubapi
    apperr → llm
    skill_api → apperr + remote

v1.3.0（接口模式）：
    githubapi → apperr   （反转）
    llm       → apperr   （反转）
    remote    → apperr   （新增，用 BusinessError）
    skill_api → apperr + remote
    apperr    →（无业务包依赖，纯基础设施）
```

apperr 从"依赖各领域"变成"被各领域依赖"，成为真正的底层基础设施。需确认 githubapi/llm 当前不依赖 apperr（v1.2.0 确认过，无循环）。

## 性能分析

| 模式 | CodeFromError 复杂度 | 实际开销 |
|---|---|---|
| v1.2.0 集中映射 | O(n × d)，n 次 errors.As 各遍历 d 层链 | 纳秒级（n=2, d=1-3） |
| v1.3.0 接口模式 | O(d)，1 次 errors.As 提取接口 | 纳秒级 |

性能不是演进动机（差异可忽略）。真正动机是**解耦**：apperr 不依赖业务包 + 新领域零改 apperr + 根治字符串匹配。

## 不做的事

- **不改 `Result[T]` 结构**：ErrCode + ErrMsg 两字段够用，前端按 code 分支。领域字段不给前端（后端决策用）。
- **不做错误码注册机制**：Code 仍用 const 直接定义，不做 map 注册表。
- **不强制改造所有业务 service**：本议题只改 remote service（4.2.3 直接相关）+ githubapi/llm（接口实现）。其他 service 逐步接入，未接入的走 fallback=CodeInternal，不影响现有行为。
- **不做错误码国际化**：ErrMsg 仍传后端原文，前端按 ErrCode 选 i18n 文案。

## 风险与权衡

1. **依赖方向反转**：githubapi/llm 从"被 apperr 依赖"变成"依赖 apperr"。需确认这两个包当前没有任何对 apperr 的间接依赖（避免循环）。v1.2.0 已确认无循环，演进时需再跑 `go build ./...` 验证。
2. **Code() 方法的归属**：githubapi.Error 的 Code() 返回 apperr.Code，语义上 githubapi 知道 apperr 的错误码常量。可接受——apperr.Code 是稳定契约，githubapi 按契约实现映射合理。
3. **BusinessError 与领域 error 的边界**：判断标准是"后端是否需要领域专属字段"。需要→领域 error；不需要→BusinessError。边界清晰但需文档化，避免新错误乱选。
4. **向后兼容**：v1.2.0 的 `codeFromGitHubAPIError`/`codeFromLLMAPIError` 函数被删，逻辑搬到 Code() 方法。外部若有直接调用这两个私有函数的代码会断（当前无，仅 apperr 内部用）。

## 测试覆盖

`apperr_test.go` 更新：

- `CodeFromError(nil)` → CodeOK
- 包装 `*githubapi.Error` 5 个 Kind → 对应 githubapi.* Code（走 Code() 方法）
- 包装 `*llm.APIError` 5 个 StatusCode → 对应 llm.* Code（走 Code() 方法）
- 包装 `*apperr.BusinessError{Code: CodeInvalid}` → CodeInvalid
- 多层 `fmt.Errorf("...: %w", err)` 仍能 `errors.As` 提取 Coder 接口
- 包装普通 `errors.New("unknown")` → CodeInternal（fallback，不再 strings.Contains）
- 各领域 error 的 `Code()` 方法单元测试

## 关联

- v1.2.0 现有设计：[error-code-system.md](../v1.2.0/error-code-system.md)
- v1.2.0 code-review 4.2.3：[v1.2.0-code-review.md](../v1.2.0/v1.2.0-code-review.md)
- 本议题作为 v1.3.0 第一步大改造，后续业务 service 接入在此基础上逐步推进

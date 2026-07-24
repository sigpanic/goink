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
        return CodeOK                 // nil err → 成功标识
    }
    var c Coder
    if errors.As(err, &c) {          // 1 次提取，遍历 Unwrap 链找实现 Coder 的 error
        code := c.Code()
        if code == CodeOK {           // 不变量：非 nil err 永不返回 CodeOK
            return CodeInternal       // Coder 误返回 CodeOK 时降级，避免错误被静默吞成成功
        }
        return code
    }
    return CodeInternal               // 未实现 Coder 的兜底
}
```

- 复杂度从 O(n × d) 降到 O(d)（n=领域数，d=error 链深度）。实际差异纳秒级，非性能驱动；真正好处是**不用写 n 个分支 + 不 import n 个包**。
- 兜底从 `strings.Contains` 改为直接 `CodeInternal`——业务层若想要精确 Code 必须实现 Coder，杜绝字符串误判。

### CodeOK 不变量（关键契约）

`CodeOK`（空字符串）是 `Code` 类型的零值命名，语义为"无错误/成功"，**不是错误码集合的成员**。它只在两处出现：

- `CodeFromError(nil)` 的返回
- `Result[T]` 零值的 `ErrCode` 字段（前端 `if (res.err_code)` 自然 falsy 检查成功）

**不变量：非 nil err 的 `CodeFromError` 永不返回 CodeOK。** 若某个 Coder 实现的 `Code()` 方法误返回 CodeOK（如 `BusinessError.Code` 字段未赋值取了零值），`CodeFromError` 会降级为 `CodeInternal`，而不是把错误静默吞成成功。

- **不 panic**：`CodeFromError` 是 API 出口必经路径，panic 会把"分类失败"升级成"进程崩溃"，影响面与错误严重度不匹配。降级即可——错误不会被误判为成功，只是分类不精确落 `CodeInternal`，`ErrMsg` 仍是原文，用户看得到错误。
- **可观测**：降级时可选加 `slog.Warn` 记录"Coder 返回 CodeOK，降级 CodeInternal"便于排查，本议题不强制。
- **源头治理**：`NewInvalid`/`NewNotFound` 构造函数天然返回非空 Code；若直接用 struct literal 构造 `BusinessError` 绕过构造函数，`Code()` 方法内可自保 `if e.Code == "" { return CodeInternal }`。源头治理 + CodeFromError 兜底，双重防线。

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
//
// 注意：字段名用 CodeVal 而非 Code，因为 Go 不允许同类型字段与方法同名，
// 而 Code() 方法是 Coder 接口契约（Code() Code），不可改名，故字段让步。
type BusinessError struct {
    CodeVal Code     // apperr.CodeInvalid / apperr.CodeNotFound / ...
    Msg     string
    Cause   error
}

func (e *BusinessError) Error() string {
    if e.Cause != nil {
        return fmt.Sprintf("%s: %v", e.Msg, e.Cause)
    }
    return e.Msg
}
func (e *BusinessError) Unwrap() error   { return e.Cause }
func (e *BusinessError) Code() Code      { return e.CodeVal }

// 构造便利函数
func NewInvalid(msg string) *BusinessError {
    return &BusinessError{CodeVal: CodeInvalid, Msg: msg}
}
func NewNotFound(msg string) *BusinessError {
    return &BusinessError{CodeVal: CodeNotFound, Msg: msg}   // 需新增 CodeNotFound 常量
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
    CodeVal: apperr.CodeInvalid,
    Msg:     fmt.Sprintf("remote: invalid target %q", target),
    Cause:   err,
}
```

### 新增错误码

```go
// 通用错误码补充
const (
    CodeNotFound Code = "not_found"   // 业务资源不存在（跨模块通用）
)
```

`CodeInvalid` 已存在。`CodeNotFound` 新增。**本议题 remote service 9 处错误无 not_found 语义**（均为 invalid 或包装底层错误），`CodeNotFound`/`NewNotFound` 为后续 service 接入预留——BusinessError 体系里 not_found 是常见分类（如资源不存在），先定义避免后续再动 apperr 常量层。若希望本议题不引入未使用符号，可暂缓新增，待首个 not_found 业务场景出现时再加。

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

### Result[T] 的定位（不是 Wails 限制）

Wails App 方法支持 `(T, error)` 双值返回，项目里大多数方法用这个（`GetCharacters` / `CreateLocation` / `Chat` / `ImportNovel` 等）。`(T, error)` 模式下 error 被 Wails 序列化为前端 rejection，前端 `try/catch` 拿到的 error 基本只有 message 字符串，无法按结构化 code 分支——失败就 toast 个提示够用，但要做"GitHub 404 → 灰色刷新 / rate limit → 倒计时 / 网络错误 → 重试按钮"这类分类反馈，光有 message 字符串不够，得 string 匹配，脆弱。

`Result[T]` 单值模式就是为这类链路设计的：成功失败都 resolve，前端统一拿 `{data, err_code, err_msg}`，按 `err_code` 枚举分支做 i18n/UI。本议题 remote skill 链路是当前唯一使用 `Result[T]` 的场景（`app/skill_api.go` 3 个方法）。

所以 `Result[T]` 不是"Wails 只能返回一个值"的妥协，是"前端需要结构化错误分类"的设计选择。简单 CRUD 用 `(T, error)` 顺手；需要分类反馈的链路用 `Result[T]`。两者并存合理，本议题不统一。

## 改动清单

| 步骤 | 文件 | 内容 | 类型 |
|---|---|---|---|
| 1 | `internal/apperr/apperr.go` | 加 `Coder` 接口；`CodeFromError` 重写为 1 次 `errors.As` 提取接口；删 githubapi/llm import；删 `codeFromGitHubAPIError`/`codeFromLLMAPIError`（逻辑移到各 error 的 Code() 方法） | refactor |
| 2 | `internal/apperr/business.go` | 新增 `BusinessError` 结构体 + `NewInvalid`/`NewNotFound` 构造函数；新增 `CodeNotFound` 常量 | feat |
| 3 | `internal/githubapi/client.go` | `Error` 实现 `Code() apperr.Code` 方法（映射逻辑从 apperr 搬来） | refactor |
| 4 | `internal/llm/types.go` | `APIError` 实现 `Code() apperr.Code` 方法（映射逻辑从 apperr 搬来） | refactor |
| 5 | `internal/skill/remote/service.go` | 实际 9 处 `fmt.Errorf("remote: ...")` 分两类处理：**4 处业务校验**（`requires non-zero novelID` / `invalid target` / `invalid skill name` / `invalid skill path`）改用 `apperr.NewInvalid`；**5 处包装底层错误**（`fetch index.json` / `parse index.json` / `fetch skill` / `create skill dir` / `write skill file`）保留 `%w` 包装不改，底层 `*githubapi.Error` 经 Unwrap 链仍能被 `errors.As` 提取走领域 `Code()` | refactor |
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

### 常量归属：集中放 apperr（方法分散 vs 常量集中）

v1.3.0 的架构是**方法分散、常量集中**：各领域的 `Code()` 方法留在领域包（githubapi.Error.Code() 在 githubapi，llm.APIError.Code() 在 llm），但所有 Code 常量集中在 apperr 包按模块分区定义。这个不对称是刻意的设计选择：

- **类型依赖无法消除**：Coder 接口 `Code() Code` 的 `Code` 类型是 `apperr.Code`，领域包实现接口必须 import apperr 拿类型。即使把常量分散到各领域包，类型依赖仍在 apperr——分散常量是表面解耦，不解决根本依赖。
- **错误码本质是前端契约**：err_code 字符串值集合需要一份权威清单，供前端文档、i18n 映射、契约稳定性审查。集中定义 = 一处可见全部 err_code，避免分散后扫多包遗漏不一致。
- **"新领域零改 apperr"的核心价值已达成**：v1.2.0 的痛点是每接入新领域要加 `errors.As` 分支 + 加映射函数（逻辑代码，易错耦合重）。Coder 接口把这个根治了——新领域只在自己的 error 上实现 `Code()`，apperr 的 `CodeFromError` 一行不改。加一个 const 字符串常量不是 v1.2.0 的痛点（只是声明字符串值，无逻辑），把这个算进"违背零改 apperr"是把目标看得过重。
- **Go 惯例**：net/http 状态码常量集中在 http 包（StatusOK/StatusNotFound...），不分散到各协议处理包。apperr 集中定义 Code 常量同理。

若未来领域多到几十个各十几个码，可在 apperr 包内按模块拆子文件（如 `codes_githubapi.go` / `codes_llm.go`），仍保持集中归属，不必跨包分散。

## 性能分析

| 模式 | CodeFromError 复杂度 | 实际开销 |
|---|---|---|
| v1.2.0 集中映射 | O(n × d)，n 次 errors.As 各遍历 d 层链 | 纳秒级（n=2, d=1-3） |
| v1.3.0 接口模式 | O(d)，1 次 errors.As 提取接口 | 纳秒级 |

性能不是演进动机（差异可忽略）。真正动机是**解耦**：apperr 不依赖业务包 + 新领域零改 apperr + 根治字符串匹配。

## 不做的事

- **不改 `Result[T]` 结构**：ErrCode + ErrMsg 两字段够用，前端按 code 分支。领域字段不给前端（后端决策用）。
- **不做错误码注册机制**：Code 仍用 const 直接定义，不做 map 注册表。
- **不强制改造所有业务 service**：本议题 scope = 基础设施（Coder 接口 + BusinessError + CodeOK 不变量）+ 已使用 apperr 的 githubapi/llm/remote。其他 service 逐步接入，未接入的走 fallback=CodeInternal。
- **fallback 行为变化的实际影响面已核查为零**：v1.2.0 fallback 是 `strings.Contains(msg, "invalid") || strings.Contains(msg, "requires non-zero")` 命中 CodeInvalid，v1.3.0 改为直接 CodeInternal，理论上是行为变化。但 `apperr.Err` 的唯一调用方是 `app/skill_api.go` 的 3 个方法（ListRemoteSkills / GetRemoteSkillContent / InstallRemoteSkill），err 来源唯一是 `remote.Service`，本次改造全覆盖（4 处业务校验改 NewInvalid，5 处包装保留 %w 走领域 Code()）。其他 service（character / git / timeline / storyarc 等）用 `(T, error)` 双值返回，不经 apperr，不受影响。全仓库 grep 确认除 remote 外无其他 `fmt.Errorf` 字符串依赖 "invalid"/"requires non-zero" 约定经 apperr 出口。
- **不做错误码国际化**：ErrMsg 仍传后端原文，前端按 ErrCode 选 i18n 文案。

## 风险与权衡

1. **依赖方向反转**：githubapi/llm 从"被 apperr 依赖"变成"依赖 apperr"。需确认这两个包当前没有任何对 apperr 的间接依赖（避免循环）。v1.2.0 已确认无循环，演进时需再跑 `go build ./...` 验证。
2. **Code() 方法的归属**：githubapi.Error 的 Code() 返回 apperr.Code，语义上 githubapi 知道 apperr 的错误码常量。可接受——apperr.Code 是稳定契约，githubapi 按契约实现映射合理。
3. **BusinessError 与领域 error 的边界**：判断标准是"后端是否需要领域专属字段"。需要→领域 error；不需要→BusinessError。边界清晰但需文档化，避免新错误乱选。
4. **向后兼容**：v1.2.0 的 `codeFromGitHubAPIError`/`codeFromLLMAPIError` 被删，逻辑搬到各 error 的 `Code()` 方法。这两个函数是包内私有，无外部调用方，删除无影响。

## 测试覆盖

`apperr_test.go` 更新：

- `CodeFromError(nil)` → CodeOK
- 包装 `*githubapi.Error` 5 个 Kind → 对应 githubapi.* Code（走 Code() 方法）
- 包装 `*llm.APIError` 5 个 StatusCode → 对应 llm.* Code（走 Code() 方法）
- 包装 `*apperr.BusinessError{Code: CodeInvalid}` → CodeInvalid
- 多层 `fmt.Errorf("...: %w", err)` 仍能 `errors.As` 提取 Coder 接口
- 包装普通 `errors.New("unknown")` → CodeInternal（fallback，不再 strings.Contains）
- **CodeOK 降级**：构造 `BusinessError{Code: CodeOK}`（或某领域 error 的 Code() 误返回 CodeOK）→ `CodeFromError` 降级为 CodeInternal，不返回 CodeOK
- **旧字符串测试改写**：v1.2.0 的 `TestErr_InvalidTarget_String` / `TestErr_RequiresNonZero_String` 依赖 `strings.Contains` fallback，v1.3.0 取消字符串匹配后这两个测试失效，改写成用 `apperr.NewInvalid(...)` 构造 BusinessError 再走 `CodeFromError` 验证 CodeInvalid
- 各领域 error 的 `Code()` 方法单元测试：放各自包的 `_test`（`githubapi_test` / `llm_test`）白盒直接调 `Code()`，验证 Kind/StatusCode → Code 映射；`apperr_test` 只做黑盒集成（通过 `apperr.Err` 触发 `CodeFromError` 验证端到端）

## 关联

- v1.2.0 现有设计：[error-code-system.md](../v1.2.0/error-code-system.md)
- v1.2.0 code-review 4.2.3：[v1.2.0-code-review.md](../v1.2.0/v1.2.0-code-review.md)
- 本议题作为 v1.3.0 第一步大改造，后续业务 service 接入在此基础上逐步推进

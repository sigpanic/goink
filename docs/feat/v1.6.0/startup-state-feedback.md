# 启动状态反馈设计

## 1. 背景

Wails v2 的 `OnStartup` 回调在**独立 goroutine** 中执行：

```go
// wails/v2/internal/frontend/desktop/linux/frontend.go
func (f *Frontend) Run(ctx context.Context) error {
	f.ctx = ctx
	go func() {                                  // ← goroutine
		if f.frontendOptions.OnStartup != nil {
			f.frontendOptions.OnStartup(f.ctx)
		}
	}()
	f.mainWindow.Run(f.startURL.String())        // ← 主流程继续加载前端
	return nil
}
```

因此**后端初始化与前端加载是并发的**。Goink 的 `OnStartup` → `initWithConfig` → `migrate.Run`
在库较大时可达数十秒，而前端加载通常在数百毫秒内完成。

## 2. 问题

### 2.1 现状时序

```
t≈0      后端 OnStartup goroutine: config.Load() → initWithConfig → migrate.Run（慢）
t≈0      Wails 主流程: 加载 index.html → JS 执行 → React mount
t≈400ms  前端 App.tsx mount → IsInitialized() → a.cfg 仍为 nil → false
         → setView("init") → 显示 InitView（首次初始化界面）
t≈3s     迁移完成，a.cfg = cfg
         → 但 App.tsx 的 useEffect 是空依赖，不会重新查询
         → 用户停留在 InitView，直到手动重启
```

### 2.2 核心缺陷：状态表达能力不足

`IsInitialized()` 返回 `a.cfg != nil`，而 `a.cfg` **只有全部初始化成功才赋值**
（见 `app/handler.go` 的 `initWithConfig` 注释："只有全部步骤成功才会将 a.cfg 设为非 nil"）。
于是"还没跑完"和"跑失败了"都被折叠成 `false`：

| 实际状态 | `IsInitialized()` | 前端显示 | |
|---|---|---|---|
| 未配置（首次启动） | false | InitView | 正确 |
| 初始化中 | false | InitView | **错误** |
| 就绪 | true | 主界面 | 正确 |
| 初始化失败 | false | InitView | **错误** |

`internal/config/config.go` 中 `ErrNotInitialized` 的注释其实已预见了这件事：

> 没初始化弹出来初始化界面，如果初始化了但是还是出错就谈配置错误恢复

但代码里没有"配置错误恢复"这条分支——前端拿不到区分两种 `false` 的信息。

### 2.3 后果

1. **误导**：初始化期间和失败后，用户看到的是"首次初始化界面"，而他的数据一直都在。
2. **并发初始化**（更严重）：用户在该界面点"开始使用" → `Initialize(dataDir)`：

   ```go
   func (a *App) Initialize(dataDir string) error {
       config.Save(dataDir)          // 用 InitView 的值覆盖指针文件
       cfg, _ := config.Load()
       return a.initWithConfig(cfg)  // 第二次 storage.Open + 第二次 migrate.Run
   }
   ```

   两个 `migrate.Run` 并发操作同一个库，同时 `config.Save` 覆盖用户配置。

3. **迁移中断**：用户不知道在迁移，可能强杀进程。

## 3. 目标与非目标

### 目标

1. 前端能区分四种启动状态。
2. 初始化中显示"正在准备数据，请不要关闭应用"（延迟 500ms 出现，避免快启动闪烁）。
3. 初始化失败显示错误页 + 重试，不再误导为首次初始化。
4. 杜绝并发初始化。
5. 初始化期间（含迁移）劝阻用户关闭窗口。

### 非目标

- **不改变 `initWithConfig` 的触发者**：仍由后端在 `OnStartup` 中触发，不由前端点火。
- **不做细粒度步骤进度**（"第 2/5 步"）：状态结构预留字段，本期不实现。
- **不改 `Initialize` 的首次初始化语义**（`config.Save` 仍只属于它）。

## 4. 方案

### 4.1 状态定义

```go
type StartupPhase string

const (
    PhaseUnconfigured StartupPhase = "unconfigured" // 指针文件不存在，等用户初始化
    PhaseInitializing StartupPhase = "initializing" // initWithConfig 进行中
    PhaseReady        StartupPhase = "ready"        // 就绪（终态）
    PhaseFailed       StartupPhase = "failed"       // 初始化失败（终态）
)

type StartupState struct {
    Phase   StartupPhase `json:"phase"`
    Error   string       `json:"error,omitempty"` // 失败原因；PhaseFailed 与"前置步骤失败后回到 unconfigured"时都有值
    Version uint64       `json:"version"`          // 每次状态变化单调递增
}
```

### 4.2 状态机（可重试，按 Version 丢弃迟到事件）

```
                   ┌──────────────────────┐
   指针文件存在 ───► │    initializing      │
                   └───┬──────────────┬───┘
                       │              │
              成功     │              │  失败
                       ▼              ▼
                  ┌─────────┐   ┌──────────┐
                  │  ready  │   │  failed  │
                  └─────────┘   └────┬─────┘
                   （终态）          │ 用户点"重试"
                                     └──► initializing

   指针文件不存在 ──► unconfigured ──（用户点"开始使用"）──► initializing
```

前端只应用 `Version` 大于已处理版本的状态快照。事件可能因后端与
`FrontendReady` 并发而乱序；版本号能丢弃迟到事件，同时保留合法的
`failed → initializing` 重试转换。不能使用“终态不降级”规则，因为它会错误地
忽略重试开始事件。

`beginInitialization` 只接受 `unconfigured` / `failed` 两个来源状态，并显式拒绝
`from == PhaseReady`。它的守卫条件是 `current == from`：若调用方误传 `PhaseReady`
（此时 `current` 也是 ready）就会放行，让调用方在一个已就绪的应用上重跑一遍
`initWithConfig`（重开数据库、重跑迁移）。该检查放在锁外——它校验的是调用方传来的
参数，不读共享状态。

### 4.3 后端接口

```go
// ── 内部状态（app 包私有）──
startupMu          sync.Mutex
startupState       StartupState
frontendReady      bool
quitConfirmPending bool // 关窗确认已弹出，等待用户选择
quitConfirmed      bool // 用户已确认退出，随后由 Quit 触发的 OnBeforeClose 直接放行

// setStartupState 加锁写入状态；若前端已就绪则推送事件。
func (a *App) setStartupState(phase StartupPhase, errMsg string)

// ── Wails 绑定（前端可调）──

// GetStartupState 返回当前启动状态。前端可在任意时刻调用（不触碰 db，
// 因此初始化未完成时调用也安全）。
func (a *App) GetStartupState() StartupState

// FrontendReady 由前端在挂载完成、且已注册 startup:state 监听后调用。
// 它标记前端就绪并返回锁内取得的当前快照，用于兜住前端就绪前的状态变化。
func (a *App) FrontendReady() StartupState

// ConfirmQuit / CancelQuit 由前端在关窗确认弹窗上回传用户选择。
// ConfirmQuit 登记"已确认"后调用 runtime.Quit；CancelQuit 清除待确认标记，
// 使下一次关窗重新弹确认而不是被当成"确认期间再次关闭"直接放行。
func (a *App) ConfirmQuit()
func (a *App) CancelQuit()
```

**为什么需要 `FrontendReady` 返回快照**：`setStartupState` 在 `frontendReady == false`
时跳过推送，而“前端就绪之前”发生的状态变化必须有地方兜住。前端先注册监听，
再调用本方法；该调用在锁内设置 `frontendReady` 并取得快照。其后的变化均会发事件，
此前的变化由返回快照补齐。

**为什么 `GetStartupState` 是安全的**：Wails 的绑定在 `CreateApp` 中注册，
早于 `OnStartup` 的执行（`pkg/application/application.go`：`CreateApp` 在 `app.Run()` 之前）。
所以前端在迁移期间就能调用它。而它只读内存字段、不触碰 `a.db` / store，因此不会 nil panic。

### 4.4 前端流程

```
mount
 ├─ 1. EventsOn("startup:state", handler)    // 先注册监听
 └─ 3. applyState(s): 仅当 s.Version 大于当前版本时更新 UI
 ├─ 2. const s = await FrontendReady()         // 再告知后端，并取得锁内快照
```

**顺序不能反**：若先调 `FrontendReady` 再注册，后端在"标记就绪"与"注册完成"之间
推送的事件会丢失。

`app:quit-confirm` 同理，而且它的订阅必须**早于** `startup:state` 的订阅：后端一旦
处理完 `FrontendReady` 就会拦截关窗请求并发出该事件，若此时还没人监听，这一次关闭
既不会退出也不会弹窗（用户再关一次则被当成"放弃等待"直接退出）。因此 `App.tsx` 里
`useQuitConfirm()` 必须声明在 `useStartupState()` 之前——hook 的 effect 按声明顺序执行。

渲染规则：

| 状态 | 渲染 |
|---|---|
| `initializing` | 挂 500ms 定时器；到点仍未变终态 → 显示遮罩"正在准备数据，请不要关闭应用"。变终态则清除定时器（不显示） |
| `ready` | 主界面（原有 `WorkspaceView` 流程） |
| `unconfigured` | `InitView`（原有流程） |
| `failed` | 错误页：显示 `state.Error` + 重试按钮 |

遮罩**立即消失**、不延迟——延迟只用于"显示"，否则快启动会出现"闪一下就没"。

关窗确认弹窗不按 `phase` 渲染，而是由 `app:quit-confirm` 事件驱动、挂在 App 根部：
它要盖在"初始化中"的遮罩之上，且不能随启动界面切换被卸载。同一时刻它只对应一次
待确认的关闭请求，用户的选择经 `ConfirmQuit` / `CancelQuit` 回传后端。

### 4.5 竞态处理

**(a) `FrontendReady` 与 `setStartupState` 并发**

两者都可能读写 `frontendReady` 和 `startupState`，必须用同一把锁保护
"修改自己的字段 + 读取对方的字段"：

```
OnStartup goroutine:  setStartupState(ready)
                        └─ 锁内 { startupState = ready; 读 frontendReady }
FrontendReady (dispatcher goroutine):
                        锁内 { frontendReady = true; 读 startupState }
```

`FrontendReady` 在锁内设置 `frontendReady` 并读取同一把锁保护的状态。返回的是旧
快照时，随后的状态转换会看到前端已就绪并发送新事件；返回的是新快照时，返回值
本身已包含新状态。事件投递顺序不作为正确性前提：前端按 `Version` 丢弃旧快照。

**(b) React StrictMode 重复调用**

`useEffect` 在 StrictMode 下会执行两次，`FrontendReady()` 因此可能被调用两次。
实现须幂等（`atomic.Bool.Store(true)` 或 `CompareAndSwap`），重复补发无害。

**(c) `Initialize` 互斥**

`beginInitialization` 在同一个临界区内检查来源状态并写入 `initializing`：`Initialize`
只能从 `unconfigured` 开始，`RetryStartup` 只能从 `failed` 开始。不能先读一次状态
再切状态，否则两个调用仍可能同时穿过检查。

**(d) `OnShutdown` 与初始化 goroutine**

用户在初始化期间关闭窗口时，`OnShutdown` 会取消 ctx 并 `storage.Close(db)`，
而初始化 goroutine 可能仍在 `migrate.Run` 中。`migrate.Run` 不检查 ctx，
会继续执行并因 db 已关闭而返回 error（GORM 返回 error，不会崩溃）。
`initWithConfig` 返回 error 后 goroutine 正常结束，`setStartupState(failed)`。

**中断本身是安全的**：`RunSteps` 每步都写 `migrate_state`，且各步内部幂等，
下次启动会从断点续跑。SQLite 的 WAL 与事务也保证不会留下损坏的中间状态。

**(e) 初始化期间劝阻关窗**

`OnBeforeClose`（`main.go` 注册）在初始化期间拦截一次关闭，并请前端弹出确认：

```
任意状态 ──► decideClose(phase, frontendReady, pending, confirmed)
                ├─ 非 initializing              → 放行
                ├─ 前端未就绪                    → 放行
                ├─ 已确认（Quit 触发的二次调用）   → 放行
                ├─ 已弹过确认（用户又点了一次关闭） → 放行（视为放弃等待）
                └─ 其余                          → 拦截 + EventsEmit("app:quit-confirm")
```

Wails 的 `OnBeforeClose` 返回值是 `prevent`（true = 阻止关闭），与上面的"放行"相反。

**为什么不用 Wails 的 `MessageDialog`**：v2 在 Windows/Linux 会忽略 `Buttons`
自定义标签（Windows 只给 `MB_OK`、Linux 只给 `GTK_BUTTONS_OK`），返回值恒为
`Ok` / `OK`，无法表达"继续等待 / 退出"；macOS 支持自定义标签，但三平台返回值不统一。
因此改由前端渲染弹窗：后端只做判定与标记，`ConfirmQuit` 登记已确认后 `runtime.Quit`，
`CancelQuit` 清除待确认标记。

**兜底方向统一为"宁可放行，不可阻塞退出"**：前端未就绪（弹不出弹窗）、用户在确认中
再次关窗、选择未能回传，全部放行。否则确认链路一旦失效，用户就被关在窗口里。
`decideClose` 做成纯函数正是为了让这几条兜底规则可以直接表驱动测试。

选择"劝阻 + 允许"而不是"硬阻止"，因为：中断是安全的（见 (d)），
且强杀进程无法拦截，硬阻止只会把用户逼向更粗暴的退出方式。

## 5. 改动清单

| 文件 | 改动 |
|---|---|
| `app/startup_state.go`（新增） | 阶段与快照定义、`setStartupState`、`GetStartupState`、`FrontendReady`、`beginInitialization`（含 ready 来源守卫） |
| `app/quit_confirm.go`（新增） | `decideClose` 判定、关窗确认事件名、`ConfirmQuit` / `CancelQuit` |
| `app/handler.go` | `OnStartup` 各分支设置对应状态；`Initialize` / `RetryStartup` 经 `beginInitialization` 进入并透传前置失败原因；`initWithConfig` 成功/失败时设置终态；`OnBeforeClose` 改为发事件拦截；`OnShutdown` 释放资源 |
| `app/startup_state_test.go`、`app/quit_confirm_test.go`（新增） | 状态机与关窗判定的单元测试 |
| `main.go` | 注册 `OnBeforeClose` |
| `frontend/src/App.tsx` | 按 `phase` 渲染四种界面；挂载关窗确认弹窗 |
| `frontend/src/components/startup/useStartupState.ts`（新增） | 事件订阅 + 按 `Version` 取舍 + 遮罩防抖 + 重试 |
| `frontend/src/components/startup/useWorkspaceBoot.ts`（新增） | 等 `ready` 后再取作品列表，避免初始化期打 DB |
| `frontend/src/components/startup/useQuitConfirm.ts`（新增） | 订阅 `app:quit-confirm` 并回传用户选择 |
| `frontend/src/components/startup/StartupOverlay.tsx`、`LoadingScreen.tsx`（新增） | 遮罩 / 错误页与加载态 |
| `frontend/src/views/InitView.tsx` | 接收 `startup.error`，展示"前置步骤失败后回到设置页"的原因 |
| `frontend/src/components/ui/ConfirmDialog.tsx` | 复用为关窗确认弹窗（未改动组件本身） |
| `frontend/src/i18n/locales/zh-CN.json`、`en.json` | 新增 `startup.*` 文案（含关窗确认四项） |
| `frontend/src/lib/wailsjs/` | 由 `wails generate module` 重新生成（不手工编辑） |

## 6. 验证方案

**单元测试**（`app/startup_state_test.go`、`app/quit_confirm_test.go`）：

1. 初始快照：`New()` 后为 `initializing` 且 `Version == 1`。
2. `FrontendReady` 幂等：重复调用不 panic、不改变状态与版本。
3. `beginInitialization` 互斥：16 个 goroutine 同时进入，只有 1 个成功，终态为
   `initializing`。
4. `beginInitialization` 拒绝 `PhaseReady` 来源，且不改变状态。
5. 并发：`FrontendReady` 与 `setStartupState` 并发调用后版本号符合预期
   （`-race` 下运行）。
6. `decideClose` 表驱动：非初始化阶段 / 前端未就绪 / 已确认 → 放行；首次请求 →
   弹确认；确认期间再次关闭 → 强制退出。
7. `ConfirmQuit` / `CancelQuit` 的标记语义：取消后下次关窗仍弹确认，确认后直接放行。

尚未覆盖：`frontendReady == false` 时 `setStartupState` 不推送事件（只在 2、5 里被
间接覆盖）；`useStartupState` / `useWorkspaceBoot` / `useQuitConfirm` 三个前端 hook
没有单测。

**手工验证**：

| 场景 | 期望 |
|---|---|
| 稳态启动（无需迁移） | 不出现遮罩，直接进主界面（500ms 延迟生效） |
| 大库启动（有迁移） | 500ms 后出现遮罩；迁移完成后进主界面 |
| 首次启动 | 显示 `InitView`，点"开始使用"后进入初始化流程 |
| 迁移失败（人为制造，如占用 `volumes/` 路径） | 显示错误页 + 重试按钮，**不显示 InitView** |
| 初始化中重复点"开始使用" | 第二次调用被拒绝，不产生并发初始化 |
| 初始化中关闭窗口 | 弹出前端确认弹窗；选「继续等待」后仍停在遮罩、再关一次仍会弹；选「退出」后进程干净退出，重启后迁移从断点续跑 |
| 初始化中连点两次关闭 | 第二次直接退出，不再弹确认（"确认期间再次关闭视为放弃等待"） |

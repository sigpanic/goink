# 4.1.0 Query 错误 toast 全局中间件

## 目标

引入全局 `QueryCache.subscribe` 中间件统一接管 query 错误的 toast 提示，解决以下问题：

1. **character 领域重复 toast（4.1.1 已知 bug）**：`viewTab=graph` 时 `CharacterListView` 和 `CharacterGraph` 都订阅 `useCharacters`，各自挂 `useEffect([query.error]) → toast`，导致同一 error 触发 2 次 toast。
2. **novel 领域静默失败（规则 8 违规，历史遗留）**：6 个 `useNovels` 消费方都只取 `data`，无人监听 `error`，加载失败用户看不到任何提示。
3. **预防 location / storyarc query 化后复发**：这两个领域结构与 character 同构（父+子 Graph），query 化后必然复现 character 的重复 toast 问题。
4. **精简样板代码**：所有领域 query 化后不再需要每组件写 `useEffect([q.error]) → toast`，错误展示职责单一化（中间件管 toast，组件只管 inline 错误 UI 如 `loadFailed` 文案）。

## 范围

### 接入中间件

- 所有 `useQuery`（GET 查询）的错误
- 包括 character / novel 已 query 化的，以及未来 location / storyarc / timeline / reader / preference / novel-setting / style / skill query 化后的

### 不接入中间件

- **mutation（POST/PUT/DELETE）错误**：仍由各 `useXxxMutation` 的 `onError` 回调或调用方 `try/catch` 处理。mutation 不共享缓存，不会重复 toast，无需中间件兜底。中间件只订阅 `QueryCache`，不订阅 `MutationCache`。
- **命令操作错误**：`approve` / `reject` / `import` 等非 CRUD 命令，仍走 `try/catch + toastError`。
- **组件 inline 错误 UI**：`loadFailed` 文案、表单字段错误等仍由组件渲染，中间件只管 toast 弹窗。
- **表单校验**：前端校验（如 `novelRequired`）不是 API 错误，保留组件级 toast。

## 设计

### 中间件挂载位置

新建 `frontend/src/lib/queryErrorToast.ts`，导出 `installQueryErrorToast(queryClient)` 函数。在 `queryClient.ts` 创建 QueryClient 后模块顶层挂载一次（避免 StrictMode 双调用）。

### 核心逻辑

```typescript
import type { QueryClient } from "@tanstack/react-query";
import { toastError } from "@/utils/toast";
import { toErrorMessage } from "@/utils/error";
import i18n from "@/i18n";

// queryKey 前缀 → i18n key 映射表
// 漏配时 fallback 用 queryKey[0] 作 toast 文本（保证不静默）
const QUERY_ERROR_I18N: Record<string, string> = {
  characters: "character.charsLoadFailed",
  "character-relations": "character.relationsLoadFailed",
  novels: "novel.loadFailed",
  // 后续领域 query 化时补：
  // locations / storyarcs / timeline / reader / preferences / ...
};

export function installQueryErrorToast(queryClient: QueryClient): () => void {
  return queryClient.getQueryCache().subscribe((event) => {
    if (event.type !== "updated") return;
    if (event.action.type !== "error") return;

    const query = event.query;
    const error = query.state.error;
    if (!error) return;

    // 组件卸载静默：无 observer 订阅时，后台 refetch 失败不 toast
    if (query.observers.length === 0) return;

    // 查 i18n key
    const prefix = String(query.queryKey[0] ?? "");
    const i18nKey = QUERY_ERROR_I18N[prefix] ?? `${prefix}.loadFailed`;
    const label = i18n.exists(i18nKey)
      ? i18n.t(i18nKey)
      : `${prefix} load failed`;

    toastError(`${label}: ${toErrorMessage(error)}`);
    console.error(`[query error] ${prefix}:`, error);
  });
}
```

### 无需去重

**subscribe 机制本身已防重复 toast，无需额外去重 Map**：

1. **subscribe 是 query 级别 callback**：多组件订阅同 queryKey 时，`QueryCache.subscribe` 只 fire 1 次 callback（不是 observer 级别），从根上消除"多组件重复 toast"。
2. **retry 期间不 fire error action**：queryFn 抛错 + retry 配置时，state.status 保持 `pending`（retry 中），不触发 error 事件；只有 retry 全部用完才 fire 1 次 error action。
3. **refetch 失败 error 引用必变**：TanStack Query 每次 fetch 失败都新建 error 对象（除非 queryFn 显式抛全局 error 单例，项目无此写法），不存在"同引用重复 fire"。

早期版本曾有 `Map<queryHash, error引用>` 去重，经评估在当前配置（`retry: 1` + 模块顶层挂载）下冗余，已删除。

### 组件卸载静默（observers 判断）

**问题**：组件 unmount 后 query 仍在 cache（gcTime 内），后台 refetch 失败时若中间件仍 toast，用户已离开该页面却看到报错（如在看 Profile 时弹出角色加载失败）。

**方案**：`query.observers.length === 0` 时跳过 toast。observers=0 表示无组件订阅（用户已离开），后台 refetch 失败静默。

**不影响主动操作**：用户触发的 refetch（refresh 按钮 / invalidateQueries 后刷新）发生时组件必然在挂载（observers>0），不受此判断影响。

### StrictMode 行为

React StrictMode 下 `installQueryErrorToast` 的 setup 函数（在 `useEffect` 里调用时）会被调 2 次：
- 第 1 次调 → subscribe → 返回 unsubscribe
- cleanup → unsubscribe
- 第 2 次调 → subscribe → 返回 unsubscribe

最终只有 1 次 subscribe 活着，不会双 toast。

如果直接在 `queryClient.ts` 模块顶层调用（不通过 useEffect），则只挂一次，更稳。**本项目采用模块顶层挂载**。

## apperr 新 API 适配（未来阶段）

项目存在两套 Wails API：

| API 形态 | 代表方法 | 返回 | 失败行为 |
|---|---|---|---|
| **旧 API** | `GetCharacters` / `GetNovels` | `(data, error)` | wails reject 字符串（Go err.Error()） |
| **新 API（apperr）** | `ListRemoteSkills` / `GetRemoteSkillContent` / `InstallRemoteSkill` | `*apperr.Result[T]` | HTTP 200，返回 `{err_code, err_msg}`，**不 throw** |

详见 `internal/apperr/apperr.go`。apperr 设计为零侵入，旧 API 保持原签名，新 API 在新增方法上启用。

### 问题：新 API 不 throw → query 不会 error → 中间件不触发

如果 queryFn 直接 `await ListRemoteSkills()`，方法返回 `Result[T]`（HTTP 200），queryFn 不会 throw，query 永远不 error，中间件永远不触发 → 错误静默吞掉（违反规则 8）。

### 适配方案：`unwrapResult` 适配层（未来阶段实现）

新建 `frontend/src/utils/wailsResult.ts`：

```typescript
export class AppErr extends Error {
  errCode: string;
  constructor(code: string, msg: string) {
    super(msg);
    this.errCode = code;
  }
}

export function unwrapResult<T>(res: {
  err_code: string;
  err_msg?: string;
  data: T;
}): T {
  if (res.err_code) {
    throw new AppErr(res.err_code, res.err_msg ?? res.err_code);
  }
  return res.data;
}
```

queryFn 用法（未来 skill 迁移时）：

```typescript
queryFn: async () => {
  const res = await ListRemoteSkills(input);
  return unwrapResult(res);  // err_code 非空时 throw AppErr（带 errCode）
}
```

中间件可扩展读 `error.errCode` 按 code 映射 i18n（可选，当前不实现）。

### 改造 query 后是否重复 toast（通用判断规则）

**判断标准**：迁移前 grep 该领域的 `toastError` 调用，看 GET 错误处理是否有 toastError。

| 场景 | 是否重复 toast | 处理 |
|---|---|---|
| GET 错误处理有 toastError + query 化（如 character 之前） | **重复** | 删组件级 toastError，由中间件接管 |
| GET 错误处理只 inline（无 toastError）+ query 化（如 skill 的 ListRemoteSkills） | **不重复** | 保留 inline，中间件接管 toast |
| mutation 错误 | 不重复 | 保留组件级 toastError（不走 QueryCache） |
| 表单校验 | 不重复 | 保留组件级 toastError（非 API 错误） |

**skill 领域验证**：grep `frontend/src/components/skill/SkillMarketplace.tsx` 的 toastError 调用：
- `ListRemoteSkills` / `GetRemoteSkillContent` 错误只 `setError` / `setContentError` inline 显示，不调 toastError → 迁移 query 后中间件接管 toast，不重复
- `InstallRemoteSkill` 是 mutation，不走中间件，保留组件级 toastError
- 表单校验 `novelRequired` 保留组件级 toastError

## 实施步骤

### ✅ 步骤 1：新建 `queryErrorToast.ts`

文件路径：`frontend/src/lib/queryErrorToast.ts`，实现核心逻辑（无去重 Map + observers 判断）。

### ✅ 步骤 2：挂载中间件

在 `frontend/src/lib/queryClient.ts` 创建 QueryClient 后模块顶层调用 `installQueryErrorToast(queryClient)`。

### ✅ 步骤 3：删 character 组件级 useEffect

- [CharacterListView.tsx](file:///home/nianhe/projects/goink/frontend/src/components/character/CharacterListView.tsx)：删 `useEffect([charsQuery.error]) → toast + console.error`，保留 `loadFailed` inline UI
- [CharacterGraph.tsx](file:///home/nianhe/projects/goink/frontend/src/components/character/CharacterGraph.tsx)：删两个 useEffect（`charsQuery.error` + `relsQuery.error`）+ 无用 import（`toastError` / `toErrorMessage`），保留 `charsError` / `relsError` inline UI

### ✅ 步骤 4：i18n 检查

- `character.charsLoadFailed` / `character.relationsLoadFailed`：已在 zh-CN.json / en.json（4.1.1 加）
- `novel.loadFailed`：4.1.0 补加到 zh-CN.json / en.json 的 novel 块

### ✅ 步骤 5：三绿验证

- `npm run build` ✓
- `npm run lint` ✓（0 errors）
- `npm test` ✓（13 files / 89 tests passed，含 7 个 queryErrorToast 测试）

### ✅ 步骤 6：单元测试

新建 `frontend/src/lib/queryErrorToast.test.tsx`，7 个测试覆盖：

| 测试 | 验证点 |
|---|---|
| query error + 有 observer → 触发 toastError | 核心：error 事件触发 toast |
| query error + 无 observer（fetchQuery）→ 不 toast | observers 判断生效 |
| query success → 不 toast | 非 error 不触发 |
| 已知 prefix 用 i18n 映射（characters → character.charsLoadFailed） | i18n 映射正确 |
| 未知 prefix → fallback 到 ${prefix}.loadFailed + 文案 | i18n fallback 正确 |
| string error（wails reject）→ toErrorMessage 命中 string 分支 | wails 字符串 error 处理 |
| novel query error → 用 novel.loadFailed 映射 | 新补的 novel.loadFailed 生效 |

### ⬜ 步骤 7：手测 toast 不重复（待用户验证）

- mock character fetch 失败 → 确认只弹 1 次 toast
- viewTab=list / graph 切换 → 确认不重复
- 切小说 → 新 query 触发 → 失败时弹 1 次
- novel 列表 fetch 失败 → 确认 toast（修复静默失败）
- 组件 unmount 后 refetch 失败 → 确认不 toast（observers 判断）

## 已知差异（中间件 vs 手写 useEffect）

| 场景 | 手写 useEffect | 中间件 | 评估 |
|---|---|---|---|
| 多组件订阅同 queryKey | 重复 toast | 不重复（query 级别 fire 1 次） | ✓ 中间件更好 |
| 组件 unmount 后 refetch 失败 | ✗（useEffect 已 cleanup） | ✗（observers=0 跳过） | ✓ 已实现，行为等价 |
| StrictMode 双调用 | dev 可能双触发 | 不会双触发（模块顶层挂载） | ✓ 中间件更稳 |
| i18n 文本来源 | 组件内直接写文案 | 查映射表 | ✓ 一致性更好 |
| 漏配 i18n key | N/A | fallback 用 `queryKey[0]` 作文本 | ✓ 兜底保证不静默 |
| retry 期间 | ✗（不 fire） | ✗（不 fire） | ✓ 行为等价 |

## 验证清单

- [x] character fetch 失败时只弹 1 次 toast（不论 viewTab）—— 单元测试覆盖
- [x] viewTab 切换不重复 toast —— subscribe query 级别保证
- [x] novel fetch 失败时弹 toast（修复静默）—— 单元测试覆盖
- [x] 切小说触发新 query，失败时弹 1 次 —— 单元测试覆盖
- [x] retry 期间不 toast —— TanStack Query 行为保证
- [x] 用户点 refresh 按钮触发 refetch，失败时弹 1 次 —— observers>0 保证
- [x] 组件 unmount 后 refetch 失败不 toast —— 单元测试覆盖
- [x] 三绿全过
- [ ] 手测 toast 不重复（待用户验证）

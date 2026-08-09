# 阶段 4b：全领域 list/search 统一

> 前置条件：[阶段 4](./04-entities-batch.md) 完成（8 实体领域已 query 化）。
> 完成后：各领域 store 统一 `ListByNovel(Search, Page, Size=-1 全量, 领域filter)` 作为基础查询方法，废弃 ListAllByNovel / SearchByNovel；全局搜索覆盖全部实体；前端领域内搜索保持 filter。
> 与 [阶段 5](./05-misc-modules.md) 正交，可独立推进。

## 核心思想：ListByNovel 作为基础查询方法

`ListByNovel` 是各领域通用的基础查询方法，能力边界：
- **分页**：Page/Size，Size=-1 表示全量（GORM Limit(-1) 取消限制），Size=0 表示 Limit(0) 快速失败返回 0 条，Size>0 正常分页
- **搜索**：Search 非空时 LIKE 模糊匹配
- **排序**：Order raw string（空=领域默认排序），调用方显式传入（如 App 层传 `"name ASC"`，MCP 传 `"updated_at DESC"`），DB 层 ORDER BY，不用 App 层 sort
- **领域专属过滤**：适合 opts 的过滤（如种类/状态），传了过滤，不传不过滤
- 作为基础方法被 App 层 / MCP / searchEntities 复用

**所有领域 ListByNovel 同步加 Order 字段**（raw string，GORM 惯例，代码传参无注入面），调用方显式传入排序，不依赖默认值。

> **Order 保留约束（重要）**：重构加 Order 字段时，store 默认值（`opts.Order==""` 走的值）与所有调用方显式传入值，都必须等于重构前该路径实际采用的 Order（原硬编码值或调用方原传值）。重构是「显式化」而非「改方向」。本文档各 Commit 里写的 Order 字符串（如「MCP 传 `updated_at DESC`」）可能不准确或与最终代码不符——**以 git 基线代码中实际采用的 Order 为准**。若需改变某领域排序方向，必须先与用户讨论确认，不得在重构过程中顺手改。

**不在 ListByNovel 边界内的特殊查询**（保留独立方法）：
- 窗口切分（timeline.ListBefore / ListAfter / ListPendingBefore；storyarc.ListNodesBefore/After/PendingByArc / GetBreakpoint）
- 节点查询（storyarc.ListByArcs / ListNodesByChapterRange）
- 关系边查询（character.ListBetweenCharacters / ListCurrentByNovel；location.ListRelationsByNovel / ListRelationsInvolving / GetChildren）
- 批量按 ID（character.GetByIDs 等）
- 状态 IN 查询（storyarc.ListNonArchived）
- reader.ListActive（revealed_chapter=0 过滤）

这些特殊方法各有语义负担（limit + 排序方向 + 状态组合 + IN + 树结构），不是单纯 WHERE 过滤，不该塞 opts。

## 背景：现状混乱根源

### 前端 GetXxx 后端实现不一致

| 领域 | App 方法 | 调的 store 方法 | 模式 | 问题 |
|---|---|---|---|---|
| character | GetCharacters | `ListAllByNovel` | 全量无分页 | ListAll 是 ListByNovel(Size=-1) 的特例，冗余 |
| location | GetLocations | `ListAllByNovel` | 全量无分页 | 同上 |
| setting | GetNovelSettings | `ListSettings` | 全量无分页 | 命名不一致 |
| preference | GetPreferences | `ListGlobalPreferences + ListNovelPreferences` | 双查询合并 | 命名不一致 |
| timeline | GetTimelineEntries(from,to) | `ListByChapterRange` | 全量按章节窗口 | 前端传 0,0 全量拉，窗口能力实际未用；前端自己在内存切窗口（ENTRY_WINDOW=20） |
| storyarc | GetStoryArcs | `ListByNovel(Size:100)` | 分页取首页 100 | **截断 bug**（注释认为不会超，但有风险） |
| reader | GetReaderPerspectives | 循环 `ListByNovel` 翻页 | 循环拉全 | **低效**（循环 Size=100 拼全量） |

### 后端搜索 API 也不统一

- `ListByNovel(Search option)`：character/location（searchEntities 复用）
- `SearchByNovel` 专用方法：timeline/storyarc/chapter（仅 searchEntities 用）
- preference/setting/reader：**完全没有**搜索方法

### 全局搜索覆盖不全

`searchEntities` 只搜 5 类（character/location/timeline/storyarc/chapter），preference/setting/reader 未接入。

## 分层架构

```
前端 (wails bind)
  ↓
App 层 (app/*.go) — GetXxx 包装 ListByNovel(Size=-1) 全量
  ↓
store 层 (internal/*/store.go) — 统一 ListByNovel(Search, Page, Size=-1, 领域filter)
  ↓
DB

MCP 层 — 直接调 store.ListByNovel(Page:N, Size:M) 分页，或调特殊方法
全局搜索 — searchEntities 调各 store.ListByNovel(Search, Size:EntityLimit)
前端领域内搜索 — 自己 filter（基于全量缓存数据）
```

## 改动范围

### store 层：各领域 ListByNovel 拓展

| 领域 | ListByNovel 现状 | 改动 |
|---|---|---|
| character | 已有 Search option | 加 Order option |
| location | 已有 Search option | 加 Order option |
| storyarc | 有 ArcType/Status，无 Search | 加 Search + Order option |
| chapter | 有 Order，无 Search | 加 Search option（Order 已有） |
| reader | 有 Type，无 Search | 加 Search + Order option |
| timeline | 有 Category/Status，无 Search | 加 Search + FromChapter + ToChapter + Order（合并 ListByChapterRange + SearchByNovel） |
| setting | 只有 ListSettings（无分页无 Search） | 改名 ListByNovel + 加 Search/Page/Order |
| preference | ListGlobal/ListNovel/ListPreferences | ListGlobalPreferences + ListNovelPreferences 各加 Search + Page + Order；删 ListPreferences（死代码） |

**LIKE 字段**：
- character: name
- location: name
- storyarc: name + description
- chapter: title + summary
- reader: content + related_truth
- timeline: title + content
- setting: content + category
- preference: content + category

**废弃方法**：
- `ListAllByNovel`（character/location/chapter）→ 被 `ListByNovel(Size=-1)` 替代
- `SearchByNovel`（timeline/storyarc/chapter）→ 被 `ListByNovel(Search)` 替代
- `ListByChapterRange`（timeline）→ 被 `ListByNovel(FromChapter, ToChapter)` 替代（前端传 0,0 全量，等价原行为；MCP 用 ListBefore/After/PendingBefore 不受影响）
- `ListSettings`（setting）→ 改名 `ListByNovel`
- `ListPreferences`（preference）→ 死代码（无生产调用方），直接删

### preference 方案：分开两个 API

preference 有 `is_global` 区分（全局 + 小说专属），App 层 GetPreferences 分开调 ListGlobalPreferences + ListNovelPreferences 两次合并成 PreferenceResult{Global, Novel}。

**确认方案**：分开两个 API，各加 Search + Page
- `ListGlobalPreferences(ctx, search, page)` — 查全局偏好，加 Search + PageParams
- `ListNovelPreferences(ctx, novelID, search, page)` — 查小说专属，加 Search + PageParams
- 删 `ListPreferences`（死代码，无生产调用方）
- App 层 GetPreferences 保持调两次，合并成 PreferenceResult

**理由**：
- 分开清晰，语义明确
- SQLite WAL 模式读无锁竞争，两次查询无性能问题
- 避免指针类型（*bool 三态）增加复杂度

**全局搜索 searchEntities 搜 preference**：调两次（ListGlobalPreferences + ListNovelPreferences）合并结果

### PageParams.Normalize 改造（已完成，commit eaf448f / 276257a）

当前：`Size<1 或 Size>100` → `Size=20`。
改造为严格 GORM 语义：`Size>=0` 原样透传（0=Limit(0) 快速失败，>0 正常分页），`Size<0` 归一化为 -1（Limit(-1) 取消限制）并强制 Page=1 保证 offset=0。新增 `Offset()` 集中计算偏移量。`NewPageResult` 归一化 nil Items 为空切片 + 修正 size<0 时 TotalPages。

影响面：所有 ListByNovel 调用方确认 Size 语义。MCP 传具体 Size（如 20/50），不受影响。

### App 层 GetXxx 内部改调 ListByNovel(Size=-1)

- `GetCharacters` → `character.ListByNovel(Size=-1)`
- `GetLocations` → `location.ListByNovel(Size=-1)`
- `GetNovelSettings` → `setting.ListByNovel(Size=-1)`（保持返回 SettingResult）
- `GetTimelineEntries(from,to)` → `timeline.ListByNovel(Size=-1, FromChapter=from, ToChapter=to)`（前端 useTimelineEntries 传 0,0 全量 + 内存切窗口，行为等价；废弃 ListByChapterRange）
- `GetStoryArcs` → `storyarc.ListByNovel(Size=-1)`（修复截断 bug）
- `GetReaderPerspectives` → `reader.ListByNovel(Size=-1)`（废弃循环拉全）
- `GetPreferences` → preference 方案待定

**修复 bug**：
- storyarc `Size:100` 截断 → `Size=-1` 全量
- reader 循环拉全 → `Size=-1` 一次拉全

### searchEntities 改造（全局搜索）

`internal/search/service.go` 的 `searchEntities` 改用统一 `ListByNovel(Search, Size:EntityLimit)`：
- character/location：已用 `ListByNovel(Search)`，保持
- timeline/storyarc/chapter：从 `SearchByNovel` 改为 `ListByNovel(Search, Size:EntityLimit)`
- **新增** preference/setting/reader 三分支：
  - preference → `Type="preference"`, `PanelID="preferences"`, `Title=Content` 截断, `Subtitle=Category`
  - setting → `Type="setting"`, `PanelID="novel-settings"`, `Title=Content` 截断, `Subtitle=Category`
  - reader → `Type="reader"`, `PanelID="reader"`, `Title=Content` 截断, `Subtitle=type 中文映射`, `ChapterNum=PlantedChapter`

**Service struct** 加 `prefStore/settingStore/readerStore` 三个字段；`NewService` 多接 3 个参数；`app/handler.go` 两处 `search.NewService` 调用补传参数（含 vecStore=nil 的早期回退路径）。

### 前端

**SearchPanel**：`TYPE_CONFIG` 加 preference/setting/reader 三项（图标 `Settings`/`Globe`/`Eye`）；`GROUP_ORDER` 加三项（建议放 timeline/storyarc 之后、rag 之前）。

**i18n**：加 `search.preference="偏好"` / `search.setting="设定"` / `search.reader="读者视角"`。

#### focusStore 修订（前置工作，所有领域共用）

当前 focusStore 设计缺陷：切走再切回时 View remount → useEffect 在 mount 时跑一遍 → focusId 残留 → 触发不期望的重新定位。用户期望「仅用户重新点击搜索条目时才触发定位」。

**修订方案**：

1. **focusMap 加 nonce**：每次 `focusEntity` 写入时带 nonce，强制值变化
```ts
interface FocusEntry { id: number; nonce: number; }
interface FocusState {
  focusMap: Partial<Record<PanelId, FocusEntry>>;
  focusEntity: (panelId: PanelId, id: number) => void;
  clearFocus: (panelId: PanelId) => void;  // 新增
}
focusEntity: (panelId, id) =>
  set({ focusMap: { [panelId]: { id, nonce: Date.now() } } }),  // A 方案整体替换保持
```

2. **新增 `clearFocus(panelId)` action**：merge 语义，只删当前 panelId 的 key（不动其他面板）
```ts
clearFocus: (panelId) =>
  set((s) => {
    const next = { ...s.focusMap };
    delete next[panelId];
    return { focusMap: next };
  }),
```

3. **抽 `useFocusWithNonce` hook**：封装「订阅 + unmount cleanup」，定位逻辑各 View 自己写
```ts
export function useFocusWithNonce(panelId: PanelId) {
  const focus = useFocusStore((s) => s.focusMap[panelId]);
  const clear = useFocusStore((s) => s.clearFocus);
  useEffect(() => {
    return () => clear(panelId);  // View unmount 清自己
  }, [panelId, clear]);
  return focus;  // { id, nonce } | undefined
}
```

**不拆 selector**：写法 A 返回 store 里的 entry 引用（不是 selector 里新构造对象），引用没变就不 re-render，等值优化生效。整体替换下 focusMap 只有当前 panelId 的 key，其他面板 View 订阅返回 undefined（没被 focus 过）或已被 clearFocus 清掉，不会误触发本面板 re-render。

4. **各 View 改造**：用 `useFocusWithNonce` 替换原 `useFocusStore` 订阅，useEffect 依赖加 nonce
```tsx
const focus = useFocusWithNonce("characters");
useEffect(() => {
  if (focus && focus.id > 0) {
    // 领域专属定位逻辑：scrollIntoView / setExpandedId / setWindowCenter 等
  }
}, [focus?.id, focus?.nonce]);  // nonce 让"重新点同一条"也触发
```

**高亮规范（重要）**：定位 useEffect 若需高亮条目，**用 state 驱动 className（render 阶段声明式应用），不要命令式 `classList.add/remove`**。原因：命令式 DOM 操作的 cleanup 容易漏移 class（曾出现快速连点多条时旧高亮残留、多条同时高亮的 bug），且违背 React 声明式范式。参考 CharacterGraph 的 `selectedCharacter` + CharacterListView 的 `highlightedId`：
```tsx
const [highlightedId, setHighlightedId] = useState<number | null>(null);
useEffect(() => {
  if (!focus || focus.id <= 0) return;
  setHighlightedId(focus.id);                                    // 高亮交给 state
  document.querySelector(`[data-xxx-id="${focus.id}"]`)
    ?.scrollIntoView({ behavior: "smooth", block: "center" });  // DOM API 留 useEffect
  const timer = setTimeout(() => setHighlightedId(null), 2000);
  return () => clearTimeout(timer);                              // cleanup 只清 timer，不再碰 class
}, [focus?.id, focus?.nonce]);
// render 阶段声明式应用：
className={`... ${highlightedId === item.id ? "ring-2 ring-primary" : ""}`}
```

`scrollIntoView` 是 DOM API 必须放 useEffect（副作用），但**高亮是 UI 状态，应走 render 阶段**，让 React 管 re-render，从根上杜绝漏移 class。

**期望行为验证**：

| 场景 | 行为 |
|---|---|
| 搜点 character 5 | focusEntity 写入 {id:5, nonce:T1} → useEffect 跑 → 定位 ✓ |
| 切到 timeline（不点搜索）→ 切回 character | CharacterListView unmount → clearFocus("characters") → remount 时 focus=undefined → useEffect 跑但不定位 ✓ |
| 不切走，关搜索再开搜索点 character 5（同一条） | focusEntity 写入 {id:5, nonce:T2} → nonce 变化 → useEffect 重新跑 → 重新定位 ✓ |
| 点 character 8（不同条） | focusId 5→8 变化 → useEffect 跑 → 定位 ✓ |

#### View 定位能力（前置工作，随各领域纵切顺手补）

**已接入 focusStore 的 5 个 View**：character/location/storyarc/timeline/reader。改造点：
- 替换订阅为 `useFocusWithNonce(panelId)`
- useEffect 依赖加 `focus?.nonce`
- ReaderView 额外补 `setExpandedId(focus.id)` 自动展开条目

**未接入的 2 个 View**：preference/novel-setting。新增：
- PreferenceView：`const focus = useFocusWithNonce("preferences")`；新增 useEffect 找到对应条目 `scrollIntoView` + 高亮
- NovelSettingView：`const focus = useFocusWithNonce("novel-settings")`；同上定位 useEffect

#### 跳转复用机制

全局搜索和领域搜索**公用同一个 useEffect**：
- 全局搜索点击 result → `focusEntity(targetPanel, id) + setActivePanel(targetPanel)`
- 领域搜索点击 result → `focusEntity(currentPanel, id)`（不切面板）
- 各 View 的 useEffect 依赖 `focusMap[panelId]`，**不关心是谁写入的**，值变化就跑

领域内搜索点击 list 项时也调 `focusEntity(currentPanel, id)`，触发同一个 useEffect 定位。

**所有领域 List 组件（侧边栏列表）的列表项加 `onClick → focusEntity`**，不区分搜索/非搜索——单击列表项即触发定位，与全局搜索点击效果一致。

**领域内搜索**：保持现状（前端 `useState + useMemo filter`），不走后端。搜索字段需与后端 Search 字段一致。

## JS filter vs 后端 LIKE 差异

前端 filter 与后端 LIKE 大部分场景等价，边界 case 有细微差异：
- LIKE 的 `%` 和 `_` 是通配符，query 含这些字符时后端当通配符，前端当普通字符
- 大小写：LIKE 取决于 DB collation，JS filter 手动 toLowerCase

**对中文项目影响极小**（中文不含 `%`/`_`），可接受。

## 领域纵切强制清单（统一要求，不只看 commitX 内容）

> **重要**：每个领域纵切（不论 Commit X 是否写明）都必须完成下列前后端搜索链路的全改造。Commit 各节只按领域列**领域专属差异**（如 reader 的 type 中文映射、preference 的 is_global 拆分），下列通用要求**不重复写在每个 commit 里**，但都必须执行。

### 后端必做

1. **store 层**：`ListByNovel` 加 `Search` option（按领域 LIKE 字段，见 L85-L93）+ `Order` option（raw string，空=领域默认值）；默认值必须等于重构前该路径的硬编码 Order（**Order 保留约束，参 064363f 教训**）
2. **App 层**：`GetXxx` 改调 `ListByNovel(Size=-1, Order=原硬编码值)` 全量；显式传 Order，不依赖默认值；废弃 `ListAllByNovel` / `ListByChapterRange` / `SearchByNovel` 等冗余方法
3. **MCP 层**：分页浏览类工具的 `executeFull` 显式传 Order（保持原路径 Order）；摘要类工具（如 `get_reader_perspective`）不调 ListByNovel 则不改
4. **searchEntities**：该领域必须有搜索分支（`Type`/`PanelID`/`Title`/`Subtitle`/`ChapterNum` 按领域配置）；Service struct 加对应 store 字段；`NewService` 多接参数；`app/handler.go` 两处调用补传

### 前端必做

1. **侧边栏 List 组件**（如 `CharacterList` / `ArcList` / `TimelineList` / `ReaderList`）：
   - 原生 input → `SearchInput` 抽象组件（参 `ArcList.tsx`）
   - `useState + useMemo filter`，filter 字段与后端 Search 字段一致
   - 列表项加 `onClick → focusEntity(panelId, id)`
2. **主 View 组件**（如 `CharacterListView` / `ArcListView` / `TimelineView` / `ReaderView`）：
   - 加 `highlightedId` state（**声明式高亮**，state 驱动 className，不命令式 classList.add/remove，参 L210-L226 高亮规范）
   - 列表项加 `data-xxx-id={entry.id}` 属性（如 `data-entry-id` / `data-node-id`）
   - focus useEffect 内：领域专属定位（如 `setWindowCenter` / `setExpandedId` / `soloArc`）+ `setHighlightedId(focus.id)` + `scrollIntoView({ behavior: "smooth", block: "center" })` + `setTimeout(() => setHighlightedId(null), 2000)` + cleanup `clearTimeout`
   - 列表项 className 加 `${highlightedId === entry.id ? "ring-2 ring-primary" : ""}`
3. **SearchPanel**：`TYPE_CONFIG` 加该领域（图标 + labelKey）；`GROUP_ORDER` 加该领域；i18n 加 `search.xxx="中文名"`

### 不在通用清单内（领域专属差异，写在各 Commit 里）

- 领域有多层结构（如 storyarc 的 arc/node）→ 需要 `FocusEntry.type` 字段 + type 透传链路 + `ListNodesByNovel` 节点搜索
- 领域有 is_global 区分（如 preference）→ 拆 ListGlobal/ListNovel 两个 API
- 领域有特殊查询（如 timeline 的 ListBefore/After/PendingBefore）→ 保留独立方法

## 分阶段 commit 计划

**纵切策略**：按领域推进，每个领域完整走 store + App + searchEntities + 前端 View 定位，一个领域一个 commit。参考阶段 4 推进顺序：character → location → storyarc → timeline → reader → preference → novel-setting。

### Commit 0（前置）：focusStore 修订 + useFocusWithNonce hook

**目标**：基础设施，所有领域共用。

**改动**：
- `frontend/src/stores/useFocusStore.ts`：
  - focusMap 类型改为 `Partial<Record<PanelId, FocusEntry>>`，FocusEntry = `{ id: number; nonce: number }`
  - `focusEntity` 内部 set 时带 `nonce: Date.now()`
  - 新增 `clearFocus(panelId)` action（merge 语义，只删当前 panelId 的 key）
- 新建 `frontend/src/hooks/useFocusWithNonce.ts`：封装「订阅 + unmount cleanup」
- 各 View 替换原 `useFocusStore((s) => s.focusMap.xxx ?? 0)` 为 `useFocusWithNonce("xxx")`，useEffect 依赖加 `focus?.nonce`
- 各 View useEffect 加 `return () => clearFocus(panelId)` cleanup（实际由 hook 统一注册）

**验证**：
- `cd frontend && npm run build && npm run lint && npm run test`
- 手测：搜点 character 5 → 切到 timeline（不点搜索）→ 切回 character → 确认**不自动定位** ✓
- 手测：搜点 character 5 → 不切走，再点 character 5 → 确认**重新定位** ✓（nonce 生效）

### Commit 1：character 领域纵切（参考模板）

**目标**：跑通纵切流程模板。

**改动**：
- store 层：character.ListByNovel 加 Order option（raw string，空=默认 `updated_at DESC`），废弃 ListAllByNovel
- App 层：`GetCharacters` 改调 `ListByNovel(Size=-1, Order="name ASC")`（保持原 name 升序行为）
- MCP 层：GetCharactersTool 显式传 `Order="updated_at DESC"`（不依赖默认值）
- searchEntities：character 已接入，保持
- 前端 SearchPanel：character 已配置 TYPE_CONFIG，保持
- 前端 View 定位：CharacterListView list 模式补 focusId useEffect（scrollIntoView + 高亮），参考已有 graph 模式

**验证**：
- `go build ./...` && `go test ./internal/character/... ./app/...`
- 手测：搜点 character → 切到 characters 面板 → 确认 list 模式自动滚动 + 高亮定位

### Commit 2：location 领域纵切

**目标**：参考 character 模板，location 同构推进。

**改动**：
- store 层：location.ListByNovel 加 Order option
- App 层：`GetLocations` 改调 `ListByNovel(Size=-1, Order="name ASC")`，废弃 `ListAllByNovel`
- searchEntities：location 已接入，保持
- 前端 SearchPanel：location 已配置，保持
- 前端 View 定位：LocationListView/LocationGraph 补 focusId useEffect（如未实现）

**验证**：
- `go build ./...` && `go test ./internal/location/... ./app/...`
- 手测：搜点 location → 切到 locations 面板 → 确认定位

### Commit 3：storyarc 领域纵切

**目标**：store 加 Search + 修复 Size:100 截断 bug + 新增 node 搜索 + focusStore type 改造。

**改动**：
- store 层：`storyarc.ListByNovel` 加 Search + Order option（保持 ArcType/Status）
- store 层：新增 `ListNodesByNovel(ctx, novelID, opts ListNodesOptions)`（搜 node title+description，含 PageParams+Order）
- 废弃 `SearchByNovel`（arc 搜索改走 ListByNovel(Search)）+ `ListNodesByChapterRange`（当前无调用方需要章节范围查，死代码；per-arc 窗口切分保留 ListNodesBefore/After/PendingByArc）
- App 层：`GetStoryArcs` 改调 `ListByNovel(Size=-1, Order="importance DESC, created_at ASC")`（修复 Size:100 截断 bug，保持原排序）
- App 层：`GetArcNodes` 改调 `ListNodesByNovel(Size=-1)`，签名从 `(novelID, from, to)` 改为 `(novelID)`
- MCP：executeFull 加 `Order: "updated_at DESC"`
- searchEntities：storyarc arc 从 `SearchByNovel` 改为 `ListByNovel(Search, Size:EntityLimit)`；新增 node 分支 `ListNodesByNovel(Search, Size:EntityLimit)`，Type="arc_node"
- 前端 focusStore：FocusEntry 加 `type?: "arc"|"node"` 字段，focusEntity 签名加 type 可选参数
- 前端 ArcList：换 SearchInput + 拉 nodes + 搜 arc+node + 列表项 onClick focusEntity(type)
- 前端 ArcListView：新增 soloArc（只看目标 arc）+ highlightedNodeId（声明式高亮）+ focus useEffect 按 type 分流（arc→过滤+窗口对齐maxChapter+展开首节点；node→过滤+高亮+窗口对齐node章节+展开）+ swimlane 渲染加 data-node-id
- 前端 SearchPanel：TYPE_CONFIG + GROUP_ORDER 加 arc_node
- i18n：加 search.arcNode

**验证**：
- `go build ./...` && `go test ./internal/storyarc/... ./app/...`
- `npm run build` && `npm run lint` && `npm run test`
- 手测：storyarc 全量加载 + arc 搜索 + node 搜索 + 点击 arc 过滤只看这条 + 点击 node 高亮+滚动

### Commit 4：timeline 领域纵切（核心争议）

**目标**：合并 SearchByNovel 到 ListByNovel opts，废弃 ListByChapterRange（死代码）。

**改动**：
- store 层：`timeline.ListByNovel` 加 Search + Order（**不加 FromChapter/ToChapter**——YAGNI，前端传 0,0 全量，窗口能力实际未用；MCP 用 ListBefore/After/PendingBefore 不受影响）
- 废弃 `ListByChapterRange`（死代码，前端传 0,0 全量等价 ListByNovel(Size=-1)）+ `SearchByNovel`
- App 层：`GetTimelineEntries(from,to)` 改调 `ListByNovel(Size=-1, Order="target_chapter ASC")`（前端 useTimelineEntries 传 0,0 全量 + 内存切窗口，行为等价；from/to 参数废弃，签名改为 `GetTimelineEntries(novelID)`）
- searchEntities：timeline 从 `SearchByNovel` 改为 `ListByNovel(Search, Size:EntityLimit)`
- 前端 SearchPanel：timeline 已配置，保持
- 前端 View 定位：TimelineView 已接入 focusStore（windowCenter 对齐），改依赖加 nonce

**验证**：
- `go build ./...` && `go test ./internal/timeline/... ./app/...`
- 手测：timeline 列表加载正常（全量数据）+ 搜索 + 章节窗口切换正常

### Commit 5：reader 领域纵切

**目标**：store 加 Search + 修复循环拉全低效。

**改动**：
- store 层：`reader.ListByNovel` 加 Search + Order option（保持 Type）
- App 层：`GetReaderPerspectives` 改调 `ListByNovel(Size=-1, Order="planted_chapter ASC")`（废弃循环拉全）
- searchEntities：新增 reader 分支（`Type="reader"`, `PanelID="reader"`, `Title=Content`, `Subtitle=type 中文映射`, `ChapterNum=PlantedChapter`）
- 前端 SearchPanel：TYPE_CONFIG 加 reader（图标 `Eye`），GROUP_ORDER 加 reader
- i18n：加 `search.reader="读者视角"`
- 前端 View 定位：ReaderView 已接入 focusStore，补 `setExpandedId(focus.id)` 自动展开

**验证**：
- `go build ./...` && `go test ./internal/reader/... ./internal/search/... ./app/...`
- 手测：创建 reader 数据 → 搜索 → 点击 → 切到 reader 面板 → 自动定位 + 展开条目

### Commit 6：preference 领域纵切（新接入搜索）

**目标**：拆 ListGlobal/ListNovel + 加 Search/Page + searchEntities 加分支。

**改动**：
- store 层：`ListGlobalPreferences` + `ListNovelPreferences` 各加 Search + Page + Order；删 `ListPreferences`（死代码）
- App 层：`GetPreferences` 保持调 ListGlobalPreferences + ListNovelPreferences 两次（各加 Search/Page）
- searchEntities：新增 preference 分支（调两次合并结果；`Type="preference"`, `PanelID="preferences"`, `Title=Content`, `Subtitle=Category`）
- Service struct 加 `prefStore` 字段；`NewService` 多接 1 个参数；`app/handler.go` 两处 `search.NewService` 调用补传参数
- 前端 SearchPanel：TYPE_CONFIG 加 preference（图标 `Settings`），GROUP_ORDER 加 preference
- i18n：加 `search.preference="偏好"`
- 前端 View 定位：PreferenceView 新接入 `useFocusWithNonce("preferences")` + 定位 useEffect

**验证**：
- `go build ./...` && `go test ./internal/preference/... ./internal/search/... ./app/...`
- 手测：创建全局 + 小说专属 preference → 搜索 → 点击 → 切到 preferences 面板 → 自动定位（注意全局偏好在另一组渲染，需确认滚动逻辑能跨组定位）

### Commit 7：novel-setting 领域纵切（新接入搜索）

**目标**：ListSettings 改名 ListByNovel + 加 Search/Page。

**改动**：
- store 层：`ListSettings` → `ListByNovel` + 加 Search/Page/Order
- App 层：`GetNovelSettings` 改调 `ListByNovel(Size=-1, Order="updated_at DESC")`（保持返回 SettingResult）
- searchEntities：新增 setting 分支（`Type="setting"`, `PanelID="novel-settings"`, `Title=Content`, `Subtitle=Category`）
- Service struct 加 `settingStore` 字段；`NewService` 多接 1 个参数；`app/handler.go` 两处 `search.NewService` 调用补传参数
- 前端 SearchPanel：TYPE_CONFIG 加 setting（图标 `Globe`），GROUP_ORDER 加 setting
- i18n：加 `search.setting="设定"`
- 前端 View 定位：NovelSettingView 新接入 `useFocusWithNonce("novel-settings")` + 定位 useEffect

**验证**：
- `go build ./...` && `go test ./internal/setting/... ./internal/search/... ./app/...`
- 手测：创建 setting → 搜索 → 点击 → 切到 novel-settings 面板 → 自动定位

### Commit 8（前置基础设施）：PageParams.Normalize 改造 — ✅ 已完成

**目标**：严格 GORM 分页语义。

**改动**（commit eaf448f / 276257a）：
- `internal/storage/pagination.go`：`Normalize` 改为严格语义（`Size>=0` 原样透传，`Size<0` 归一化 -1 全量并强制 Page=1）；新增 `Offset()` 集中计算偏移量
- `NewPageResult`：nil Items 归一化为空切片（避免 JSON null）；`size<0` 有数据时 TotalPages=1
- 各领域 store 的 `offset := (pp.Page-1)*pp.Size` 统一替换为 `pp.Offset()`
- 修复 4 处全量截断 bug：GetNovels / GetWritingStats / SaveGitConfig / GetStoryArcs（Size=-1）；location_tools executeNetwork（Size=10000 → -1）
- 补测试：`pagination_test.go` 覆盖 nil→[] 和各 size 档位 TotalPages

**验证**：`go build ./...` && `go test ./internal/storage/...` — 三绿通过

## 风险评估

| 风险 | 等级 | 说明 |
|---|---|---|
| Normalize 改造影响面 | 中 | Size=-1 全量语义，所有 ListByNovel 调用方需确认。MCP 传具体 Size 不受影响 |
| preference is_global 跳转 | 中 | 全局偏好 IsGlobal=true 时 NovelID=0，搜索可命中，但跳转到当前小说 preferences 面板时全局偏好在另一组渲染，需确认滚动逻辑能跨组定位。建议 Subtitle 标注「全局」 |
| 废弃方法调用方遗漏 | 低 | go build 会暴露编译错误 |
| 前端 filter vs LIKE 差异 | 低 | 中文项目影响极小 |
| storyarc 截断修复 | 低 | Size=-1 全量，修复后数据完整 |

## 领域内搜索为何不走后端

前端都是全量拉取（数据已在 query 缓存），领域内搜索前端 filter 是合理的：
- 零延迟（内存过滤，无需 RPC）
- 数据已全量缓存
- 全局搜索必须后端（跨实体+正文+RAG），场景不同

## 阶段 4b 完成标准

- 各领域 store 统一 `ListByNovel(Search, Page, Size=-1, 领域filter)` 作为基础查询方法
- 废弃 ListAllByNovel / ListSettings / SearchByNovel / ListPreferences（死代码）
- 保留特殊查询方法（窗口/节点/关系边/批量ID 等）
- 修复 storyarc Size:100 截断 bug
- 修复 reader 循环拉全低效
- 全局搜索覆盖 8 类实体（原 5 + 新 3）
- SearchPanel 分组展示 preference/setting/reader
- 点击搜索结果跳转切面板 + 定位条目
- 前后端测试全绿
- 手测三类实体搜索 + 跳转 + 定位全通过

完成后进入 [阶段 5](./05-misc-modules.md)。

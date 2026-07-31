# 通用规范

本文件定义全改造过程通用的约束：queryKey 规范、测试原则、commit 风格、目录约定、useApp 处理纪律。所有阶段文档都遵守本规范。

## 1. queryKey 规范

> 严格按规范生成 key，避免乱写导致缓存不共享或过度失效。所有 key 常量集中在 `src/lib/queryKeys.ts`（阶段 1.3 建）。

### 1.1 命名规则

实体名复数 = 列表，单数 = 单个；父级 id 作为第二段，子级 id 作为第三段。

### 1.2 key 清单

```
列表（复数）：
["novels"]                    全局小说列表（无 novelId）
["chapters", novelId]
["characters", novelId]
["character-relations", novelId]
["locations", novelId]
["location-relations", novelId]
["storyarcs", novelId]
["arc-nodes", arcId]          子资源用父 id
["timeline", novelId]
["reader", novelId]
["preferences", novelId]
["novel-settings", novelId]
["style-samples", novelId]
["skills"]                    全局

单个（单数）：
["novel", id]
["chapter", filePath]         章节内容按文件路径
["character", id]

chat（部分走 query，流式不走）：
["sessions", novelId]         session 列表可缓存
session messages 流式数据不走 query，保持本地 state
```

### 1.3 mutation 失效规则

mutation 成功后调 `qc.invalidateQueries({ queryKey: [实体名, ...] })`。创建/删除/更新 → 失效对应列表 key；单个实体更新同时失效单数和复数两个 key。

## 2. 测试原则

> **测行为不测实现**。这是测试能否跨越重构存活的唯一标准。

### 2.1 三层区分

| 测试类型 | 何时写 | 寿命 |
|---|---|---|
| 纯行为测试（用户输入→可观察输出） | 现在写 | 永久有效 |
| 纯函数单元测试（rebuildTurns 等工具） | 现在写 | 与架构无关 |
| 架构相关测试（store 订阅、query 缓存） | 重构后写 | 架构稳定后补 |
| 实现细节测试（内部 state 值、props 数量） | **永远不写** | 必然失效 |

### 2.2 三原则

1. **测「用户能看到什么」，不测「组件内部状态是什么」**。用 `getByRole`/`getByText`/`getByLabelText`；不用 `container.querySelector` 读 DOM 结构、不直接读组件 state。
2. **测「调用什么 API」，不测「状态怎么流转」**。mock Wails 函数，断言「被调用了什么参数」；不断言中间状态机值。
3. **测「组件渲染了什么」，不测「组件接收了什么 props」**。断言 DOM 出现对应内容；不断言 `<SidePanel activePanel={...}>` 的 prop 值。

### 2.3 mock 模式

参考现有 `CharacterList.test.tsx`：用 `vi.mock("@/hooks/useApp", ...)` 只 mock 该测试用到的 Wails 函数（不全量 mock 100+），每个 mock 函数用 `vi.fn()`，`beforeEach` 里 `vi.clearAllMocks()` 并设默认返回值。

测试环境 i18n 返回 key 本身（如 `"character.noCharacters"`），断言用 key 字符串匹配。异步数据用 `screen.findByText` 等待。

### 2.4 EventsOn 订阅测试

EventsOn 订阅在测试里要 mock，避免真实事件监听泄漏：mock `@/lib/wailsjs/runtime/runtime` 的 `EventsOn` 返回一个 no-op unsub 函数，其他用到的 runtime 函数也一并 mock。

## 3. commit 风格

遵循 Conventional Commits（pre-commit hook 会校验）：`type(scope): description`。

- type ∈ feat fix docs style refactor perf test build ci chore revert
- 重构用 `refactor`，加测试用 `test`，装依赖用 `build`，建新文件用 `feat`
- description 用英文，具体，不加 emoji，不加 Co-Authored-By
- 每个 commit 只包含该步骤的改动

示例：`build(frontend): add zustand deps` / `refactor(workspace): replace negation chain` / `test(workspace): add panel tests` / `feat(novel): add useNovels query`

## 4. 目录约定（领域聚合）

> 现状：`components/` 已按领域分了 20+ 子目录（character/、novel/、location/ 等）。新代码顺应这个结构，实现真正的领域自治——一个领域文件夹内聚该领域全部代码（UI + query + store + mutation + dialogs）。

新代码布局（改造后逐步形成）：

- **领域内全部聚合到 `frontend/src/components/{domain}/`**：query、store、mutation、组件、dialogs 都放这里
  - 例：`components/novel/` 放 BookshelfView + NovelDialogs + useNovels + useNovelStore + useCreateNovel/Update/Delete
  - `components/character/` 放 CharacterListView + CharacterGraph + useCharacters + useCharacterRelations + useCharacterStore + useCreateCharacter 等
  - 改某领域只看一个目录
- **跨领域应用级 store 放 `frontend/src/stores/`**：usePanelStore、useFocusStore、useTabStore（UI 框架状态，不属于单一领域）
- **全局基建放 `frontend/src/lib/`**：queryClient、queryKeys（跨领域共享的常量/实例）
- **跨领域类型放 `frontend/src/types/`**：如 PanelId 联合类型
- **应用级页面放 `frontend/src/views/`**：InitView、WorkspaceView（应用骨架，非单一领域）
- **应用级通用 hook 留 `frontend/src/hooks/`**：useTheme、useLayoutState、useWindowState（跨领域共享）；领域相关 hook 顺势归入对应 `components/{domain}/`

原则：领域内聚到 `components/{domain}/`，跨领域的应用级状态/基建/类型才上浮到顶层 `stores/`、`lib/`、`types/`、`views/`。

## 5. useApp 处理纪律

> 详见设计文档「useApp 模式分析与废弃路径」。

- 阶段 1-4：useApp.ts 保留不动，新代码（query/store/mutation）直接 import Wails 函数，不用 useApp。
- 阶段 5 迁移时：组件迁移到 useQuery 自然不再调 `useApp()`；EventsOn 改用 `qc.invalidateQueries()`。
- 阶段 5 结束后：所有消费方迁完，删 useApp.ts。
- **顺序纪律**：先改 EventsOn 用 invalidateQueries → 再删 loadXxx → 最后删 useApp。不能跳序，否则重现丢事件 bug。

## 6. 验证清单（每步通用）

每个步骤完成后跑：

1. `npm run build`（pre-commit 自动）
2. `npm run lint`（pre-commit 自动）
3. `npm run test`（pre-commit 自动）
4. 手动验证（`wails dev`，按步骤文档的「手测点」逐项点）

注意：pre-commit hook 按 staged 文件分层触发，Go 改动跑 go build/test/golangci-lint，前端改动跑 build/lint/test，纯文档跳过。本改造只动前端。

# 阶段 4b：偏好/设定/读者接入全局搜索

> 前置条件：[阶段 4](./04-entities-batch.md) 的 4.5 reader / 4.6 preference / 4.7 novel-setting 完成，三个 View 已订阅 useFocusStore 并具备「按 id 定位」能力。
> 完成后：全局搜索覆盖 preference / novel-setting / reader 三类实体，搜索结果分组展示，点击跳转定位到具体条目。
> 与 [阶段 5.5](./05-misc-modules.md) 的关系：5.5 是前端 search 数据层对齐 query 缓存，本阶段是后端搜索范围扩展 + 前端分组展示，两者正交，互不阻塞。

## 背景

当前全局搜索（后端 [internal/search/service.go](../../../internal/search/service.go) 的 `searchEntities`）只搜 character/location/timeline/storyarc/chapter 五类实体。preference/novel-settings/reader 三类未接入，搜索结果不展示这三类条目。

前端导航链路（[useFocusStore](../../../frontend/src/stores/useFocusStore.ts) + WorkspaceView 的 `handleSearchNavigateEntity`）在 [阶段 2.8](./02-workspaceview.md) 已就绪，可处理任意 panelId 跳转。缺的是后端搜索这三类实体 + 前端 SearchPanel 分组展示。

## 前置工作（View 定位能力）

PreferenceView/NovelSettingView 当前没有「按 id 定位」能力（2.8 只删了死代码 focusId prop，未订阅 store 也无定位 useEffect）。ReaderView 已订阅 focusId 但 `setExpandedId` 未联动。搜索接入前必须先补：

- **PreferenceView**：订阅 `useFocusStore((s) => s.focusMap.preferences ?? 0)`；新增 `useEffect([focusId, items])` 找到对应条目并 `scrollIntoView` + 高亮。
- **NovelSettingView**：订阅 `useFocusStore((s) => s.focusMap["novel-settings"] ?? 0)`；同上定位 useEffect。
- **ReaderView**：在现有 focusId useEffect 里补 `setExpandedId(focusId)`，跳转后自动展开条目（当前只 `setWindowCenter` 滚到章节窗口，不展开）。

若阶段 4.5/4.6/4.7 迁移时已顺手补上，本步跳过。

## 改动文件

后端（5 文件）：
- `internal/preference/store.go` — 新增 `SearchByNovel`
- `internal/setting/store.go` — 新增 `SearchByNovel`
- `internal/reader/store.go` — 新增 `SearchByNovel`
- `internal/search/service.go` — Service struct 加 3 个 store 字段 + `NewService` 接 3 个参数 + `searchEntities` 加 3 个分支
- `app/handler.go` — 两处 `search.NewService` 调用补传 `a.preference/a.setting/a.reader`（含 vecStore=nil 的早期回退路径）

前端（2 文件 + 前置工作的 View 改动）：
- `frontend/src/components/search/SearchPanel.tsx` — `TYPE_CONFIG` 加 3 项 + `GROUP_ORDER` 加 3 项
- `frontend/src/locales/*.json` — 加 `search.preference/setting/reader` 三个 i18n key

## 怎么做

> ⚠️ **实行前先调研**：具体读 `internal/preference/store.go`、`internal/setting/store.go`、`internal/reader/store.go`，确认这三个 store 是否已提供 Search 方法或可复用的 List 方法。若已有，直接接入 `service.go`，不重复造轮子。

### 后端

1. 三个 store 各新增 `SearchByNovel(ctx, novelID, query, limit)` 方法，照 `internal/timeline/store.go` 或 `internal/storyarc/store.go` 的 `SearchByNovel` 模板抄。LIKE 字段：
   - preference：`Content` + `Category`（注意 `IsGlobal=true` 时 `NovelID=0`，需用 `is_global = ? OR novel_id = ?` 过滤，与 `ListPreferences` 一致）
   - setting：`Content` + `Category`（直接 `WHERE novel_id = ?`，v2 已取消全局设定）
   - reader：`Content` + `RelatedTruth`
2. `service.go`：Service struct 加 `prefStore/settingStore/readerStore` 三个字段；`NewService` 多接 3 个参数；`searchEntities` 在 chapter 分支后追加 3 个分支，返回 `Result{Type, ID, Title, Subtitle, PanelID}`：
   - preference → `Type="preference"`, `PanelID="preferences"`, `Title=Content`（前 30 字符截断）, `Subtitle=Category`
   - setting → `Type="setting"`, `PanelID="novel-settings"`, `Title=Content` 截断, `Subtitle=Category`
   - reader → `Type="reader"`, `PanelID="reader"`, `Title=Content` 截断, `Subtitle=type 中文映射`, `ChapterNum=PlantedChapter`
3. `handler.go` 两处 `search.NewService` 调用补参数（含 vecStore=nil 的早期回退路径，避免降级时 search 不可用）。

### 前端

4. `SearchPanel.tsx` 的 `TYPE_CONFIG` 加 preference/setting/reader 三项（图标 + labelKey，建议 `Settings`/`Globe`/`Eye`）；`GROUP_ORDER` 加三项（建议放 timeline/storyarc 之后、rag 之前）。
5. i18n 加 `search.preference="偏好"` / `search.setting="设定"` / `search.reader="读者视角"`。
6. 若前置工作的 View 订阅/定位未在阶段 4 完成，本步补。

## 验证

- `go build ./...` + `go test ./internal/search/... ./internal/preference/... ./internal/setting/... ./internal/reader/...`
- `cd frontend && npm run build && npm run lint && npm run test`
- 手测：创建含特定关键词的 preference/setting/reader 数据 → 搜索框输入关键词 → 确认三类结果分别展示在 preference/setting/reader 分组下 → 点击结果确认切面板 + 定位/高亮对应条目。

## 风险

- **中**：preference 全局偏好 `IsGlobal=true` 时 `NovelID=0`，搜索可命中，但跳转到当前小说的 preferences 面板时全局偏好在另一组渲染（PreferenceView 拆 global/novelPrefs 两组），需确认滚动逻辑能跨组定位（可能需展开全局区域）。建议搜索结果 Subtitle 标注「全局」字样，或 Title 前加 `[全局]` 前缀。
- **低**：后端 `SearchByNovel` 纯 LIKE 查询，模式成熟（照 timeline/storyarc）。
- **低**：`service.go` 加分支不影响现有 5 个实体搜索路径，互不耦合。
- **低**：`handler.go` 的 `NewService` 签名变化，漏改会编译失败暴露。

## commit

建议拆两个 commit（前后端解耦，可独立验证）：

1. `feat(search): add preference/setting/reader stores to SearchAll`（后端）
2. `feat(search): add preference/setting/reader groups to SearchPanel`（前端）

## 阶段 4b 完成标准

- preference/novel-setting/reader 三类实体出现在全局搜索结果中
- 三类结果在 SearchPanel 分组展示，点击跳转切面板 + 定位到具体条目
- 后端 `searchEntities` 覆盖 8 类实体（原 5 + 新 3）
- 前后端测试全绿
- 手测三类实体搜索 + 跳转 + 定位全通过

完成后进入 [阶段 5](./05-misc-modules.md)。

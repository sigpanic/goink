# 统一容器 Tab 模型设计

## 一、背景与问题

### 现状

当前 ContentPanel 的 tab 是单层结构，每个 tab 直接渲染一个视图，通过 `viewMode` 字段在多个视图间切换：

- 章节 tab：`viewMode` 在 `content`（正文 Monaco）/ `outline`（大纲只读 Markdown）间切换
- skill tab：`viewMode` 在 `preview`（SkillPreview）/ `edit`（SkillEditForm 表单）间切换
- goink.md tab：`viewMode` 在 `content`（Monaco）/ `preview`（Markdown 预览）间切换

### 问题

当章节需要支持「大纲手动编辑」时，暴露了单层 tab + viewMode 切换的本质缺陷：

1. **一个 tab 对应多个文件时被迫特殊化**
   章节正文存 `chapters/NNN.md`，大纲存 `outlines/NNN.md`，是两个独立文件。硬塞进一个章节 tab 后，必须区分：
   - 内容字段：`tab.content` vs `tab.outlineContent`
   - dirty 字段：`tab.isDirty` vs `tab.outlineIsDirty`
   - 保存路径：`tab.path` vs 派生 `outlinePath(chapterNum)`

2. **保存逻辑被迫分流**
   `handleEditorChange`（正文）与 `handleOutlineEditorChange`（大纲）几乎重复，只差 path 派生。`doSave` 要加 `dirtyKey` 参数区分重置哪个 dirty 字段。`savingRef` 要带 `dirtyKey` 标记。

3. **不通用**
   每增加一个「同 tab 多文件」场景（如未来章节设定表单、角色卡等），都要新增一套 dirty 字段 + 分流逻辑，技术债累积。

### 根因

单层 tab 模型无法表达「一个逻辑实体（章节/skill）包含多个独立可编辑视图」的结构。viewMode 切换适合「一个文件多个视图」（如 skill 的预览/编辑），但不适合「多个独立文件聚合」（如章节的正文+大纲）。

## 二、目标：统一容器模型

### 核心思路

把 tab 升级为**容器**，容器内挂多个**子 tab**，每个子 tab 是独立的可编辑单元（有自己的 path/content/isDirty/renderer），完全复用现有保存机制，零特殊化。

```
[章节 tab] (容器)
  ├─ [正文子tab] → Monaco, path=chapters/001.md
  ├─ [大纲子tab] → Monaco, path=outlines/001.md
  └─ [设定表单子tab] → 表单 (未来扩展)
```

### 通用性

同一套容器模型容纳所有文件编辑场景：

| tab 类型 | 形态 | 子 tab |
|---|---|---|
| 章节 | 容器 | 正文 Monaco / 大纲 Monaco / 设定表单（未来） |
| skill | 容器 | 预览 Markdown / 编辑表单 / 源码 Monaco（未来） |
| goink.md | 容器 | 预览 Markdown / 编辑 Monaco |
| diff | 叶子 | 无（保持现状，renderer=diff） |

## 三、数据模型设计

### EditorTab 扩展

在现有 `EditorTab` 基础上加 `children` 字段区分容器与叶子：

```ts
type EditorTab = {
  id: string;
  title: string;
  type: "file" | "diff";
  // 容器字段：有 children 即为容器
  children?: EditorTab[];
  activeChildId?: string;  // 容器记忆当前激活子 tab
  // 叶子字段：容器无这些
  path?: string;
  content?: string;
  isDirty?: boolean;
  viewMode?: "preview" | "edit" | "source";
  renderer?: "monaco" | "markdown" | "skill-form" | "diff";
  readOnly?: boolean;
  // diff tab 专用
  diff?: string;
  original?: string;
  modified?: string;
  changeType?: string;
  reason?: string;
  toolId?: string;
};
```

### 判定规则

- `children` 存在且非空 → **容器**：渲染子 TabBar + 子 tab，顶层无自身页面
- `children` 不存在 → **叶子**：直接渲染 content（现状不变）

### 子 tab 的独立性

每个子 tab 拥有完整的叶子字段（path/content/isDirty/viewMode/renderer），与现有单层 tab 完全同构。这意味着：

- `handleEditorChange(tabId, value)` 通用（tabId 可以是子 tab id）
- `doSave(tabId, path, content)` 通用（path 来自子 tab 自身）
- `savingRef` 通用（id/path/content/dirtyKey 都来自子 tab，且 dirtyKey 可移除，因为子 tab 各自有 isDirty）
- Ctrl+S / 失焦保存 / file:changed 监听只需定位到「当前激活的子 tab」

**消除 dirtyKey/outlineIsDirty 特殊化**：正文和大纲都是子 tab，各有 isDirty，不再需要分流字段。

## 四、渲染逻辑

### ContentPanel 渲染分层

```
ContentPanel
  ├─ 外层 TabBar（章节/skill/goink.md/diff tab）
  └─ activeTab 渲染
      ├─ 若 activeTab.children 非空（容器）
      │   ├─ 子 TabBar（正文/大纲/...）
      │   └─ activeChild 渲染（按 renderer 分发）
      │       ├─ monaco → ContentEditor
      │       ├─ markdown → Markdown / SkillPreview
      │       ├─ skill-form → SkillEditForm
      │       └─ diff → DiffEditor
      └─ 否则（叶子）
          └─ 按 renderer 分发（现状不变）
```

### 子 TabBar

复用现有 `TabBar` 组件，传入 `activeTab.children` 作为 tabs，`activeTab.activeChildId` 作为 activeTabId。子 tab 也可关闭（关闭章节的某个子 tab）。

## 五、useEditorTabs 改造

### API 扩展

现有 API 保留，新增子 tab 操作：

- `openChild(parentId, child: EditorTab)`：向容器添加子 tab
- `closeChild(parentId, childId)`：移除子 tab
- `updateChild(parentId, childId, patch)`：更新子 tab
- `setActiveChild(parentId, childId)`：切换激活子 tab

### activeTab 定位

`activeTab` 仍指外层 tab。渲染时若 activeTab 是容器，再取 `activeTab.children.find(c => c.id === activeTab.activeChildId)` 作为「当前渲染的叶子」。

### 快捷键与 savingRef

Ctrl+S / Ctrl+Shift+V / 失焦保存都操作「当前激活的叶子」（可能是子 tab）。`savingRef.current` 存叶子 tab 的 {id, path, content}，不再需要 dirtyKey。

### file:changed 监听

遍历所有外层 tab，若是容器再遍历其 children，比对 path。命中后更新对应子 tab 的 content 并重置 isDirty。

### 持久化

localStorage 存嵌套结构（含 children）。加版本号字段，旧数据（扁平结构）走迁移函数或清空（用户丢失 tab 记忆，可接受）。

## 六、迁移路径（分阶段）

### 阶段 1：模型通用化 + 章节迁移（本轮目标）

1. `EditorTab` 加 `children` / `activeChildId` / `renderer` 字段
2. `useEditorTabs` 加子 tab 操作 API
3. `TabBar` 支持二级渲染（子 TabBar）
4. `ContentPanel` 渲染分层（容器 vs 叶子）
5. 章节打开时自动建正文 + 大纲子 tab，激活正文
6. skill / goink.md / diff 保持现状（无 children，仍走 viewMode 切换）
7. localStorage 加版本迁移

**收益**：消除大纲编辑的 dirtyKey/outlineIsDirty 特殊化，章节正文/大纲对称处理。

### 阶段 2：skill 迁移（后续迭代）

- skill tab 变容器，children: [预览子tab(Markdown), 编辑子tab(skill-form)]
- SkillEditForm 的 onSave 适配子 tab（path 来自子 tab）
- 移除 skill 的 viewMode=preview/edit 分支

### 阶段 3：goink.md 迁移（后续迭代）

- goink.md tab 变容器，children: [预览子tab(Markdown), 编辑子tab(Monaco)]
- 移除 goink.md 的 viewMode 分支

### 阶段 4：diff tab 评估（可选）

评估 diff 是否也容器化（当前保持叶子，renderer=diff）。若 diff 审批流程需要更复杂展示，可后续纳入。

## 七、风险点与缓解

### 高风险

1. **diff tab 归属**
   当前 diff 是顶层 tab，`handleDiffApprove` 的 `closeTab` + `doOpenFile` 切回文件 tab。容器化后 diff 属于哪个外层？
   - **缓解**：diff 保持顶层叶子（不纳入子 tab），只让章节/skill/goink.md 容器化。降低 diff 相关逻辑改动。

2. **localStorage 迁移**
   现有 `goink_tabs_all` 存扁平结构，升级后格式不兼容。
   - **缓解**：加版本号字段，旧数据走迁移函数（把扁平 tab 包成单子 tab 容器）或直接清空。tab 记忆丢失可接受。

3. **快捷键定位**
   Ctrl+S/Ctrl+Shift=V 当前监听全局 tabs，子 tab 引入后要精确定位「当前激活子 tab」。
   - **缓解**：`savingRef` 已通用，只要子 tab 也能设 savingRef。定位逻辑用「外层 activeTab + 内层 activeChildId」两级查找。

4. **file:changed 遍历两层**
   - **缓解**：抽 `walkTabs(tabs, cb)` 工具函数递归遍历容器与叶子，统一处理。

### 中风险

- **activeTab 双层记忆**：切换外层 tab 时内层 active 要恢复记忆（每个容器记自己的 activeChildId）
- **doOpenFile 去重**：当前按 path 在顶层去重，容器化后要在子 tab 层去重
- **TabBar 二级布局**：窄屏可能拥挤，需设计紧凑形态

## 八、与方案 Y 的关系

当前已实现的「方案 Y」（viewMode=outline-edit + dirtyKey/outlineIsDirty）是**过渡方案**，能用但有特殊化技术债：

- 引入 `dirtyKey` 字段做分流
- `outlineIsDirty` 与 `isDirty` 双轨
- `handleOutlineEditorChange` 与 `handleEditorChange` 重复

容器化阶段 1 落地后，方案 Y 的改动应**回退**：

- 移除 `outlineIsDirty` 字段
- 移除 `handleOutlineEditorChange`
- 移除 `doSave` 的 `dirtyKey` 参数
- 移除 `savingRef.dirtyKey`
- 移除 viewMode 的 `outline-edit` 值
- 大纲编辑改由「大纲子 tab」承担，复用 ContentEditor + handleEditorChange + doSave

## 九、决策记录

- **2026-08-14**：确认走统一容器模型，分阶段迁移。本轮只做阶段 1（章节容器化），skill/goink.md 后续迭代。本轮暂不动代码（风险偏高，需充分设计评审）。
- 方案 Y 作为过渡保留，待阶段 1 落地后回退。

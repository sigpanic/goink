# 主题系统设计

## 架构

```
[data-theme="light"] / [data-theme="dark"]   ← <html> 属性，唯一的主题开关
        │
        ▼
index.css  ─  每个 [data-theme] 定义一组 CSS 变量（--background / --foreground / ...）
        │
        ▼
Tailwind @theme inline  ─  --color-xxx: var(--xxx)  把变量暴露为 utility class
        │
        ▼
组件  ─  className="bg-background text-foreground"  只写语义 class
        │
        ▼
useTheme()  ─  读/写 [data-theme]，MutationObserver 跨组件同步
```

主题不写死两态。`[data-theme="sepia"]` 只需加一个 CSS block，组件零改动。

## 配色层

### 1. 语义 UI 色（index.css）

全站框架色。所有组件默认用这套：

| 变量 | 用途 |
|---|---|
| `--background` | 页面底色 |
| `--foreground` | 主文字 |
| `--card` | 卡片/浮层底色 |
| `--muted` / `--muted-foreground` | 次级底色/文字 |
| `--border` | 边框/分隔线 |
| `--primary` | 品牌强调（按钮、链接、图标） |
| `--destructive` | 删除/错误操作 |
| `--success` / `--success-foreground` / `--success-border` | 成功/已完成 |
| `--warning` / `--warning-foreground` / `--warning-border` | 警告/需注意 |
| `--danger-bg` / `--danger-border` | 危险操作底色 |
| `--sidebar-*` | 侧栏专用 |

浅色：白色系，primary 紫色 `oklch(0.546 0.245 262.881)`。
深色：GitHub 风格蓝灰，`--background: oklch(0.157 0.012 258)`，`--foreground: 0.88`（比纯白略收敛）。

### 2. Tag 指示色（index.css）

5 组，替换 `bg-{color}-50 text-{color}-600`：

| 变量 | 语义 |
|---|---|
| `--tag-blue` / `--tag-blue-foreground` | 进行中/未回收 |
| `--tag-green` / `--tag-green-foreground` | 已完成/已回收 |
| `--tag-amber` / `--tag-amber-foreground` | 悬念/伏笔 |
| `--tag-rose` / `--tag-rose-foreground` | 误解 |
| `--tag-purple` / `--tag-purple-foreground` | 故事弧线 |

浅色：淡底（0.97）+ 深前景，还原 Tailwind 的 `-50`/`-600` 搭配。
深色：半透明底（chroma / 0.15）+ 亮前景。

### 3. Tool 卡片色（index.css）

4 组，替换 ToolCallCard.css 的硬编码 oklch：

| 变量 | 用途 |
|---|---|
| `--tool-blue` / `--tool-blue-border` | 执行中 |
| `--tool-amber` / `--tool-amber-border` | 审批等待 |
| `--tool-green` / `--tool-green-border` | 完成 |
| `--tool-red` / `--tool-red-border` | 失败 |

浅色：淡底 + 饱和边框。深色：半透明底 + 亮边框。

### 4. 气泡色（index.css）

| 变量 | 用途 |
|---|---|
| `--bubble-user` / `--bubble-user-foreground` | User 消息气泡 |

浅色：与 `--primary` 一致，品牌紫底白字。
深色：`primary` 的 20% 透明度，前景色与 assistant 气泡对齐（`var(--foreground)`），避免大面积亮蓝色块。

### 5. 图渲染色（graphColors.ts）

g6/cytoscape 不接受 CSS 变量，JS 维护 `Record<Theme, Colors>`：

| Theme | bg | edge | edgeDim | dimFill | dimText |
|---|---|---|---|---|---|
| light | `#fafbfc` | `#3b82f6` | `#cbd5e1` | `#f1f5f9` | `#94a3b8` |
| dark | `#161b22` | `#58a6ff` | `#30363d` | `#21262d` | `#8b949e` |

详见 `frontend/src/components/graphColors.ts`。

### 6. PALETTE（数据可视化弧线色）

用于 StoryArcGraph 和 ArcListView 区分不同故事弧线。两套常量：

```ts
const PALETTE_LIGHT = [{ fill: '#dbeafe', stroke: '#3b82f6', text: '#1d4ed8', edge: '#60a5fa' }, ...]
const PALETTE_DARK  = [{ fill: 'oklch(0.58 0.15 255 / 0.15)', stroke: 'oklch(0.72 0.15 255)', ... }, ...]
```

组件通过 `useTheme().theme` 选组：`PALETTE = { light: PALETTE_LIGHT, dark: PALETTE_DARK }[theme]`。

### 7. 不动的东西

- `text-red-500` / `text-red-600` / `text-rose-500` 错误消息 — 功能色，深浅可读
- ContextRing 状态环 `#e74c3c` / `#f39c12` / `#52c41a` — 独立色系
- Highlight.js 语法着色 — 独立色系

## 新增一个主题

1. `index.css`：复制 `[data-theme="dark"]` block，改名为 `[data-theme="xxx"]`，调色值
2. `hooks/useTheme.ts`：`THEMES` 数组加 `'xxx'`，`NEXT` 字典加一行
3. `components/graphColors.ts`：`C` 字典加 `xxx: { ... }` 一组色
4. 弧线组件中 PALETTE 字典加 `xxx: PALETTE_XXX`
5. WorkspaceView 主题切换按钮改成下拉（或加三态切换）
6. `index.html` 的防闪脚本无需改动（`resolveTheme` 会从 localStorage 读取）

组件零改动。

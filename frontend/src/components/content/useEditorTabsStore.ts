import { create } from "zustand";
import {
  chapterNumFromPath,
  isContentPath,
  outlinePath,
  type EditorTab,
} from "@/components/content/types";

// useEditorTabsStore: 编辑器 tab 集的全局内存单例 + 持久化。
//
// 背景（issue #46）：ContentPanel 在切换面板时会整体 unmount，之前 useEditorTabs 用
// useState 本地状态 + 仅 beforeunload 写 localStorage，导致：
//  1. 会话内切走面板再回来，tab 列表丢失（恢复的是上次关应用时的旧 tab 集）；
//  2. 滚动位置（Monaco viewState / 大纲 scrollTop）无记忆；
//  3. ContentEditor 无 key，所有 tab 共用一个 Monaco 实例，滚动位置被共享。
//
// 方案：普通 zustand store（内存单例，unmount 不丢状态）+ 原生 localStorage 手写读写
// （debounced + beforeunload 批写 + 版本号字段）。
// - 不持久化 content/isDirty（跨重启重新 fetch，与现状一致）、diff tab（临时审批流程）。
// - 位置按 `novelId:path:mode` 键控：tabId 每次会话重新生成，不能用作持久化键；
//   path 取各 mode 的实际文件路径（正文=chapters/NNN.md，大纲=outlines/NNN.md），
//   天然契合未来统一容器 tab 模型（子 tab 即独立 path）。schema 变更走版本号迁移。

const STORAGE_KEY = "goink_tabs_all";
const STORAGE_VERSION = 1;
const PERSIST_DEBOUNCE_MS = 300;

let idSeq = 0;
function nextId(type: EditorTab["type"]): string {
  return `${type}_${++idSeq}`;
}

// TabMeta：持久化的 tab 字段（不含运行时 content / diff 等）。
type TabMeta = Pick<
  EditorTab,
  "path" | "title" | "type" | "viewMode" | "readOnly"
>;

// ReadingPosition：单个 (novelId, path, mode) 的阅读位置。
// - viewState：Monaco editor.saveViewState() 输出（content / outline-edit 模式）
// - scrollTop：大纲 Markdown 视图滚动位置（outline 模式）
export interface ReadingPosition {
  scrollTop?: number;
  viewState?: unknown;
  updatedAt: number;
}

interface NovelTabs {
  tabs: EditorTab[];
  activeTabId: string | null;
}

interface EditorTabsStoreState {
  byNovel: Record<string, NovelTabs>;
  positions: Record<string, ReadingPosition>;
  openTab: (
    novelId: number,
    tab: Omit<EditorTab, "id"> & { id?: string },
  ) => string;
  closeTab: (novelId: number, id: string) => void;
  closeAllTabs: (novelId: number) => void;
  setActiveTabId: (novelId: number, id: string) => void;
  updateTab: (novelId: number, id: string, patch: Partial<EditorTab>) => void;
  openDiffTab: (
    novelId: number,
    data: {
      path: string;
      title: string;
      diff: string;
      original: string;
      modified: string;
      changeType: string;
      reason: string;
      toolId: string;
    },
  ) => string;
  setPosition: (key: string, pos: ReadingPosition) => void;
}

// ── 持久化 ├─ 序列化 ─────────────────────────────────────────

type PersistedNovel = { tabs: TabMeta[]; activePath: string | null };
type PersistedEnvelope = {
  version: number;
  byNovel: Record<string, PersistedNovel>;
  positions: Record<string, ReadingPosition>;
};

function toPersisted(state: EditorTabsStoreState): PersistedEnvelope {
  const byNovel: Record<string, PersistedNovel> = {};
  for (const [key, entry] of Object.entries(state.byNovel)) {
    const metas: TabMeta[] = entry.tabs
      .filter((t) => t.type !== "diff")
      .map((t) => ({
        path: t.path,
        title: t.title,
        type: t.type,
        viewMode: t.viewMode,
        readOnly: t.readOnly,
      }));
    if (metas.length === 0) continue;
    const activeTab = entry.tabs.find((t) => t.id === entry.activeTabId);
    byNovel[key] = {
      tabs: metas,
      activePath:
        activeTab && activeTab.type !== "diff" ? activeTab.path : null,
    };
  }
  return {
    version: STORAGE_VERSION,
    byNovel,
    positions: state.positions,
  };
}

// ── 持久化 ├─ 反序列化（版本不匹配/解析失败 → 空）─────────────

const EMPTY_BYNOVEL: Record<string, NovelTabs> = {};

function loadInitial(): {
  byNovel: Record<string, NovelTabs>;
  positions: Record<string, ReadingPosition>;
} {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return { byNovel: EMPTY_BYNOVEL, positions: {} };
    const data = JSON.parse(raw) as PersistedEnvelope;
    if (!data || data.version !== STORAGE_VERSION) {
      return { byNovel: EMPTY_BYNOVEL, positions: {} };
    }
    const byNovel: Record<string, NovelTabs> = {};
    for (const [key, entry] of Object.entries(data.byNovel || {})) {
      const tabs: EditorTab[] = (entry.tabs || [])
        .filter((t) => t && t.type !== "diff")
        .map((t) => ({ ...t, id: nextId(t.type) }));
      if (tabs.length === 0) continue;
      let activeTabId: string | null = null;
      if (entry.activePath) {
        const active = tabs.find((t) => t.path === entry.activePath);
        if (active) activeTabId = active.id;
      }
      if (!activeTabId) activeTabId = tabs[0].id;
      byNovel[key] = { tabs, activeTabId };
    }
    return { byNovel, positions: data.positions || {} };
  } catch {
    return { byNovel: EMPTY_BYNOVEL, positions: {} };
  }
}

const initial = loadInitial();

// ── 持久化 ├─ debounce + beforeunload 批写 ───────────────────

let persistTimer: ReturnType<typeof setTimeout> | null = null;

function schedulePersist() {
  if (persistTimer) clearTimeout(persistTimer);
  persistTimer = setTimeout(flushPersist, PERSIST_DEBOUNCE_MS);
}

function flushPersist() {
  if (persistTimer) {
    clearTimeout(persistTimer);
    persistTimer = null;
  }
  try {
    localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify(toPersisted(useEditorTabsStore.getState())),
    );
  } catch {
    /* ignored */
  }
}

// withNovel：对指定小说条目做局部更新（无条目时按空集起步）。
function withNovel(
  key: string,
  updater: (entry: NovelTabs) => NovelTabs,
): (s: EditorTabsStoreState) => Pick<EditorTabsStoreState, "byNovel"> {
  return (s) => ({
    byNovel: {
      ...s.byNovel,
      [key]: updater(s.byNovel[key] ?? { tabs: [], activeTabId: null }),
    },
  });
}

// positionKeysForTab：该 tab 关闭时需要清理的所有位置键。
// 与 ContentPanel.positionKeyFor 的键名规则保持一致：
//   - 正文：`novelId:chapters/NNN.md:content`
//   - 大纲/大纲编辑：`novelId:outlines/NNN.md:outline` / `:outline-edit`
function positionKeysForTab(
  novelId: number,
  tab: Pick<EditorTab, "path">,
): string[] {
  const ns = `${novelId}:`;
  const keys = new Set<string>([`${ns}${tab.path}:content`]);
  // chapters/NNN.md（goink.md 除外）才有对应的大纲文件键
  if (isContentPath(tab.path) && tab.path !== "goink.md") {
    const outlineP = outlinePath(chapterNumFromPath(tab.path));
    keys.add(`${ns}${outlineP}:outline`);
    keys.add(`${ns}${outlineP}:outline-edit`);
  }
  return [...keys];
}

export const useEditorTabsStore = create<EditorTabsStoreState>((set, get) => ({
  byNovel: initial.byNovel,
  positions: initial.positions,

  openTab: (novelId, tab) => {
    const key = String(novelId);
    const id = tab.id ?? nextId(tab.type);
    const existing = get().byNovel[key]?.tabs.find(
      (t) => t.path === tab.path && t.type === tab.type,
    );
    if (existing) {
      set(withNovel(key, (e) => ({ ...e, activeTabId: existing.id })));
      return existing.id;
    }
    const newTab: EditorTab = { ...tab, id };
    set(
      withNovel(key, (e) => ({
        ...e,
        tabs: [...e.tabs, newTab],
        activeTabId: id,
      })),
    );
    return id;
  },

  closeTab: (novelId, id) => {
    const key = String(novelId);
    const entry = get().byNovel[key];
    if (!entry || entry.tabs.length === 0) return;
    const closing = entry.tabs.find((t) => t.id === id);
    set((s) => {
      const nextByNovel = withNovel(key, (e) => {
        if (e.tabs.length <= 1) return { tabs: [], activeTabId: null };
        const idx = e.tabs.findIndex((t) => t.id === id);
        const next = e.tabs.filter((t) => t.id !== id);
        let activeTabId = e.activeTabId;
        if (e.activeTabId === id) {
          const newIdx = Math.min(idx, next.length - 1);
          activeTabId = next[newIdx]?.id ?? null;
        }
        return { tabs: next, activeTabId };
      })(s);
      const positions = { ...s.positions };
      if (closing) {
        for (const k of positionKeysForTab(novelId, closing)) {
          delete positions[k];
        }
      }
      return { byNovel: nextByNovel.byNovel, positions };
    });
  },

  closeAllTabs: (novelId) => {
    const key = String(novelId);
    const prefix = `${novelId}:`;
    set((s) => {
      const next = { ...s.byNovel };
      delete next[key];
      const positions: Record<string, ReadingPosition> = {};
      for (const [k, v] of Object.entries(s.positions)) {
        if (!k.startsWith(prefix)) positions[k] = v;
      }
      return { byNovel: next, positions };
    });
  },

  setActiveTabId: (novelId, id) => {
    set(withNovel(String(novelId), (e) => ({ ...e, activeTabId: id })));
  },

  updateTab: (novelId, id, patch) => {
    set(
      withNovel(String(novelId), (e) => ({
        ...e,
        tabs: e.tabs.map((t) => (t.id === id ? { ...t, ...patch } : t)),
      })),
    );
  },

  openDiffTab: (novelId, data) => {
    const key = String(novelId);
    const id = nextId("diff");
    set(
      withNovel(key, (e) => ({
        ...e,
        tabs: [...e.tabs, { id, type: "diff", ...data }],
        activeTabId: id,
      })),
    );
    return id;
  },

  setPosition: (key, pos) =>
    set((s) => ({ positions: { ...s.positions, [key]: pos } })),
}));

useEditorTabsStore.subscribe(() => schedulePersist());

if (typeof window !== "undefined") {
  window.addEventListener("beforeunload", flushPersist);
}

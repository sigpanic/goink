import { useCallback, useRef } from "react";
import type { EditorTab } from "@/components/content/types";
import { useEditorTabsStore } from "./useEditorTabsStore";

// useEditorTabs: useEditorTabsStore 的按小说选择器薄封装，API 与旧实现一致。
// 状态存全局 zustand 单例（ContentPanel 切面板 unmount 后不丢失，会话内恢复）+ 原生
// localStorage 持久化（跨重启恢复），持久化细节见 useEditorTabsStore。
const EMPTY_TABS: EditorTab[] = [];

export function useEditorTabs(novelId: number) {
  const key = String(novelId);
  const tabs = useEditorTabsStore((s) => s.byNovel[key]?.tabs ?? EMPTY_TABS);
  const activeTabId = useEditorTabsStore(
    (s) => s.byNovel[key]?.activeTabId ?? null,
  );
  const activeTab = tabs.find((t) => t.id === activeTabId) ?? null;
  // store 在模块加载时同步从 localStorage 恢复，首次渲染即有 tab，故恒为 true。
  // 保留 initRef 仅因 ContentPanel 现有代码解构引用它。
  const initRef = useRef(true);

  const openTab = useCallback(
    (tab: Omit<EditorTab, "id"> & { id?: string }) =>
      useEditorTabsStore.getState().openTab(novelId, tab),
    [novelId],
  );
  const closeTab = useCallback(
    (id: string) => useEditorTabsStore.getState().closeTab(novelId, id),
    [novelId],
  );
  const closeAllTabs = useCallback(
    () => useEditorTabsStore.getState().closeAllTabs(novelId),
    [novelId],
  );
  const setActiveTabId = useCallback(
    (id: string) => useEditorTabsStore.getState().setActiveTabId(novelId, id),
    [novelId],
  );
  const updateTab = useCallback(
    (id: string, patch: Partial<EditorTab>) =>
      useEditorTabsStore.getState().updateTab(novelId, id, patch),
    [novelId],
  );
  const openDiffTab = useCallback(
    (data: {
      path: string;
      title: string;
      diff: string;
      original: string;
      modified: string;
      changeType: string;
      reason: string;
      toolId: string;
    }) => useEditorTabsStore.getState().openDiffTab(novelId, data),
    [novelId],
  );

  return {
    tabs,
    activeTab,
    activeTabId,
    openTab,
    closeTab,
    closeAllTabs,
    setActiveTabId,
    updateTab,
    openDiffTab,
    initRef,
  };
}

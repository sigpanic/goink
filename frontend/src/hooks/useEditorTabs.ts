import { useState, useCallback, useEffect, useRef } from "react";
import type { EditorTab } from "@/components/content/types";

let idSeq = 0;
function nextId(prefix: string) {
  return `${prefix}_${++idSeq}`;
}

type TabMeta = Pick<
  EditorTab,
  "path" | "title" | "type" | "viewMode" | "readOnly"
>;

export function useEditorTabs(novelId: number) {
  const [tabs, setTabs] = useState<EditorTab[]>([]);
  const [activeTabId, setActiveTabId] = useState<string | null>(null);
  const prevNovelIdRef = useRef(novelId);
  const allMetasRef = useRef<Record<string, TabMeta[]>>({});
  const initRef = useRef(false);
  const activeTabIdRef = useRef(activeTabId);
  useEffect(() => {
    activeTabIdRef.current = activeTabId;
  }, [activeTabId]);

  // 启动加载：从 localStorage 恢复
  useEffect(() => {
    try {
      const raw = localStorage.getItem("goink_tabs_all");
      if (raw) allMetasRef.current = JSON.parse(raw);
    } catch {
      /* ignored */
    }
    const key = String(novelId);
    const saved = allMetasRef.current[key];
    if (saved?.length) {
      const restored: EditorTab[] = saved.map((t) => ({
        ...t,
        id: nextId(t.type === "diff" ? "diff" : "file"),
      }));
      setTabs(restored);
      setActiveTabId(restored[0].id);
    }
    initRef.current = true;
    // eslint-disable-next-line react-hooks/exhaustive-deps -- novelId changes are handled by the dedicated switch effect below
  }, []);

  // beforeunload 时保存到 localStorage
  useEffect(() => {
    function save() {
      try {
        localStorage.setItem(
          "goink_tabs_all",
          JSON.stringify(allMetasRef.current),
        );
      } catch {
        /* ignored */
      }
    }
    window.addEventListener("beforeunload", save);
    return () => window.removeEventListener("beforeunload", save);
  }, []);

  // novelId 变化：切换标签集
  useEffect(() => {
    if (!initRef.current) return;
    const oldKey = String(prevNovelIdRef.current);
    const newKey = String(novelId);
    if (oldKey === newKey) return;

    prevNovelIdRef.current = novelId;
    const saved = allMetasRef.current[newKey];
    if (saved?.length) {
      const restored: EditorTab[] = saved.map((t) => ({
        ...t,
        id: nextId(t.type === "diff" ? "diff" : "file"),
      }));
      setTabs(restored);
      setActiveTabId(restored[0].id);
    } else {
      setTabs([]);
      setActiveTabId(null);
    }
  }, [novelId]);

  // tabs 变化时更新内存缓存（不立即写磁盘，beforeunload 时统一写）
  useEffect(() => {
    if (!initRef.current) return;
    const key = String(prevNovelIdRef.current);
    const metas: TabMeta[] = tabs.map((t) => ({
      path: t.path,
      title: t.title,
      type: t.type,
      viewMode: t.viewMode,
      readOnly: t.readOnly,
    }));
    if (metas.length > 0) {
      allMetasRef.current[key] = metas;
    } else {
      delete allMetasRef.current[key];
    }
  }, [tabs]);

  const activeTab = tabs.find((t) => t.id === activeTabId) ?? null;

  const openTab = useCallback(
    (tab: Omit<EditorTab, "id"> & { id?: string }) => {
      const id = tab.id ?? nextId(tab.type);
      setTabs((prev) => {
        const existing = prev.find(
          (t) => t.path === tab.path && t.type === tab.type,
        );
        if (existing) {
          setActiveTabId(existing.id);
          return prev;
        }
        return [...prev, { ...tab, id }];
      });
      setActiveTabId(id);
    },
    [],
  );

  const closeAllTabs = useCallback(() => {
    setTabs([]);
    setActiveTabId(null);
  }, []);

  const closeTab = useCallback((id: string) => {
    setTabs((prev) => {
      if (prev.length <= 1) {
        setActiveTabId(null);
        return [];
      }
      const idx = prev.findIndex((t) => t.id === id);
      const next = prev.filter((t) => t.id !== id);
      if (activeTabIdRef.current === id) {
        const newIdx = Math.min(idx, next.length - 1);
        setActiveTabId(next[newIdx].id);
      }
      return next;
    });
  }, []);

  const updateTab = useCallback((id: string, patch: Partial<EditorTab>) => {
    setTabs((prev) => prev.map((t) => (t.id === id ? { ...t, ...patch } : t)));
  }, []);

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
    }) => {
      const id = nextId("diff");
      setTabs((prev) => [...prev, { id, type: "diff", ...data }]);
      setActiveTabId(id);
      return id;
    },
    [],
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

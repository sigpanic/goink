import { useCallback, useEffect, useRef } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { EventsOn } from "@/lib/wailsjs/runtime/runtime";
import { contentKeys } from "@/lib/queryKeys";
import { GetContent } from "@/lib/wailsjs/go/app/App";
import { toastError } from "@/utils/toast";
import { toErrorMessage } from "@/utils/error";
import { useEditorTabsStore } from "./useEditorTabsStore";
import { isContentConflict, type EditorSaveQueue } from "./editorSaveQueue";
import type { EditorTab } from "./types";
import type { app } from "@/lib/wailsjs/go/models";

type DirtyKey = "isDirty" | "outlineIsDirty";
type UpdateTab = (tabId: string, patch: Partial<EditorTab>) => void;
export interface ConflictSnapshot {
  diskContent: string;
  localContent: string;
  eventVersion: number;
}
export type ConflictChoiceResult =
  { status: "resolved" } | { status: "changed"; snapshot: ConflictSnapshot };
interface Options {
  novelId: number;
  tabs: EditorTab[];
  queue: EditorSaveQueue;
  fetchContent: (novelId: number, path: string) => Promise<string>;
  updateTab: UpdateTab;
  mutateSaveContent: (input: app.SaveContentInput) => Promise<unknown>;
}

function fields(outline: boolean) {
  return outline
    ? {
        content: "outlineContent" as const,
        base: "outlineContentBase" as const,
        dirty: "outlineIsDirty" as const,
        conflict: "outlineContentConflict" as const,
      }
    : {
        content: "content" as const,
        base: "contentBase" as const,
        dirty: "isDirty" as const,
        conflict: "contentConflict" as const,
      };
}

export function useEditorFileSync({
  novelId,
  tabs,
  queue,
  fetchContent,
  updateTab,
  mutateSaveContent,
}: Options) {
  const qc = useQueryClient();
  const { t } = useTranslation();
  const novelIdRef = useRef(novelId);
  const tabsRef = useRef(tabs);
  const eventVersionsRef = useRef(new Map<string, number>());
  useEffect(() => {
    novelIdRef.current = novelId;
    tabsRef.current = tabs;
  }, [novelId, tabs]);

  const scheduleSave = useCallback(
    (
      tabId: string,
      path: string,
      content: string,
      expectedContent: string,
      dirtyKey: DirtyKey,
    ) => {
      queue.schedule(
        { novelId, tabId, path, content, dirtyKey, expectedContent },
        (expected) =>
          mutateSaveContent({
            novel_id: novelId,
            path,
            content,
            expected_content: expected,
          }),
        (written) =>
          updateTab(tabId, {
            [dirtyKey === "isDirty" ? "contentBase" : "outlineContentBase"]:
              written,
          }),
        () => updateTab(tabId, { [dirtyKey]: false }),
        (error) => {
          if (isContentConflict(error)) {
            updateTab(tabId, {
              [dirtyKey === "isDirty"
                ? "contentConflict"
                : "outlineContentConflict"]: true,
            });
            return;
          }
          toastError(t("common.saveFailed") + ": " + toErrorMessage(error));
          console.error(error);
        },
      );
    },
    [queue, novelId, mutateSaveContent, updateTab, t],
  );

  const protectDraft = useCallback(
    (tab: EditorTab, path: string, outline: boolean): boolean => {
      const latest =
        useEditorTabsStore
          .getState()
          .byNovel[String(novelIdRef.current)]?.tabs.find(
            (item) => item.id === tab.id,
          ) ?? tab;
      const keys = fields(outline);
      if (!latest[keys.dirty] && !queue.isPending(novelIdRef.current, path))
        return false;
      queue.pause(novelIdRef.current, path);
      updateTab(tab.id, { [keys.conflict]: true });
      return true;
    },
    [queue, updateTab],
  );

  const loadConflictSnapshot = useCallback(
    async (
      tabId: string,
      path: string,
      outline: boolean,
    ): Promise<ConflictSnapshot> => {
      const currentNovelId = novelIdRef.current;
      queue.pause(currentNovelId, path);
      await queue.waitForIdle(currentNovelId, path);
      const eventKey = `${currentNovelId}:${path}`;
      let diskContent = "";
      let eventVersion = 0;
      for (let attempt = 0; attempt < 3; attempt++) {
        const before = eventVersionsRef.current.get(eventKey) ?? 0;
        diskContent = await GetContent(currentNovelId, path);
        eventVersion = eventVersionsRef.current.get(eventKey) ?? 0;
        if (before === eventVersion) break;
        if (attempt === 2) throw new Error(t("content.conflictChangedAgain"));
      }
      const tab = useEditorTabsStore
        .getState()
        .byNovel[String(currentNovelId)]?.tabs.find(
          (item) => item.id === tabId,
        );
      if (!tab) throw new Error(t("content.conflictTabClosed"));
      return {
        diskContent,
        localContent: tab[fields(outline).content] ?? "",
        eventVersion,
      };
    },
    [queue, t],
  );

  const chooseConflictVersion = useCallback(
    async (
      tabId: string,
      path: string,
      outline: boolean,
      choice: "disk" | "local",
      shown: ConflictSnapshot,
    ): Promise<ConflictChoiceResult> => {
      const currentNovelId = novelIdRef.current;
      const latest = await loadConflictSnapshot(tabId, path, outline);
      if (
        latest.diskContent !== shown.diskContent ||
        latest.localContent !== shown.localContent ||
        latest.eventVersion !== shown.eventVersion
      ) {
        return { status: "changed", snapshot: latest };
      }
      const keys = fields(outline);
      queue.discard(currentNovelId, path);
      if (choice === "disk" || shown.localContent === shown.diskContent) {
        qc.setQueryData(
          contentKeys.detail(currentNovelId, path),
          shown.diskContent,
        );
        updateTab(tabId, {
          [keys.content]: shown.diskContent,
          [keys.base]: shown.diskContent,
          [keys.dirty]: false,
          [keys.conflict]: false,
        });
        return { status: "resolved" };
      }
      scheduleSave(
        tabId,
        path,
        shown.localContent,
        shown.diskContent,
        keys.dirty,
      );
      if (await queue.flush(currentNovelId, path)) {
        const afterSave = await loadConflictSnapshot(tabId, path, outline);
        if (
          afterSave.diskContent !== shown.localContent ||
          afterSave.localContent !== shown.localContent ||
          afterSave.eventVersion !== shown.eventVersion
        ) {
          updateTab(tabId, {
            [keys.dirty]: afterSave.diskContent !== afterSave.localContent,
            [keys.conflict]: true,
          });
          return { status: "changed", snapshot: afterSave };
        }
        updateTab(tabId, { [keys.conflict]: false });
        return { status: "resolved" };
      }
      const afterFailure = await loadConflictSnapshot(tabId, path, outline);
      if (
        afterFailure.diskContent !== shown.diskContent ||
        afterFailure.localContent !== shown.localContent ||
        afterFailure.eventVersion !== shown.eventVersion
      ) {
        return { status: "changed", snapshot: afterFailure };
      }
      throw new Error(t("common.saveFailed"));
    },
    [loadConflictSnapshot, queue, qc, scheduleSave, updateTab, t],
  );

  const refreshApprovedFile = useCallback(
    async (tab: EditorTab, path: string, viewMode: "content" | "outline") => {
      const outline = viewMode === "outline";
      if (protectDraft(tab, path, outline)) return;
      try {
        const fresh = await fetchContent(novelIdRef.current, path);
        const current = useEditorTabsStore
          .getState()
          .byNovel[String(novelIdRef.current)]?.tabs.find(
            (item) => item.id === tab.id,
          );
        if (!current || protectDraft(current, path, outline)) return;
        const keys = fields(outline);
        updateTab(tab.id, {
          viewMode,
          [keys.content]: fresh,
          [keys.base]: fresh,
          [keys.dirty]: false,
        });
      } catch {
        /* ignored */
      }
    },
    [fetchContent, protectDraft, updateTab],
  );

  // ── file:changed 事件监听 ─────────────────────────────────
  // 用 ref 读取最新 tabs，避免因 tabs 变化频繁重建订阅丢失事件
  useEffect(() => {
    const unsub = EventsOn(
      "file:changed",
      async (data: { novel_id: number; path: string }) => {
        if (data.novel_id !== novelIdRef.current) return;
        const eventKey = `${data.novel_id}:${data.path}`;
        eventVersionsRef.current.set(
          eventKey,
          (eventVersionsRef.current.get(eventKey) ?? 0) + 1,
        );
        const currentTabs =
          useEditorTabsStore.getState().byNovel[String(data.novel_id)]?.tabs ??
          tabsRef.current;
        for (const tab of currentTabs) {
          if (tab.type !== "file") continue;
          const outline =
            tab.outlinePath === data.path && tab.path !== data.path;
          if (tab.path !== data.path && !outline) continue;
          if (protectDraft(tab, data.path, outline)) continue;
          const keys = fields(outline);
          try {
            // 5.2 commit 3: 改 qc.invalidateQueries + fetchContent（走 query 缓存通道，不经 useApp）。
            // 先 invalidate 标 stale，再 fetchContent 才会重新拉取（否则 fetchQuery 返回旧缓存）。
            qc.invalidateQueries({
              queryKey: contentKeys.detail(data.novel_id, data.path),
            });
            const fresh = await fetchContent(data.novel_id, data.path);
            const current = useEditorTabsStore
              .getState()
              .byNovel[String(data.novel_id)]?.tabs.find(
                (item) => item.id === tab.id,
              );
            if (!current || protectDraft(current, data.path, outline)) continue;
            updateTab(tab.id, {
              [keys.content]: fresh,
              [keys.base]: fresh,
              [keys.dirty]: false,
            });
          } catch {
            /* 文件可能被删 */
          }
        }
      },
    );
    return () => unsub();
  }, [qc, fetchContent, updateTab, protectDraft]);

  return {
    scheduleSave,
    loadConflictSnapshot,
    chooseConflictVersion,
    refreshApprovedFile,
  };
}

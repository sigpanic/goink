import { useCallback, useEffect, useRef } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { contentKeys } from "@/lib/queryKeys";
import { aiFileVersion, subscribeAIFileChanges } from "./aiFileChanges";
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
        stale: "outlineNeedsRefresh" as const,
      }
    : {
        content: "content" as const,
        base: "contentBase" as const,
        dirty: "isDirty" as const,
        conflict: "contentConflict" as const,
        stale: "contentNeedsRefresh" as const,
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
  const refreshesRef = useRef(new Set<string>());
  useEffect(() => {
    novelIdRef.current = novelId;
  }, [novelId]);

  const readLatestContent = useCallback(
    async (
      path: string,
      fresh = false,
    ): Promise<{ content: string; version: number }> => {
      const currentNovelId = novelIdRef.current;
      let version = aiFileVersion(currentNovelId, path);
      let content: string;
      try {
        content = fresh
          ? await GetContent(currentNovelId, path)
          : await fetchContent(currentNovelId, path);
      } catch (error) {
        if (version === aiFileVersion(currentNovelId, path)) throw error;
        content = await GetContent(currentNovelId, path);
      }
      while (version !== aiFileVersion(currentNovelId, path)) {
        version = aiFileVersion(currentNovelId, path);
        content = await GetContent(currentNovelId, path);
      }
      return { content, version };
    },
    [fetchContent],
  );

  const applyLoadedContent = useCallback(
    (
      tabId: string,
      path: string,
      outline: boolean,
      loaded: { content: string; version: number },
    ): boolean => {
      const currentNovelId = novelIdRef.current;
      if (aiFileVersion(currentNovelId, path) !== loaded.version) return false;
      const current = useEditorTabsStore
        .getState()
        .byNovel[String(currentNovelId)]?.tabs.find(
          (item) => item.id === tabId,
        );
      const keys = fields(outline);
      if (
        !current ||
        (outline ? current.outlinePath : current.path) !== path ||
        current[keys.dirty] ||
        queue.isPending(currentNovelId, path)
      )
        return false;
      updateTab(tabId, {
        [keys.content]: loaded.content,
        [keys.base]: loaded.content,
        [keys.dirty]: false,
        [keys.conflict]: false,
        [keys.stale]: false,
      });
      return true;
    },
    [queue, updateTab],
  );

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
      let diskContent = "";
      let eventVersion = 0;
      for (let attempt = 0; attempt < 3; attempt++) {
        const before = aiFileVersion(currentNovelId, path);
        diskContent = await GetContent(currentNovelId, path);
        eventVersion = aiFileVersion(currentNovelId, path);
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
          [keys.stale]: false,
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
        updateTab(tabId, {
          [keys.conflict]: false,
          [keys.stale]: false,
        });
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
    async (
      tab: EditorTab,
      path: string,
      viewMode: "content" | "outline",
      activate = false,
    ) => {
      const outline = viewMode === "outline";
      const refreshKey = `${tab.id}:${path}`;
      if (refreshesRef.current.has(refreshKey)) return;
      refreshesRef.current.add(refreshKey);
      try {
        while (true) {
          if (protectDraft(tab, path, outline)) return;
          const loaded = await readLatestContent(path, true);
          const current = useEditorTabsStore
            .getState()
            .byNovel[String(novelIdRef.current)]?.tabs.find(
              (item) => item.id === tab.id,
            );
          if (!current || protectDraft(current, path, outline)) return;
          if (aiFileVersion(novelIdRef.current, path) !== loaded.version)
            continue;
          const applied = applyLoadedContent(tab.id, path, outline, loaded);
          if (activate && applied) {
            updateTab(tab.id, { viewMode });
          }
          return;
        }
      } catch {
        /* ignored */
      } finally {
        refreshesRef.current.delete(refreshKey);
      }
    },
    [applyLoadedContent, protectDraft, readLatestContent, updateTab],
  );

  useEffect(() => {
    return subscribeAIFileChanges(({ novelId: changedNovelId, path }) => {
      if (
        changedNovelId !== novelIdRef.current &&
        !path.startsWith("~/.goink/skills/")
      )
        return;
      const currentTabs =
        useEditorTabsStore.getState().byNovel[String(novelIdRef.current)]
          ?.tabs ?? [];
      for (const tab of currentTabs) {
        if (tab.type !== "file") continue;
        if (tab.path === path) {
          void refreshApprovedFile(tab, path, "content");
        } else if (tab.outlinePath === path) {
          void refreshApprovedFile(tab, path, "outline");
        }
      }
    });
  }, [refreshApprovedFile]);

  useEffect(() => {
    for (const tab of tabs) {
      if (tab.type !== "file") continue;
      if (tab.contentNeedsRefresh)
        void refreshApprovedFile(tab, tab.path, "content");
      if (tab.outlineNeedsRefresh && tab.outlinePath)
        void refreshApprovedFile(tab, tab.outlinePath, "outline");
    }
  }, [tabs, refreshApprovedFile]);

  return {
    scheduleSave,
    loadConflictSnapshot,
    chooseConflictVersion,
    refreshApprovedFile,
    readLatestContent,
    applyLoadedContent,
  };
}

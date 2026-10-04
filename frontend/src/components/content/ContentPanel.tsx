import {
  useState,
  useEffect,
  useCallback,
  useRef,
  forwardRef,
  useImperativeHandle,
} from "react";
import { type OnMount, DiffEditor } from "@monaco-editor/react";
import { FileText, Loader2 } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { toastError } from "@/utils/toast";
import { toErrorMessage } from "@/utils/error";
import { useEditorTabs } from "./useEditorTabs";
import { useEditorTabsStore } from "./useEditorTabsStore";
import { useNovelStore } from "@/components/novel/useNovelStore";
import { useEditorStore } from "@/stores/useEditorStore";
import { useThemeStore, type Theme } from "@/stores/useThemeStore";
import { chapterKeys } from "@/lib/queryKeys";
import { aiFileVersion } from "./aiFileChanges";
import { GetChapters } from "@/lib/wailsjs/go/app/App";
import type { chapter } from "@/lib/wailsjs/go/models";
import TabBar from "./TabBar";
import ContentEditor from "./ContentEditor";
import OutlineViewer from "./OutlineViewer";
import SkillPreview from "./SkillPreview";
import { useFileContent } from "./useFileContent";
import { useSaveContent } from "./useSaveContent";
import { getEditorSaveQueue } from "./editorSaveQueue";
import ContentConflictNotice from "./ContentConflictNotice";
import { useEditorFileSync } from "./useEditorFileSync";
import SkillEditForm from "@/components/skill/SkillEditForm";
import Markdown from "@/components/Markdown";
import {
  isContentPath,
  isOutlinePath,
  isStandaloneMarkdownPath,
  isSkillPath,
  isVolumeOutlinePath,
  skillNameFromPath,
  sourceFromPath,
} from "./types";
import type { EditorTab } from "./types";
import "./ContentPanel.css";

const MONACO_THEME: Record<Theme, string> = { light: "light", dark: "vs-dark" };

export interface ContentPanelHandle {
  openFile: (
    path: string,
    title: string,
    readOnly?: boolean,
    initialViewMode?: string,
  ) => void;
  openFileWithHighlight: (
    path: string,
    title: string,
    matchPos: number,
    matchLen: number,
  ) => void;
  clearHighlight: () => void;
  closeAllTabs: () => void;
  openDiffTab: (data: {
    path: string;
    title: string;
    diff: string;
    original: string;
    modified: string;
    changeType: string;
    reason: string;
    toolId: string;
  }) => void;
  handleDiffApprove: (toolId: string) => Promise<void>;
  handleDiffReject: (toolId: string) => Promise<void>;
}

// 3.8 后续：onContentChange/onDirtyChange 删，activeContent/isDirty 迁 useEditorStore。
// ContentPanel 直接调 useEditorStore.getState().setActiveContent/setIsDirty，StatusBar 自己订阅。
const ContentPanel = forwardRef<ContentPanelHandle>(
  function ContentPanel(_props, ref) {
    // 3.8: novelId 从 useNovelStore 订阅（替代 prop）。切小说时 store 变化触发 re-render，行为等价。
    const novelId = useNovelStore((s) => s.activeNovelId);
    const { t } = useTranslation();
    const qc = useQueryClient();
    // 5.2 commit 1: GetContent 走 query 缓存通道（fetchContent），直接 import wailsjs 不经 useApp。
    // 5.2 commit 2: SaveContent 走 useSaveContent mutation（onSuccess 失效 contentKeys.detail），useApp 清零。
    const { fetchContent } = useFileContent();
    const saveContentMutation = useSaveContent();
    const mutateSaveContent = saveContentMutation.mutateAsync;
    const editorSaveQueue = getEditorSaveQueue(qc);
    const {
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
    } = useEditorTabs(novelId);

    const { theme } = useThemeStore();
    const [isLoading, setIsLoading] = useState(false);
    const editorRef = useRef<Parameters<OnMount>[0] | null>(null);
    const pendingHighlightRef = useRef<{
      matchPos: number;
      matchLen: number;
    } | null>(null);
    const didApplyHighlightRef = useRef(false); // handleEditorMount 已应用高亮时跳过清除
    const novelIdRef = useRef(novelId);
    const activeTabRef = useRef(activeTab);
    activeTabRef.current = activeTab;

    const {
      scheduleSave: scheduleEditorSave,
      loadConflictSnapshot,
      chooseConflictVersion,
      refreshApprovedFile,
      readLatestContent,
      applyLoadedContent,
    } = useEditorFileSync({
      novelId,
      tabs,
      queue: editorSaveQueue,
      fetchContent,
      updateTab,
      mutateSaveContent,
    });

    useEffect(() => {
      novelIdRef.current = novelId;
    }, [novelId]);

    useEffect(() => {
      if (activeTab?.type === "file") {
        // 3.8 后续：activeContent 迁 useEditorStore，StatusBar 自己订阅。
        // 大纲编辑态用 outlineContent 作为当前活动内容
        const c =
          activeTab.viewMode === "outline-edit"
            ? (activeTab.outlineContent ?? "")
            : (activeTab.content ?? "");
        useEditorStore.getState().setActiveContent(c);
      }
    }, [activeTab]);

    useEffect(() => {
      // 3.8 后续：isDirty 迁 useEditorStore，StatusBar 自己订阅。
      // 大纲编辑态用 outlineIsDirty 反映脏状态
      const isDirty =
        activeTab?.viewMode === "outline-edit"
          ? (activeTab.outlineIsDirty ?? false)
          : (activeTab?.isDirty ?? false);
      useEditorStore.getState().setIsDirty(isDirty);
    }, [activeTab?.isDirty, activeTab?.outlineIsDirty, activeTab?.viewMode]);

    const resolveChapter = useCallback(
      async (path: string): Promise<chapter.Chapter | undefined> => {
        if (!path.startsWith("chapters/") && !isOutlinePath(path))
          return undefined;
        const chapters = await qc.fetchQuery({
          queryKey: chapterKeys.list(novelId),
          queryFn: () => GetChapters(novelId),
        });
        const matching = chapters?.find(
          (item) => item.file_path === path || item.outline_file_path === path,
        );
        if (matching) return matching;
        await qc.invalidateQueries({ queryKey: chapterKeys.list(novelId) });
        const refreshed = await qc.fetchQuery({
          queryKey: chapterKeys.list(novelId),
          queryFn: () => GetChapters(novelId),
        });
        return refreshed?.find(
          (item) => item.file_path === path || item.outline_file_path === path,
        );
      },
      [qc, novelId],
    );

    // ── 大纲内容加载（handleSetViewMode 与恢复 tab effect 共用） ──

    const loadOutlineContent = useCallback(
      (tab: EditorTab) => {
        if (
          tab.type !== "file" ||
          !isContentPath(tab.path) ||
          tab.path === "goink.md"
        ) {
          return;
        }
        const outline = tab.outlinePath
          ? Promise.resolve(tab.outlinePath)
          : resolveChapter(tab.path).then((item) => {
              if (item)
                updateTab(tab.id, { outlinePath: item.outline_file_path });
              return item?.outline_file_path;
            });
        outline
          .then((path) =>
            path && novelIdRef.current === novelId
              ? readLatestContent(path).then((loaded) => ({ path, loaded }))
              : undefined,
          )
          .then((result) => {
            if (result)
              applyLoadedContent(tab.id, result.path, true, result.loaded);
          })
          .catch(() => {
            const current = useEditorTabsStore
              .getState()
              .byNovel[String(novelId)]?.tabs.find((item) => item.id === tab.id);
            if (
              tab.outlinePath &&
              current?.outlineContent == null &&
              !current?.outlineNeedsRefresh
            )
              applyLoadedContent(tab.id, tab.outlinePath, true, {
                content: "",
                version: aiFileVersion(novelId, tab.outlinePath),
              });
          });
      },
      [novelId, readLatestContent, resolveChapter, updateTab, applyLoadedContent],
    );

    // 从 localStorage 恢复 tab 后，自动加载文件内容
    const loadedRef = useRef<Set<string>>(new Set());
    useEffect(() => {
      // novelId 变化时重置
      loadedRef.current.clear();
    }, [novelId]);
    useEffect(() => {
      if (!initRef.current) return;
      // 加载正文
      const needsLoadContent = tabs.filter(
        (tab) =>
          tab.type === "file" &&
          tab.content == null &&
          !loadedRef.current.has(tab.id + ":content"),
      );
      for (const tab of needsLoadContent) {
        loadedRef.current.add(tab.id + ":content");
        readLatestContent(tab.path)
          .then((loaded) => {
            applyLoadedContent(tab.id, tab.path, false, loaded);
          })
          .catch(() => {
            const current = useEditorTabsStore
              .getState()
              .byNovel[String(novelId)]?.tabs.find((item) => item.id === tab.id);
            if (current?.content == null && !current?.isDirty)
              updateTab(tab.id, { content: t("content.loadFailedCloseTab") });
          });
      }
      // 加载大纲（恢复后 viewMode 是 outline/outline-edit 时）
      const needsLoadOutline = tabs.filter(
        (tab) =>
          tab.type === "file" &&
          (tab.viewMode === "outline" || tab.viewMode === "outline-edit") &&
          tab.outlineContent == null &&
          !loadedRef.current.has(tab.id + ":outline"),
      );
      for (const tab of needsLoadOutline) {
        loadedRef.current.add(tab.id + ":outline");
        loadOutlineContent(tab);
      }
      // eslint-disable-next-line react-hooks/exhaustive-deps -- initRef.current is mutable and not a valid dependency; effect should only re-run when tabs/novelId change
    }, [tabs, novelId, t, updateTab, loadOutlineContent, readLatestContent, applyLoadedContent]);

    // Ctrl+Shift+V 切换技能预览
    useEffect(() => {
      const handler = (e: KeyboardEvent) => {
        if ((e.ctrlKey || e.metaKey) && e.shiftKey && e.key === "V") {
          const tab = tabs.find((t) => t.id === activeTabId);
          if (
            tab?.type === "file" &&
            (isSkillPath(tab.path) || isStandaloneMarkdownPath(tab.path))
          ) {
            e.preventDefault();
            const newMode = tab.viewMode === "preview" ? "content" : "preview";
            updateTab(tab.id, { viewMode: newMode });
          }
        }
      };
      window.addEventListener("keydown", handler);
      return () => window.removeEventListener("keydown", handler);
    }, [tabs, activeTabId, updateTab]);

    // ── 切换 viewMode：按需加载大纲内容 ──────────────────────

    const handleSetViewMode = useCallback(
      (tabId: string, mode: "content" | "outline" | "outline-edit") => {
        const tab = tabs.find((t) => t.id === tabId);
        if (!tab) return;

        if (mode !== "content" && !tab.outlinePath) {
          void resolveChapter(tab.path)
            .then((item) => {
              if (!item) return;
              const resolvedTab = {
                ...tab,
                outlinePath: item.outline_file_path,
              };
              updateTab(tabId, {
                outlinePath: item.outline_file_path,
                viewMode: mode,
              });
              loadOutlineContent(resolvedTab);
            })
            .catch(() => undefined);
          return;
        }

        updateTab(tabId, { viewMode: mode });

        // 切换到大纲（预览或编辑）时，如果未加载（或上次加载时文件不存在）则重新加载
        if (
          (mode === "outline" || mode === "outline-edit") &&
          !tab.outlineContent
        ) {
          loadOutlineContent(tab);
        }
      },
      [tabs, updateTab, loadOutlineContent, resolveChapter],
    );

    // ── 阅读位置键（正文/大纲各自独立，见 useEditorTabsStore）──────

    const positionKeyFor = useCallback(
      (tab: EditorTab, mode: string): string | undefined => {
        let p = tab.path;
        // 大纲（预览/编辑）对应实际文件 outlines/NNN.md，正文对应 chapters/NNN.md
        if (
          (mode === "outline" || mode === "outline-edit") &&
          isContentPath(tab.path) &&
          tab.path !== "goink.md"
        ) {
          if (!tab.outlinePath) return undefined;
          p = tab.outlinePath;
        }
        return `${novelId}:${p}:${mode}`;
      },
      [novelId],
    );

    // ── 保存逻辑 ────────────────────────────────────────────

    const doSave = useCallback(
      async (
        tabId: string,
        path: string,
        content: string,
        dirtyKey: "isDirty" | "outlineIsDirty" = "isDirty",
      ) => {
        if (!novelIdRef.current) return;
        try {
          // 5.2 commit 2: SaveContent 走 mutation（onSuccess 失效 contentKeys.detail），
          // 调用方负责 updateTab(isDirty:false) + toastError（tab 是本地 state，不进 query cache）。
          await mutateSaveContent({
            novel_id: novelIdRef.current,
            path,
            content,
          });
          updateTab(tabId, { [dirtyKey]: false });
        } catch (err) {
          toastError(t("common.saveFailed") + ": " + toErrorMessage(err));
          console.error(err);
        }
      },
      [mutateSaveContent, updateTab, t],
    );

    const handleCloseTab = useCallback(
      async (id: string) => {
        const storedTabs =
          useEditorTabsStore.getState().byNovel[String(novelId)]?.tabs;
        const tab = storedTabs
          ? storedTabs.find((item) => item.id === id)
          : tabs.find((item) => item.id === id);
        if (!tab) return;
        if (tab?.type === "file") {
          if (tab.isDirty && !(await editorSaveQueue.flush(novelId, tab.path)))
            return;
          if (
            tab.outlineIsDirty &&
            tab.outlinePath &&
            !(await editorSaveQueue.flush(novelId, tab.outlinePath))
          )
            return;
        }
        const latestTabs =
          useEditorTabsStore.getState().byNovel[String(novelId)]?.tabs;
        const latest = latestTabs?.find((item) => item.id === id);
        if (latestTabs && !latest) return;
        if (latest?.isDirty || latest?.outlineIsDirty) return;
        closeTab(id);
      },
      [tabs, editorSaveQueue, novelId, closeTab],
    );

    const handleCloseAllTabs = useCallback(async () => {
      for (const tab of tabs) {
        if (tab.type !== "file") continue;
        if (tab.isDirty && !(await editorSaveQueue.flush(novelId, tab.path)))
          return;
        if (
          tab.outlineIsDirty &&
          tab.outlinePath &&
          !(await editorSaveQueue.flush(novelId, tab.outlinePath))
        )
          return;
      }
      const remaining =
        useEditorTabsStore.getState().byNovel[String(novelId)]?.tabs;
      if (remaining?.some((tab) => tab.isDirty || tab.outlineIsDirty)) return;
      closeAllTabs();
    }, [tabs, editorSaveQueue, novelId, closeAllTabs]);

    // Ctrl+S 立即保存
    useEffect(() => {
      const handler = (e: KeyboardEvent) => {
        if ((e.ctrlKey || e.metaKey) && !e.shiftKey && e.key === "s") {
          e.preventDefault();
          const activeNovelId = novelIdRef.current;
          const entry =
            useEditorTabsStore.getState().byNovel[String(activeNovelId)];
          const tab =
            entry?.tabs.find((item) => item.id === entry.activeTabId) ??
            activeTabRef.current;
          if (tab?.type !== "file") return;
          const path =
            tab.viewMode === "outline-edit" ? tab.outlinePath : tab.path;
          if (path) void editorSaveQueue.flush(activeNovelId, path);
        }
      };
      window.addEventListener("keydown", handler);
      return () => window.removeEventListener("keydown", handler);
    }, [editorSaveQueue]);

    const handleEditorChange = useCallback(
      (tabId: string, value: string | undefined) => {
        const content = value ?? "";
        const tab =
          useEditorTabsStore
            .getState()
            .byNovel[String(novelId)]?.tabs.find((t) => t.id === tabId) ??
          tabs.find((t) => t.id === tabId);
        if (!tab) return;
        updateTab(tabId, { content, isDirty: true });
        // 3.8 后续：activeContent 迁 useEditorStore。
        useEditorStore.getState().setActiveContent(content);

        scheduleEditorSave(
          tabId,
          tab.path,
          content,
          tab.contentBase ?? tab.content ?? "",
          "isDirty",
        );
      },
      [tabs, novelId, updateTab, scheduleEditorSave],
    );

    // 大纲编辑：内容存 outlineContent，保存路径派生 outlinePath
    const handleOutlineEditorChange = useCallback(
      (tabId: string, value: string | undefined) => {
        const content = value ?? "";
        const tab =
          useEditorTabsStore
            .getState()
            .byNovel[String(novelId)]?.tabs.find((t) => t.id === tabId) ??
          tabs.find((t) => t.id === tabId);
        if (!tab?.outlinePath) return;
        updateTab(tabId, { outlineContent: content, outlineIsDirty: true });
        useEditorStore.getState().setActiveContent(content);

        scheduleEditorSave(
          tabId,
          tab.outlinePath,
          content,
          tab.outlineContentBase ?? tab.outlineContent ?? "",
          "outlineIsDirty",
        );
      },
      [tabs, novelId, updateTab, scheduleEditorSave],
    );

    const monacoRef = useRef<any>(null);

    // 将 rune 偏移转为 Monaco 行列号（1-based）
    function runeOffsetToMonaco(
      text: string,
      runeOffset: number,
    ): { line: number; col: number } {
      let runeCount = 0;
      const lines = text.split("\n");
      for (let i = 0; i < lines.length; i++) {
        const lineRunes = [...lines[i]].length;
        if (runeCount + lineRunes >= runeOffset) {
          return { line: i + 1, col: runeOffset - runeCount + 1 };
        }
        runeCount += lineRunes + 1; // +1 for \n
      }
      return { line: lines.length, col: 1 };
    }

    const doHighlight = useCallback(
      (
        editor: Parameters<OnMount>[0],
        content: string,
        matchPos: number,
        matchLen: number,
      ) => {
        const monaco = monacoRef.current;
        if (!monaco || !editor.getModel()) return;

        const totalLines = editor.getModel()!.getLineCount();
        const { line, col } = runeOffsetToMonaco(content, matchPos);
        const clampedEnd = Math.min(matchPos + matchLen, [...content].length);
        const { line: endLine, col: endCol } = runeOffsetToMonaco(
          content,
          clampedEnd,
        );
        const ctxEnd = Math.min(endLine + 1, totalLines);

        const decorations: any[] = [
          {
            range: new monaco.Range(Math.max(1, line - 1), 1, ctxEnd, 1),
            options: {
              isWholeLine: true,
              className: "search-context-highlight",
            },
          },
          {
            range: new monaco.Range(line, col, endLine, endCol),
            options: { className: "search-keyword-highlight" },
          },
        ];

        const collection = (editor as any)._searchDecorations;
        if (collection) collection.clear();
        (editor as any)._searchDecorations =
          editor.createDecorationsCollection(decorations);

        editor.revealPositionInCenter({ lineNumber: line, column: col });
        editor.setPosition({ lineNumber: line, column: col });
      },
      [],
    );

    const handleEditorMount: OnMount = useCallback(
      (editor, monaco) => {
        editorRef.current = editor;
        monacoRef.current = monaco;
        const tab = activeTabRef.current;
        const path =
          tab?.viewMode === "outline-edit" ? tab.outlinePath : tab?.path;
        const mountedNovelId = novelIdRef.current;
        editor.onDidBlurEditorText(() => {
          if (path) void editorSaveQueue.flush(mountedNovelId, path);
        });
        // 编辑器挂载后检查待处理高亮（直接取 Monaco model 内容，避免 ref 时序问题）。
        const pending = pendingHighlightRef.current;
        if (pending) {
          const content = editor.getModel()?.getValue();
          if (content) {
            doHighlight(editor, content, pending.matchPos, pending.matchLen);
            pendingHighlightRef.current = null;
            didApplyHighlightRef.current = true;
          }
        }
      },
      [editorSaveQueue, doHighlight],
    );

    // ── 打开/激活文件 tab ──────────────────────────────────

    const titleFromPath = useCallback(
      (p: string): string => {
        if (p === "goink.md") return t("content.storyStatus");
        if (isSkillPath(p))
          return `${t("content.skillLabel")}${skillNameFromPath(p)}`;
        return p;
      },
      [t],
    );

    const doOpenFile = useCallback(
      (
        path: string,
        title?: string,
        readOnly?: boolean,
        initialViewMode?: string,
      ) => {
        const existing = tabs.find((t) => t.path === path && t.type === "file");
        if (existing) {
          if (isVolumeOutlinePath(path) && title && existing.title !== title) {
            updateTab(existing.id, { title });
          }
          if (!existing.outlinePath && path.startsWith("chapters/")) {
            void resolveChapter(path)
              .then((item) => {
                if (item)
                  updateTab(existing.id, {
                    outlinePath: item.outline_file_path,
                  });
              })
              .catch(() => undefined);
          }
          if (initialViewMode) {
            updateTab(existing.id, {
              viewMode: initialViewMode as EditorTab["viewMode"],
            });
          }
          setActiveTabId(existing.id);
          // 3.8 后续：activeContent 迁 useEditorStore。
          useEditorStore.getState().setActiveContent(existing.content ?? "");
          return;
        }

        const skReadOnly = readOnly ?? path.startsWith("/builtin/skills/");
        const initialMode: EditorTab["viewMode"] =
          (initialViewMode as EditorTab["viewMode"]) ||
          (skReadOnly ? "preview" : isSkillPath(path) ? "preview" : "content");

        setIsLoading(true);
        Promise.all([
          readLatestContent(path).catch(() => ({
            content: "",
            version: aiFileVersion(novelId, path),
          })),
          resolveChapter(path).catch(() => undefined),
        ])
          .then(async ([initial, item]) => {
            if (novelIdRef.current !== novelId) return;
            let loaded = initial;
            while (loaded.version !== aiFileVersion(novelId, path)) {
              loaded = await readLatestContent(path, true);
            }
            if (novelIdRef.current !== novelId) return;
            const c = loaded.content;
            const display =
              title ||
              (item
                ? `${t("sidebar.chapterN", { n: item.reading_number })} ${item.title}`
                : titleFromPath(path));
            openTab({
              type: "file",
              path,
              outlinePath: item?.outline_file_path,
              title: display,
              content: c,
              contentBase: c,
              isDirty: false,
              viewMode: initialMode,
              readOnly: skReadOnly,
            });
            // 3.8 后续：activeContent 迁 useEditorStore。
            useEditorStore.getState().setActiveContent(c);
          })
          .finally(() => setIsLoading(false));
      },
      [
        novelId,
        tabs,
        readLatestContent,
        openTab,
        setActiveTabId,
        titleFromPath,
        updateTab,
        resolveChapter,
        t,
      ],
    );

    const clearHighlight = useCallback(() => {
      const editor = editorRef.current as any;
      if (editor?._searchDecorations) {
        editor._searchDecorations.clear();
        editor._searchDecorations = null;
      }
    }, []);

    const doOpenFileWithHighlight = useCallback(
      (path: string, title: string, matchPos: number, matchLen: number) => {
        if (matchPos < 0) {
          doOpenFile(path, title);
          return;
        }
        const existing = tabs.find((t) => t.path === path && t.type === "file");
        // 当前激活的 tab：直接应用高亮，不走 pending（setActiveTabId 同值不触发 effect）
        if (
          existing &&
          existing.id === activeTabId &&
          existing.content &&
          editorRef.current
        ) {
          doHighlight(editorRef.current, existing.content, matchPos, matchLen);
          return;
        }
        pendingHighlightRef.current = { matchPos, matchLen };
        if (existing) {
          setActiveTabId(existing.id);
          return;
        }
        doOpenFile(path, title);
      },
      [doOpenFile, tabs, activeTabId, setActiveTabId, doHighlight],
    );

    // tab 切换 / 内容就绪：有 pending 且 editor model 存活就应用高亮，否则清除旧高亮。
    // didApplyHighlightRef：handleEditorMount 在 layout effect 阶段消费 pending 后，
    // 标记跳过后续 effect 的清除，避免刚设的高亮被擦除。
    useEffect(() => {
      if (didApplyHighlightRef.current) {
        didApplyHighlightRef.current = false;
        return;
      }
      const editor = editorRef.current as any;
      const pending = pendingHighlightRef.current;
      // 必须检查 editor.getModel()：key 变化导致 ContentEditor 重建时，
      // unmount/remount 之间 editorRef 可能指向已销毁的旧 editor（model 为 null），
      // 此时不应消费 pending，留给 handleEditorMount 处理。
      if (pending && activeTab?.content && editor?.getModel()) {
        doHighlight(
          editor,
          activeTab.content,
          pending.matchPos,
          pending.matchLen,
        );
        pendingHighlightRef.current = null;
        return;
      }
      if (editor?._searchDecorations) {
        editor._searchDecorations.clear();
        editor._searchDecorations = null;
      }
    }, [activeTab?.id, activeTab?.content, doHighlight]);

    const filePathFromDiff = useCallback(
      async (
        diffPath: string,
      ): Promise<{
        filePath: string;
        viewMode: "content" | "outline";
      }> => {
        const item = await resolveChapter(diffPath).catch(() => undefined);
        if (item?.outline_file_path === diffPath) {
          return { filePath: item.file_path, viewMode: "outline" };
        }
        return { filePath: diffPath, viewMode: "content" };
      },
      [resolveChapter],
    );

    // ── 审批操作（由 WorkspaceView 通过 ref 调用）───────────

    const handleDiffApprove = useCallback(
      async (toolId: string) => {
        const dt = tabs.find((t) => t.type === "diff" && t.toolId === toolId);
        if (!dt) return;

        const { filePath, viewMode } = await filePathFromDiff(dt.path);
        const ft = tabs.find((t) => t.type === "file" && t.path === filePath);

        if (ft) await refreshApprovedFile(ft, dt.path, viewMode, true);

        closeTab(dt.id);
        doOpenFile(filePath, undefined, undefined, viewMode);
      },
      [tabs, closeTab, doOpenFile, filePathFromDiff, refreshApprovedFile],
    );

    const handleDiffReject = useCallback(
      async (toolId: string) => {
        const dt = tabs.find((t) => t.type === "diff" && t.toolId === toolId);
        if (!dt) return;

        const { filePath, viewMode } = await filePathFromDiff(dt.path);
        closeTab(dt.id);
        doOpenFile(filePath, undefined, undefined, viewMode);
      },
      [tabs, closeTab, doOpenFile, filePathFromDiff],
    );

    // ── 暴露给父组件的方法 ──────────────────────────────────

    useImperativeHandle(
      ref,
      () => ({
        openFile: doOpenFile,
        openFileWithHighlight: doOpenFileWithHighlight,
        clearHighlight,
        closeAllTabs: handleCloseAllTabs,
        openDiffTab,
        handleDiffApprove,
        handleDiffReject,
      }),
      [
        doOpenFile,
        doOpenFileWithHighlight,
        clearHighlight,
        handleCloseAllTabs,
        openDiffTab,
        handleDiffApprove,
        handleDiffReject,
      ],
    );

    // ── 渲染 ────────────────────────────────────────────────

    const tabBtnClass = (active: boolean) =>
      `px-3 py-1 text-xs rounded transition-colors cursor-pointer ${
        active
          ? "bg-muted text-foreground font-medium"
          : "text-muted-foreground hover:text-foreground"
      }`;

    // 空状态
    if (!activeTab) {
      return (
        <main className="flex-1 bg-background flex flex-col min-w-0 min-h-0 border-r overflow-hidden">
          <TabBar
            tabs={tabs}
            activeTabId={activeTabId}
            onSelect={setActiveTabId}
            onClose={handleCloseTab}
          />
          <div className="flex-1 flex items-center justify-center">
            {tabs.length === 0 ? (
              <div className="text-center">
                <FileText className="w-12 h-12 text-muted-foreground/20 mx-auto mb-3" />
                <p className="text-sm text-muted-foreground">
                  {t("content.selectOrCreateChapter")}
                </p>
              </div>
            ) : (
              <div className="text-center">
                <FileText className="w-12 h-12 text-muted-foreground/20 mx-auto mb-3" />
                <p className="text-sm text-muted-foreground">
                  {t("content.selectTab")}
                </p>
              </div>
            )}
          </div>
        </main>
      );
    }

    // Diff tab
    if (activeTab.type === "diff") {
      const isOutline = activeTab.path?.startsWith("outlines/");

      return (
        <main className="flex-1 bg-background flex flex-col min-w-0 min-h-0 border-r overflow-hidden">
          <TabBar
            tabs={tabs}
            activeTabId={activeTabId}
            onSelect={setActiveTabId}
            onClose={handleCloseTab}
          />
          <div className="flex items-center px-4 py-2 border-b shrink-0 select-none">
            <span className="text-sm font-medium truncate">
              {activeTab.title}
            </span>
          </div>
          <div className="flex-1 overflow-auto">
            {isOutline ? (
              <div className="p-6">
                <Markdown content={activeTab.modified ?? ""} />
              </div>
            ) : (
              <DiffEditor
                height="100%"
                language="markdown"
                theme={MONACO_THEME[theme]}
                original={activeTab.original}
                modified={activeTab.modified}
                onMount={(editor) => {
                  setTimeout(() => {
                    const modified = editor.getModifiedEditor();
                    const changes = editor.getLineChanges();
                    if (changes?.length) {
                      modified.revealLineInCenter(
                        changes[0].modifiedStartLineNumber,
                      );
                      modified.setPosition({
                        lineNumber: changes[0].modifiedStartLineNumber,
                        column: 1,
                      });
                    }
                  }, 100);
                }}
                options={{
                  minimap: { enabled: false },
                  scrollBeyondLastLine: false,
                  fontSize: 15,
                  lineHeight: 26,
                  fontFamily: "'Noto Serif SC', 'Source Han Serif SC', serif",
                  lineNumbers: "on",
                  wordWrap: "on",
                  automaticLayout: true,
                  readOnly: true,
                  renderSideBySide: false,
                  renderIndicators: true,
                }}
              />
            )}
          </div>
        </main>
      );
    }

    // File tab
    const viewMode = activeTab.viewMode || "content";
    const conflictOutline =
      viewMode === "outline" || viewMode === "outline-edit";
    const conflictPath = conflictOutline
      ? activeTab.outlinePath
      : activeTab.path;
    const hasConflict = conflictOutline
      ? activeTab.outlineContentConflict
      : activeTab.contentConflict;
    return (
      <main className="flex-1 bg-background flex flex-col min-w-0 min-h-0 border-r overflow-hidden">
        <TabBar
          tabs={tabs}
          activeTabId={activeTabId}
          onSelect={setActiveTabId}
          onClose={handleCloseTab}
        />
        <div className="flex items-center justify-between px-4 py-2 border-b shrink-0 select-none">
          <span className="text-sm font-medium truncate">
            {activeTab.title}
          </span>
          <div className="flex items-center gap-0.5 shrink-0">
            {isStandaloneMarkdownPath(activeTab.path) ? (
              <button
                onClick={() =>
                  updateTab(activeTab.id, {
                    viewMode: viewMode === "preview" ? "content" : "preview",
                  })
                }
                className={tabBtnClass(viewMode === "preview")}
              >
                {t("content.preview")}
              </button>
            ) : isSkillPath(activeTab.path) ? (
              <>
                <button
                  onClick={() =>
                    updateTab(activeTab.id, { viewMode: "preview" })
                  }
                  className={tabBtnClass(viewMode === "preview")}
                >
                  {t("content.preview")}
                </button>
                {!activeTab.readOnly && (
                  <button
                    onClick={() =>
                      updateTab(activeTab.id, { viewMode: "edit" })
                    }
                    className={tabBtnClass(viewMode === "edit")}
                  >
                    {t("content.edit")}
                  </button>
                )}
              </>
            ) : (
              <>
                <button
                  onClick={() => handleSetViewMode(activeTab.id, "content")}
                  className={tabBtnClass(viewMode === "content")}
                >
                  {t("content.body")}
                </button>
                <button
                  onClick={() => handleSetViewMode(activeTab.id, "outline")}
                  className={tabBtnClass(viewMode === "outline")}
                >
                  {t("content.outline")}
                </button>
                <button
                  onClick={() =>
                    handleSetViewMode(activeTab.id, "outline-edit")
                  }
                  className={tabBtnClass(viewMode === "outline-edit")}
                >
                  {t("content.outlineEdit")}
                </button>
              </>
            )}
          </div>
        </div>

        {hasConflict && conflictPath && (
          <ContentConflictNotice
            key={`${activeTab.id}:${conflictPath}`}
            tabId={activeTab.id}
            path={conflictPath}
            outline={conflictOutline}
            onLoad={loadConflictSnapshot}
            onChoose={chooseConflictVersion}
          />
        )}

        <div className="flex-1 min-h-0">
          {isLoading ? (
            <div className="flex items-center justify-center h-full">
              <Loader2 className="w-5 h-5 animate-spin text-muted-foreground" />
            </div>
          ) : viewMode === "preview" ? (
            isVolumeOutlinePath(activeTab.path) ? (
              <div className="h-full overflow-auto px-6 py-4">
                <Markdown content={activeTab.content ?? ""} />
              </div>
            ) : (
              <SkillPreview
                content={activeTab.content ?? ""}
                source={sourceFromPath(activeTab.path)}
              />
            )
          ) : viewMode === "edit" ? (
            <SkillEditForm
              content={activeTab.content ?? ""}
              source={sourceFromPath(activeTab.path)}
              readOnly={activeTab.readOnly}
              onSave={async (newContent) => {
                await doSave(
                  activeTab.id,
                  activeTab.path,
                  newContent as string,
                );
                updateTab(activeTab.id, {
                  content: newContent,
                  viewMode: "preview",
                });
              }}
              onCancel={() => updateTab(activeTab.id, { viewMode: "preview" })}
            />
          ) : viewMode === "content" ? (
            <ContentEditor
              key={`${activeTab.id}:content`}
              positionKey={positionKeyFor(activeTab, "content")}
              value={activeTab.content ?? ""}
              onChange={(v) => handleEditorChange(activeTab.id, v)}
              onMount={handleEditorMount}
              editorTheme={MONACO_THEME[theme]}
            />
          ) : viewMode === "outline-edit" ? (
            <ContentEditor
              key={`${activeTab.id}:outline-edit`}
              positionKey={positionKeyFor(activeTab, "outline-edit")}
              value={activeTab.outlineContent ?? ""}
              onChange={(v) => handleOutlineEditorChange(activeTab.id, v)}
              onMount={handleEditorMount}
              editorTheme={MONACO_THEME[theme]}
            />
          ) : (
            <OutlineViewer
              key={`${activeTab.id}:outline`}
              positionKey={positionKeyFor(activeTab, "outline")}
              content={activeTab.outlineContent ?? ""}
            />
          )}
        </div>
      </main>
    );
  },
);

export default ContentPanel;

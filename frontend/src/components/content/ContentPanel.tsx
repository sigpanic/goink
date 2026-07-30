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
import { toastError } from "@/utils/toast";
import { useApp } from "@/hooks/useApp";
import { useEditorTabs } from "@/hooks/useEditorTabs";
import { useTheme, type Theme } from "@/hooks/useTheme";
import { EventsOn } from "@/lib/wailsjs/runtime/runtime";
import TabBar from "./TabBar";
import ContentEditor from "./ContentEditor";
import OutlineViewer from "./OutlineViewer";
import SkillPreview from "./SkillPreview";
import SkillEditForm from "@/components/skill/SkillEditForm";
import Markdown from "@/components/Markdown";
import {
  outlinePath,
  isContentPath,
  isOutlinePath,
  isSkillPath,
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
  handleDiffReject: (toolId: string) => void;
}

interface Props {
  novelId: number;
  onContentChange?: (content: string) => void;
  onDirtyChange?: (isDirty: boolean) => void;
}

const ContentPanel = forwardRef<ContentPanelHandle, Props>(
  function ContentPanel({ novelId, onContentChange, onDirtyChange }, ref) {
    const app = useApp();
    const { t } = useTranslation();
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

    const { theme } = useTheme();
    const [isLoading, setIsLoading] = useState(false);
    const saveTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
    const editorRef = useRef<Parameters<OnMount>[0] | null>(null);
    const savingRef = useRef<{
      id: string;
      path: string;
      content: string;
    } | null>(null);
    const pendingHighlightRef = useRef<{
      matchPos: number;
      matchLen: number;
    } | null>(null);
    const didApplyHighlightRef = useRef(false); // handleEditorMount 已应用高亮时跳过清除
    const novelIdRef = useRef(novelId);
    const tabsRef = useRef(tabs);

    useEffect(() => {
      novelIdRef.current = novelId;
    }, [novelId]);
    useEffect(() => {
      tabsRef.current = tabs;
    }, [tabs]);

    useEffect(() => {
      return () => {
        if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
      };
    }, []);

    useEffect(() => {
      if (activeTab?.type === "file") {
        onContentChange?.(activeTab.content ?? "");
      }
    }, [activeTab, onContentChange]);

    useEffect(() => {
      onDirtyChange?.(activeTab?.isDirty ?? false);
    }, [activeTab?.isDirty, onDirtyChange]);

    // 从 localStorage 恢复 tab 后，自动加载文件内容
    const loadedRef = useRef<Set<string>>(new Set());
    useEffect(() => {
      // novelId 变化时重置
      loadedRef.current.clear();
    }, [novelId]);
    useEffect(() => {
      if (!initRef.current) return;
      const needsLoad = tabs.filter(
        (tab) =>
          tab.type === "file" &&
          tab.content == null &&
          !loadedRef.current.has(tab.id),
      );
      if (needsLoad.length === 0) return;
      for (const tab of needsLoad) {
        loadedRef.current.add(tab.id);
        app
          .GetContent(novelId, tab.path)
          .then((content) => {
            updateTab(tab.id, { content: content ?? "" });
          })
          .catch(() => {
            updateTab(tab.id, { content: t("content.loadFailedCloseTab") });
          });
      }
      // eslint-disable-next-line react-hooks/exhaustive-deps -- initRef.current is mutable and not a valid dependency; effect should only re-run when tabs/novelId change
    }, [tabs, novelId, app, t, updateTab]);

    // Ctrl+Shift+V 切换技能预览
    useEffect(() => {
      const handler = (e: KeyboardEvent) => {
        if ((e.ctrlKey || e.metaKey) && e.shiftKey && e.key === "V") {
          const tab = tabs.find((t) => t.id === activeTabId);
          if (
            tab?.type === "file" &&
            (isSkillPath(tab.path) || tab.path === "goink.md")
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
      (tabId: string, mode: "content" | "outline") => {
        const tab = tabs.find((t) => t.id === tabId);
        if (!tab) return;

        updateTab(tabId, { viewMode: mode });

        // 切换到大纲时，如果未加载（或上次加载时文件不存在）则重新加载
        if (mode === "outline" && tab.type === "file" && !tab.outlineContent) {
          const derivedOutline =
            isContentPath(tab.path) && tab.path !== "goink.md"
              ? outlinePath(
                  parseInt(tab.path.replace(/.*\//, "").replace(".md", "")),
                )
              : null;
          if (derivedOutline) {
            app
              .GetContent(novelId, derivedOutline)
              .then((oc) => {
                updateTab(tabId, { outlineContent: oc || "" });
              })
              .catch(() => {
                updateTab(tabId, { outlineContent: "" });
              });
          }
        }
      },
      [novelId, tabs, app, updateTab],
    );

    // ── 保存逻辑 ────────────────────────────────────────────

    const doSave = useCallback(
      async (tabId: string, path: string, content: string) => {
        if (!novelIdRef.current) return;
        try {
          await app.SaveContent({
            novel_id: novelIdRef.current,
            path,
            content,
          });
          updateTab(tabId, { isDirty: false });
        } catch (err) {
          toastError(
            t("common.saveFailed") +
              ": " +
              (err instanceof Error ? err.message : String(err)),
          );
          console.error(err);
        }
      },
      [app, updateTab, t],
    );

    // Ctrl+S 立即保存
    useEffect(() => {
      const handler = (e: KeyboardEvent) => {
        if ((e.ctrlKey || e.metaKey) && !e.shiftKey && e.key === "s") {
          e.preventDefault();
          if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
          const s = savingRef.current;
          if (s) doSave(s.id, s.path, s.content);
        }
      };
      window.addEventListener("keydown", handler);
      return () => window.removeEventListener("keydown", handler);
    }, [doSave]);

    const handleEditorChange = useCallback(
      (tabId: string, value: string | undefined) => {
        const content = value ?? "";
        updateTab(tabId, { content, isDirty: true });
        onContentChange?.(content);

        if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
        const tab = tabs.find((t) => t.id === tabId);
        if (!tab) return;
        savingRef.current = { id: tabId, path: tab.path, content };
        saveTimerRef.current = setTimeout(() => {
          if (!savingRef.current) return;
          const s = savingRef.current;
          doSave(s.id, s.path, s.content);
        }, 500);
      },
      [tabs, updateTab, doSave, onContentChange],
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
        editor.onDidBlurEditorText(() => {
          if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
          const s = savingRef.current;
          if (!s) return;
          doSave(s.id, s.path, s.content);
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
      [doSave, doHighlight],
    );

    // ── file:changed 事件监听 ─────────────────────────────────
    // 用 ref 读取最新 tabs，避免因 tabs 变化频繁重建订阅丢失事件

    useEffect(() => {
      const unsub = EventsOn("file:changed", async (data: any) => {
        if (data.novel_id !== novelIdRef.current) return;

        for (const tab of tabsRef.current) {
          if (tab.type !== "file") continue;

          let needRefresh = false;
          let refreshKey: "content" | "outlineContent" = "content";

          if (tab.path === data.path) {
            needRefresh = true;
            refreshKey = "content";
          } else {
            const derivedOutline =
              isContentPath(tab.path) && tab.path !== "goink.md"
                ? outlinePath(
                    parseInt(tab.path.replace(/.*\//, "").replace(".md", "")),
                  )
                : null;
            if (derivedOutline && derivedOutline === data.path) {
              needRefresh = true;
              refreshKey = "outlineContent";
            }
          }

          if (needRefresh) {
            try {
              const fresh = await app.GetContent(data.novel_id, data.path);
              const patch: Partial<EditorTab> = { [refreshKey]: fresh };
              if (refreshKey === "content") patch.isDirty = false;
              updateTab(tab.id, patch);
            } catch {
              /* 文件可能被删 */
            }
          }
        }
      });
      return () => unsub();
    }, [app, updateTab]);

    // ── 打开/激活文件 tab ──────────────────────────────────

    const titleFromPath = useCallback(
      (p: string): string => {
        if (p.startsWith("chapters/")) {
          const num = parseInt(p.replace("chapters/", "").replace(".md", ""));
          return t("sidebar.chapterN", { n: num });
        }
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
        const display = title || titleFromPath(path);
        const existing = tabs.find((t) => t.path === path && t.type === "file");
        if (existing) {
          if (initialViewMode) {
            updateTab(existing.id, {
              viewMode: initialViewMode as EditorTab["viewMode"],
            });
          }
          setActiveTabId(existing.id);
          onContentChange?.(existing.content ?? "");
          return;
        }

        const skReadOnly = readOnly ?? path.startsWith("/builtin/skills/");
        const initialMode: EditorTab["viewMode"] =
          (initialViewMode as EditorTab["viewMode"]) ||
          (skReadOnly ? "preview" : isSkillPath(path) ? "preview" : "content");

        setIsLoading(true);
        app
          .GetContent(novelId, path)
          .then((content) => {
            const c = content ?? "";
            openTab({
              type: "file",
              path,
              title: display,
              content: c,
              isDirty: false,
              viewMode: initialMode,
              readOnly: skReadOnly,
            });
            onContentChange?.(c);
          })
          .catch(() => {
            openTab({
              type: "file",
              path,
              title: display,
              content: "",
              isDirty: false,
              viewMode: initialMode,
              readOnly: skReadOnly,
            });
            onContentChange?.("");
          })
          .finally(() => setIsLoading(false));
      },
      [
        novelId,
        tabs,
        app,
        openTab,
        setActiveTabId,
        onContentChange,
        titleFromPath,
        updateTab,
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

    function filePathFromDiff(diffPath: string): {
      filePath: string;
      viewMode: "content" | "outline";
    } {
      if (isOutlinePath(diffPath)) {
        return {
          filePath: diffPath.replace("outlines/", "chapters/"),
          viewMode: "outline",
        };
      }
      return { filePath: diffPath, viewMode: "content" };
    }

    // ── 审批操作（由 WorkspaceView 通过 ref 调用）───────────

    const handleDiffApprove = useCallback(
      async (toolId: string) => {
        const dt = tabs.find((t) => t.type === "diff" && t.toolId === toolId);
        if (!dt) return;

        const { filePath, viewMode } = filePathFromDiff(dt.path);
        const ft = tabs.find((t) => t.type === "file" && t.path === filePath);

        if (ft) {
          try {
            const fresh = await app.GetContent(novelId, dt.path);
            const patch: Partial<EditorTab> = { viewMode };
            if (viewMode === "outline") {
              patch.outlineContent = fresh;
            } else {
              patch.content = fresh;
              patch.isDirty = false;
            }
            updateTab(ft.id, patch);
          } catch {
            /* ignored */
          }
        }

        closeTab(dt.id);
        doOpenFile(filePath);
      },
      [novelId, tabs, app, updateTab, closeTab, doOpenFile],
    );

    const handleDiffReject = useCallback(
      (toolId: string) => {
        const dt = tabs.find((t) => t.type === "diff" && t.toolId === toolId);
        if (!dt) return;

        const { filePath } = filePathFromDiff(dt.path);
        closeTab(dt.id);
        doOpenFile(filePath);
      },
      [tabs, closeTab, doOpenFile],
    );

    // ── 暴露给父组件的方法 ──────────────────────────────────

    useImperativeHandle(
      ref,
      () => ({
        openFile: doOpenFile,
        openFileWithHighlight: doOpenFileWithHighlight,
        clearHighlight,
        closeAllTabs,
        openDiffTab,
        handleDiffApprove,
        handleDiffReject,
      }),
      [
        doOpenFile,
        doOpenFileWithHighlight,
        clearHighlight,
        closeAllTabs,
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
            onClose={closeTab}
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
            onClose={closeTab}
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
                  lineNumbers: "off",
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
    return (
      <main className="flex-1 bg-background flex flex-col min-w-0 min-h-0 border-r overflow-hidden">
        <TabBar
          tabs={tabs}
          activeTabId={activeTabId}
          onSelect={setActiveTabId}
          onClose={closeTab}
        />
        <div className="flex items-center justify-between px-4 py-2 border-b shrink-0 select-none">
          <span className="text-sm font-medium truncate">
            {activeTab.title}
          </span>
          <div className="flex items-center gap-0.5 shrink-0">
            {activeTab.path === "goink.md" ? (
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
              </>
            )}
          </div>
        </div>

        <div className="flex-1 min-h-0">
          {isLoading ? (
            <div className="flex items-center justify-center h-full">
              <Loader2 className="w-5 h-5 animate-spin text-muted-foreground" />
            </div>
          ) : viewMode === "preview" ? (
            <SkillPreview
              content={activeTab.content ?? ""}
              source={sourceFromPath(activeTab.path)}
            />
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
              value={activeTab.content ?? ""}
              onChange={(v) => handleEditorChange(activeTab.id, v)}
              onMount={handleEditorMount}
              editorTheme={MONACO_THEME[theme]}
            />
          ) : (
            <OutlineViewer content={activeTab.outlineContent ?? ""} />
          )}
        </div>
      </main>
    );
  },
);

export default ContentPanel;

import { useState, useEffect, useCallback, useRef, useMemo } from "react";
import { flushSync } from "react-dom";
import { useTranslation } from "react-i18next";
import { useApp } from "@/hooks/useApp";
import type { imp, novel, chapter } from "@/hooks/useApp";
import type { git } from "@/lib/wailsjs/go/models";
import ActivityBar from "@/components/shell/ActivityBar";
import StatusBar from "@/components/shell/StatusBar";
import SidePanel from "@/components/sidebar/SidePanel";
import ContentPanel, {
  type ContentPanelHandle,
} from "@/components/content/ContentPanel";
import CharacterListView from "@/components/character/CharacterListView";
import LocationListView from "@/components/location/LocationListView";
import ArcListView from "@/components/storyarc/ArcListView";
import TimelineView from "@/components/timeline/TimelineView";
import ReaderView from "@/components/reader/ReaderView";
import PreferenceView from "@/components/preference/PreferenceView";
import NovelSettingView from "@/components/novel-setting/NovelSettingView";
import BookshelfView from "@/components/novel/BookshelfView";
import NovelEditDialog from "@/components/novel/NovelEditDialog";
import NovelDeleteDialog from "@/components/novel/NovelDeleteDialog";
import ImportProgressDialog from "@/components/novel/ImportProgressDialog";
import ExportDialog from "@/components/export/ExportDialog";
import ChatPanel from "@/components/chat/ChatPanel";
import GitHubLink from "@/components/shell/GitHubLink";
import SettingsDialog from "@/components/settings/SettingsDialog";
import HelpDialog from "@/components/help/HelpDialog";
import ProfileView from "@/components/profile/ProfileView";
import GitCommitView from "@/components/git/GitCommitView";
import ExtractWorkspaceView from "@/components/extract/ExtractWorkspaceView";
import UpdateDialog from "@/components/update/UpdateDialog";
import ErrorBoundary from "@/components/shared/ErrorBoundary";
import { search } from "@/lib/wailsjs/go/models";
import type { update as updateModels } from "@/lib/wailsjs/go/models";
import { CheckUpdate } from "@/lib/wailsjs/go/app/App";
import { Settings, User, HelpCircle, Moon, Sun } from "lucide-react";
import {
  WindowMinimise,
  WindowToggleMaximise,
  Quit,
} from "@/lib/wailsjs/runtime/runtime";
import Logo from "@/components/Logo";
import { useTheme, type Theme } from "@/hooks/useTheme";
import { useLayoutState } from "@/hooks/useLayoutState";
import { useWindowState } from "@/hooks/useWindowState";
import { useImportNovel } from "@/hooks/useImportNovel";
import { RefreshContext } from "@/hooks/useRefresh";
import type { PanelId, SidebarPanelId } from "@/types/panel";

const THEME_ICON: Record<Theme, React.ReactNode> = {
  light: <Moon className="w-5 h-5" />,
  dark: <Sun className="w-5 h-5" />,
};

// 走 ContentPanel 的面板（activePanel 路由层）。ContentPanel 内部通过 tab 机制
// 显示章节/skill/goink.md/diff/大纲，不经过 activePanel 路由。
const CONTENT_PANEL_IDS = new Set<PanelId>(["chapters", "skills"]);

interface Props {
  initialNovelId: number;
  initialShowHelp?: boolean;
}

export default function WorkspaceView({
  initialNovelId,
  initialShowHelp,
}: Props) {
  const { t } = useTranslation();
  const THEME_LABEL: Record<Theme, string> = {
    light: t("workspace.darkMode"),
    dark: t("workspace.lightMode"),
  };
  const app = useApp();
  const contentRef = useRef<ContentPanelHandle>(null);

  const [novels, setNovels] = useState<novel.Novel[]>([]);
  const [activeNovelId, setActiveNovelId] = useState(initialNovelId);
  const [activePanel, setActivePanel] = useState<PanelId>(
    initialNovelId ? "chapters" : "novels",
  );
  const [sidebarPanel, setSidebarPanel] = useState<SidebarPanelId | null>(null);
  const [searchQuery, setSearchQuery] = useState("");
  const [searchResults, setSearchResults] = useState<search.Result[]>([]);
  const [characterFocusId, setCharacterFocusId] = useState<number>(0);
  const [locationFocusId, setLocationFocusId] = useState<number>(0);
  const [timelineFocusId, setTimelineFocusId] = useState<number>(0);
  const [arcFocusId, setArcFocusId] = useState<number>(0);
  const [readerFocusId, setReaderFocusId] = useState<number>(0);
  const [preferenceFocusId, setPreferenceFocusId] = useState<number>(0);
  const [settingFocusId, setSettingFocusId] = useState<number>(0);
  const [styleSampleFocusId, setStyleSampleFocusId] = useState<number | null>(
    null,
  );
  const [showCreate, setShowCreate] = useState(false);
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [showSettings, setShowSettings] = useState(false);
  const [showHelp, setShowHelp] = useState(false);
  const [tabTarget, setTabTarget] = useState<{
    path: string;
    title: string;
  } | null>(null);
  const [activeContent, setActiveContent] = useState("");
  const [isDirty, setIsDirty] = useState(false);
  const [activeSkillName, setActiveSkillName] = useState<string | null>(null);
  const [selectedGitFile, setSelectedGitFile] = useState<git.FileDiff | null>(
    null,
  );
  const [platformOS, setPlatformOS] = useState("");
  const loadedRef = useRef(false);
  const { theme, toggle: toggleTheme } = useTheme();
  const { isMaximised, setIsMaximised } = useWindowState();
  const {
    sidePanelWidth,
    chatPanelWidth,
    setSidePanelWidth,
    setChatPanelWidth,
  } = useLayoutState();
  const [sidebarClosed, setSidebarClosed] = useState(false);

  // 侧边栏 List 与主区 View 的刷新信号：View 保存/删除后 bumpRefresh，
  // List 的 useEffect 依赖 refreshNonce，变化即重载。Provider 包裹下方 SidePanel + 各 View。
  const [refreshNonce, setRefreshNonce] = useState(0);
  const bumpRefresh = useCallback(() => setRefreshNonce((n) => n + 1), []);
  const refreshValue = useMemo(
    () => ({ refreshNonce, bumpRefresh }),
    [refreshNonce, bumpRefresh],
  );

  // ── 更新检查 ────────────────────────────────────────────
  const [showUpdate, setShowUpdate] = useState(false);
  const [updateResult, setUpdateResult] =
    useState<updateModels.CheckResult | null>(null);

  // ── 书籍管理弹窗 ──────────────────────────────────────
  const [editingNovel, setEditingNovel] = useState<novel.Novel | null>(null);
  const [deletingNovel, setDeletingNovel] = useState<novel.Novel | null>(null);
  const [showCreateDialog, setShowCreateDialog] = useState(false);
  const [exportNovelId, setExportNovelId] = useState<number | null>(null);

  // ── 窗口状态 ────────────────────────────────────────────

  useEffect(() => {
    app.GetPlatform().then((info) => {
      if (info.os) setPlatformOS(info.os as string);
    });
  }, [app]);

  // ── 首次进入自动弹帮助 ──────────────────────────────────

  useEffect(() => {
    if (initialShowHelp) setShowHelp(true);
  }, [initialShowHelp]);

  // ── 启动后延迟检查更新 ──────────────────────────────────

  useEffect(() => {
    const timer = setTimeout(async () => {
      try {
        const result = await CheckUpdate(false);
        if (result && result.hasUpdate) {
          setUpdateResult(result);
          setShowUpdate(true);
        }
      } catch {
        /* 静默失败 */
      }
    }, 30_000);
    return () => clearTimeout(timer);
  }, []);

  // ── 作品列表 ────────────────────────────────────────────

  const loadNovels = useCallback(async () => {
    const list = await app.GetNovels();
    setNovels(list ?? []);
    loadedRef.current = true;
  }, [app]);

  const handleImportedNovel = useCallback(
    async (res: imp.ImportResult) => {
      await loadNovels();
      setActiveNovelId(res.novel_id);
      setActivePanel("chapters");
      contentRef.current?.closeAllTabs();
      setTabTarget(null);
      setActiveContent("");
      setSelectedGitFile(null);
      await app.SetActiveNovel({ novel_id: res.novel_id });
    },
    [app, loadNovels],
  );

  const importNovel = useImportNovel({ app, onImported: handleImportedNovel });

  useEffect(() => {
    loadNovels();
  }, [loadNovels]);

  // ── SidePanel → ContentPanel 桥接 ─────────────────────────

  function handleSelectChapter(ch: chapter.Chapter) {
    const chTitle = `${t("sidebar.chapterN", { n: ch.chapter_number })} ${ch.title}`;
    setTabTarget({ path: ch.file_path, title: chTitle });
    contentRef.current?.openFile(ch.file_path, chTitle);
  }

  function handleSelectGoink() {
    setTabTarget({ path: "goink.md", title: t("workspace.storyStatus") });
    contentRef.current?.openFile("goink.md", t("workspace.storyStatus"));
  }

  // ── Approval ────────────────────────────────────────────

  async function handleApprove(toolId: string, feedback: string) {
    await app.ApproveTool(toolId, true, feedback);
    await contentRef.current?.handleDiffApprove(toolId);
  }

  async function handleReject(toolId: string, feedback: string) {
    await app.ApproveTool(toolId, false, feedback);
    contentRef.current?.handleDiffReject(toolId);
  }

  function handleApprovalFileEdit(data: {
    path: string;
    title: string;
    diff: string;
    original: string;
    modified: string;
    changeType: string;
    reason: string;
    toolId: string;
  }) {
    contentRef.current?.openDiffTab(data);
  }

  // ── 自动选择小说 ────────────────────────────────────────

  useEffect(() => {
    if (!loadedRef.current) return;
    const exists = novels.find((n) => n.id === activeNovelId);
    if (!exists && novels.length > 0) {
      const first = novels[0];
      setActiveNovelId(first.id);
      setActivePanel("chapters");
      app.SetActiveNovel({ novel_id: first.id });
    } else if (novels.length === 0) {
      setActivePanel("novels");
    }
  }, [app, novels, activeNovelId]);

  function handleActivitySelect(id: SidebarPanelId) {
    const currentPanel = sidebarPanel ?? activePanel;
    if (id === currentPanel && !sidebarClosed) {
      setSidebarClosed(true);
      return;
    }
    setSidebarClosed(false);
    if (id === "search") {
      setSidebarPanel("search");
    } else {
      setSidebarPanel(null);
      setActivePanel(id);
      contentRef.current?.clearHighlight();
    }
  }

  function handleSelectGitFile(file: git.FileDiff) {
    setSelectedGitFile(file);
  }

  function handleSearchNavigateEntity(panelId: PanelId, entityId: number) {
    setCharacterFocusId(0);
    setLocationFocusId(0);
    setTimelineFocusId(0);
    setArcFocusId(0);
    setReaderFocusId(0);
    setPreferenceFocusId(0);
    switch (panelId) {
      case "characters":
        setCharacterFocusId(entityId);
        break;
      case "locations":
        setLocationFocusId(entityId);
        break;
      case "timeline":
        setTimelineFocusId(entityId);
        break;
      case "storyarcs":
        setArcFocusId(entityId);
        break;
      case "reader":
        setReaderFocusId(entityId);
        break;
      case "preferences":
        setPreferenceFocusId(entityId);
        break;
      case "novel-settings":
        setSettingFocusId(entityId);
        break;
    }
    setActivePanel(panelId);
  }

  function handleSearchNavigateChapter(
    filePath: string,
    title: string,
    _chapterNum: number,
    matchPos: number,
    matchLen: number,
  ) {
    flushSync(() => setActivePanel("chapters"));
    if (matchPos >= 0 && matchLen > 0) {
      contentRef.current?.openFileWithHighlight(
        filePath,
        title,
        matchPos,
        matchLen,
      );
    } else {
      contentRef.current?.openFile(filePath, title);
    }
  }

  async function handleSelectNovel(n: novel.Novel) {
    try {
      setActiveNovelId(n.id);
      setActivePanel("chapters");
      contentRef.current?.closeAllTabs();
      setTabTarget(null);
      setActiveContent("");
      setSelectedGitFile(null);
      await app.SetActiveNovel({ novel_id: n.id });
    } catch (err) {
      console.error(err);
    }
  }

  async function handleCreateNovel() {
    try {
      if (!title.trim()) return;
      const n = await app.CreateNovel({
        title: title.trim(),
        description: description.trim(),
      });
      if (n) {
        setTitle("");
        setDescription("");
        setShowCreate(false);
        await loadNovels();
        setActiveNovelId(n.id);
        setActivePanel("chapters");
        await app.SetActiveNovel({ novel_id: n.id });
      }
    } catch (err) {
      console.error(err);
    }
  }

  async function handleCreateNovelFromDialog(input: {
    title: string;
    description: string;
    genre: string;
  }) {
    try {
      const n = await app.CreateNovel({
        title: input.title,
        description: input.description,
        genre: input.genre,
      });
      if (n) {
        setShowCreateDialog(false);
        await loadNovels();
        setActiveNovelId(n.id);
        setActivePanel("chapters");
        await app.SetActiveNovel({ novel_id: n.id });
      }
    } catch (err) {
      console.error(err);
      throw err;
    }
  }

  async function handleUpdateNovel(input: {
    title: string;
    description: string;
    genre: string;
  }) {
    if (!editingNovel) return;
    try {
      await app.UpdateNovel(editingNovel.id, input);
      setEditingNovel(null);
      await loadNovels();
    } catch (err) {
      console.error(err);
      throw err;
    }
  }

  async function handleDeleteNovel() {
    if (!deletingNovel) return;
    try {
      await app.DeleteNovel(deletingNovel.id);
      setDeletingNovel(null);
      await loadNovels();
    } catch (err) {
      console.error(err);
      throw err;
    }
  }

  async function handleExportNovel(format: "epub" | "markdown" | "txt") {
    if (exportNovelId == null) return;
    try {
      await app.ExportNovel(exportNovelId, format);
    } catch (err) {
      console.error(err);
      throw err;
    }
  }

  async function handleSaveCover(novelID: number, file: File) {
    const buf = await file.arrayBuffer();
    await app.SaveCover(novelID, Array.from(new Uint8Array(buf)));
  }

  const activeNovel = novels.find((n) => n.id === activeNovelId);

  // ── 窗口按钮样式 ────────────────────────────────────────

  const winBtn =
    "w-12 h-full flex items-center justify-center cursor-pointer text-foreground/80 hover:text-foreground hover:bg-black/25 hover:shadow-md transition-all";
  const closeBtn =
    "w-12 h-full flex items-center justify-center cursor-pointer text-foreground/80 hover:text-destructive-foreground hover:bg-destructive transition-colors";

  return (
    <RefreshContext.Provider value={refreshValue}>
      <div className="h-screen flex flex-col overflow-hidden">
        <header
          className="h-11 flex items-center border-b bg-sidebar shrink-0 select-none cursor-default"
          style={{ "--wails-draggable": "drag" } as React.CSSProperties}
          onDoubleClick={() => {
            WindowToggleMaximise();
            setIsMaximised((prev) => !prev);
          }}
        >
          <Logo className="h-7 w-7 ml-3" />
          <span className="text-sm font-medium pl-2 flex-1">
            {activeNovel?.title ?? "Goink"}
          </span>
          <div
            className="flex items-center h-full"
            style={{ "--wails-draggable": "no-drag" } as React.CSSProperties}
          >
            <GitHubLink />
            <button
              onClick={() => setActivePanel("profile")}
              className={`text-muted-foreground hover:text-foreground transition-colors cursor-pointer w-8 h-8 flex items-center justify-center ml-2 ${activePanel === "profile" ? "text-foreground" : ""}`}
              title={t("workspace.profile")}
            >
              <User className="w-5 h-5" />
            </button>
            <button
              onClick={() => setShowHelp(true)}
              className="text-muted-foreground hover:text-foreground transition-colors cursor-pointer w-8 h-8 flex items-center justify-center"
              title={t("workspace.help")}
            >
              <HelpCircle className="w-5 h-5" />
            </button>
            <button
              onClick={toggleTheme}
              className="text-muted-foreground hover:text-foreground transition-colors cursor-pointer w-8 h-8 flex items-center justify-center"
              title={THEME_LABEL[theme]}
            >
              {THEME_ICON[theme]}
            </button>
            <button
              onClick={() => setShowSettings(true)}
              className="text-muted-foreground hover:text-foreground transition-colors cursor-pointer w-8 h-8 flex items-center justify-center mr-1"
              title={t("workspace.settings")}
            >
              <Settings className="w-5 h-5" />
            </button>
            {platformOS !== "darwin" && (
              <>
                <button
                  onClick={WindowMinimise}
                  className={winBtn}
                  title={t("workspace.minimize")}
                >
                  <svg width="12" height="12" viewBox="0 0 12 12">
                    <path
                      d="M2.5 6h7"
                      stroke="currentColor"
                      strokeWidth="1.1"
                      strokeLinecap="round"
                    />
                  </svg>
                </button>
                <button
                  onClick={() => {
                    WindowToggleMaximise();
                    setIsMaximised((prev) => !prev);
                  }}
                  className={winBtn}
                  title={
                    isMaximised
                      ? t("workspace.restore")
                      : t("workspace.maximize")
                  }
                >
                  {isMaximised ? (
                    <svg width="12" height="12" viewBox="0 0 12 12">
                      <rect
                        x="4"
                        y="1.5"
                        width="6.5"
                        height="6.5"
                        rx="1"
                        fill="none"
                        stroke="currentColor"
                        strokeWidth=".9"
                      />
                      <rect
                        x="1.5"
                        y="2.5"
                        width="6.5"
                        height="6.5"
                        rx="1"
                        fill="var(--color-sidebar)"
                        stroke="currentColor"
                        strokeWidth=".9"
                      />
                    </svg>
                  ) : (
                    <svg width="12" height="12" viewBox="0 0 12 12">
                      <rect
                        x="1.5"
                        y="1.5"
                        width="9"
                        height="9"
                        stroke="currentColor"
                        strokeWidth=".9"
                        rx=".5"
                        fill="none"
                      />
                    </svg>
                  )}
                </button>
                <button
                  onClick={Quit}
                  className={closeBtn}
                  title={t("workspace.close")}
                >
                  <svg width="12" height="12" viewBox="0 0 12 12">
                    <path
                      d="M2.5 2.5l7 7M9.5 2.5l-7 7"
                      stroke="currentColor"
                      strokeWidth="1"
                      strokeLinecap="round"
                    />
                  </svg>
                </button>
              </>
            )}
          </div>
        </header>

        <div className="flex-1 flex min-h-0 overflow-hidden">
          <ActivityBar
            activeId={sidebarPanel ?? activePanel}
            onSelect={handleActivitySelect}
          />

          {!sidebarClosed && (
            <SidePanel
              activePanel={sidebarPanel ?? activePanel}
              novels={novels}
              novelId={activeNovelId}
              onSelectNovel={handleSelectNovel}
              onSelectChapter={handleSelectChapter}
              onSelectGoink={handleSelectGoink}
              onExportNovel={(id) => setExportNovelId(id)}
              target={tabTarget}
              showCreate={showCreate}
              setShowCreate={setShowCreate}
              title={title}
              setTitle={setTitle}
              description={description}
              setDescription={setDescription}
              onCreateNovel={handleCreateNovel}
              activeSkillName={activeSkillName}
              onSelectSkill={(path, title, readOnly) => {
                setActiveSkillName(title);
                contentRef.current?.openFile(path, title, readOnly);
              }}
              onEditSkill={(path, title, readOnly) => {
                setActiveSkillName(title);
                contentRef.current?.openFile(path, title, readOnly, "edit");
              }}
              onNewSkill={(name) => {
                setActiveSkillName(`${t("workspace.skillLabel")}${name}`);
                contentRef.current?.openFile(
                  `skills/${name}.md`,
                  `${t("workspace.skillLabel")}${name}`,
                  false,
                  "edit",
                );
              }}
              onSearchNavigateEntity={handleSearchNavigateEntity}
              onSearchNavigateChapter={handleSearchNavigateChapter}
              searchQuery={searchQuery}
              searchResults={searchResults}
              onSearchChange={(q, r) => {
                setSearchQuery(q);
                setSearchResults(r);
              }}
              onSelectGitFile={handleSelectGitFile}
              onSelectStyleSample={(id) => setStyleSampleFocusId(id)}
              sidePanelWidth={sidePanelWidth}
              onSidePanelResize={setSidePanelWidth}
            />
          )}

          {activePanel === "novels" ? (
            <BookshelfView
              novels={novels}
              activeNovelId={activeNovelId}
              onSelectNovel={handleSelectNovel}
              onEditNovel={setEditingNovel}
              onDeleteNovel={setDeletingNovel}
              onCreateNovel={() => setShowCreateDialog(true)}
              onSaveCover={handleSaveCover}
              onExportNovel={(n) => setExportNovelId(n.id)}
              onImportNovel={() => importNovel.startImport()}
            />
          ) : (
            CONTENT_PANEL_IDS.has(activePanel) && (
              <ContentPanel
                ref={contentRef}
                novelId={activeNovelId}
                onContentChange={setActiveContent}
                onDirtyChange={setIsDirty}
              />
            )
          )}

          {/* Always mounted: pattern extraction is a long-running task, unmounting would interrupt progress listeners */}
          <div
            className={
              activePanel === "style-samples"
                ? "flex-1 flex flex-col min-h-0"
                : "hidden"
            }
          >
            <ErrorBoundary>
              <ExtractWorkspaceView
                novelId={activeNovelId}
                focusSampleId={styleSampleFocusId}
                onFocusSampleHandled={() => setStyleSampleFocusId(null)}
              />
            </ErrorBoundary>
          </div>
          {activePanel === "characters" ? (
            <ErrorBoundary>
              <CharacterListView
                novelId={activeNovelId}
                focusId={characterFocusId}
              />
            </ErrorBoundary>
          ) : activePanel === "locations" ? (
            <ErrorBoundary>
              <LocationListView
                novelId={activeNovelId}
                focusId={locationFocusId}
              />
            </ErrorBoundary>
          ) : activePanel === "storyarcs" ? (
            <ErrorBoundary>
              <ArcListView novelId={activeNovelId} focusArcId={arcFocusId} />
            </ErrorBoundary>
          ) : activePanel === "timeline" ? (
            <ErrorBoundary>
              <TimelineView
                novelId={activeNovelId}
                focusEntryId={timelineFocusId}
              />
            </ErrorBoundary>
          ) : activePanel === "reader" ? (
            <ErrorBoundary>
              <ReaderView novelId={activeNovelId} focusId={readerFocusId} />
            </ErrorBoundary>
          ) : activePanel === "preferences" ? (
            <ErrorBoundary>
              <PreferenceView
                novelId={activeNovelId}
                focusId={preferenceFocusId}
              />
            </ErrorBoundary>
          ) : activePanel === "novel-settings" ? (
            <ErrorBoundary>
              <NovelSettingView
                novelId={activeNovelId}
                focusId={settingFocusId}
              />
            </ErrorBoundary>
          ) : activePanel === "git" ? (
            <ErrorBoundary>
              <GitCommitView file={selectedGitFile} />
            </ErrorBoundary>
          ) : activePanel === "profile" ? (
            <ErrorBoundary>
              <ProfileView />
            </ErrorBoundary>
          ) : null}

          {activePanel !== "profile" && (
            <ChatPanel
              novelId={activeNovelId}
              onApprove={handleApprove}
              onReject={handleReject}
              onApprovalFileEdit={handleApprovalFileEdit}
              chatPanelWidth={chatPanelWidth}
              onChatPanelResize={setChatPanelWidth}
            />
          )}
        </div>

        <StatusBar content={activeContent} isDirty={isDirty} />

        <SettingsDialog
          open={showSettings}
          onClose={() => setShowSettings(false)}
          initialTab="general"
        />

        <HelpDialog open={showHelp} onClose={() => setShowHelp(false)} />

        <NovelEditDialog
          open={showCreateDialog}
          onClose={() => setShowCreateDialog(false)}
          onSave={handleCreateNovelFromDialog}
        />
        <NovelEditDialog
          open={!!editingNovel}
          novel={editingNovel}
          onClose={() => setEditingNovel(null)}
          onSave={handleUpdateNovel}
        />
        <NovelDeleteDialog
          open={!!deletingNovel}
          novelTitle={deletingNovel?.title ?? ""}
          onClose={() => setDeletingNovel(null)}
          onConfirm={handleDeleteNovel}
        />

        <ExportDialog
          open={exportNovelId !== null}
          novelTitle={novels.find((n) => n.id === exportNovelId)?.title ?? ""}
          onClose={() => setExportNovelId(null)}
          onExport={handleExportNovel}
        />

        <ImportProgressDialog
          {...importNovel.dialogProps}
          modelKey={importNovel.modelKey}
          setModelKey={importNovel.setModelKey}
          modelOptions={importNovel.modelOptions}
          onStartLLM={importNovel.startLLMImport}
        />

        <UpdateDialog
          open={showUpdate}
          result={updateResult}
          onClose={() => setShowUpdate(false)}
        />
      </div>
    </RefreshContext.Provider>
  );
}

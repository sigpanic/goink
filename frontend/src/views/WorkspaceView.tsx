import { useState, useEffect, useLayoutEffect, useCallback, useRef } from "react";
import { flushSync } from "react-dom";
import { useTranslation } from "react-i18next";
import { useApp } from "@/hooks/useApp";
import type { imp, novel, chapter } from "@/hooks/useApp";
import type { git } from "@/lib/wailsjs/go/models";
import ActivityBar from "@/components/shell/ActivityBar";
import StatusBar from "@/components/shell/StatusBar";
import WindowControls from "@/components/shell/WindowControls";
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
import NovelDialogs from "@/components/novel/NovelDialogs";
import ImportProgressDialog from "@/components/novel/ImportProgressDialog";
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
import { WindowToggleMaximise } from "@/lib/wailsjs/runtime/runtime";
import Logo from "@/components/Logo";
import { useTheme, type Theme } from "@/hooks/useTheme";
import { useLayoutState } from "@/hooks/useLayoutState";
import { useWindowState } from "@/hooks/useWindowState";
import { useImportNovel } from "@/hooks/useImportNovel";
import type { PanelId, SidebarPanelId } from "@/types/panel";
import { usePanelStore } from "@/stores/usePanelStore";
import { useFocusStore } from "@/stores/useFocusStore";
import { useNovels } from "@/components/novel/useNovels";
import { useNovelStore } from "@/components/novel/useNovelStore";
import { useCreateNovel } from "@/components/novel/useCreateNovel";
import { novelKeys } from "@/lib/queryKeys";
import { useQueryClient } from "@tanstack/react-query";

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

  // novels 走 useNovels query（3.1）：替换原 novels state + loadNovels + useEffect。
  // 30s staleTime 内切面板不重复 fetch；novelsLoading 守卫「自动选小说」effect（替代 loadedRef）。
  const { data: novels = [], isLoading: novelsLoading } = useNovels();
  const queryClient = useQueryClient();
  // 小说领域 UI 状态：activeNovelId 留 WorkspaceView（路由用）；
  // 对话框开关 + setter 由 NovelDialogs 订阅（3.6）；唯独 setExportNovelId 留此
  // —— SidePanel 通过 onExportNovel 触发开 dialog，setter 引用稳定不引发重渲染。
  const activeNovelId = useNovelStore((s) => s.activeNovelId);
  const setActiveNovelId = useNovelStore((s) => s.setActiveNovelId);
  const setExportNovelId = useNovelStore((s) => s.setExportNovelId);
  // 3.7: switchNovel action（set activeNovelId + SetActiveNovel 后端）。
  // switchToNovel 改瘦 wrapper 调它；closeAllTabs 删（useEditorTabs 接管 tab 切换）。
  const switchNovel = useNovelStore((s) => s.switchNovel);
  // 3.3: 创建小说 mutation。onSuccess 失效 novelKeys.all；
  // handleCreateNovel（SidePanel 内联表单）专用，dialog 路径的 createNovel 实例在 NovelDialogs。
  const createNovel = useCreateNovel();
  // activePanel/sidebarPanel/sidebarClosed 外置到 usePanelStore（2.7）。
  // 用 selector 订阅（而非整体解构）：actions 引用稳定不触发 re-render；
  // activePanel 等值变化才 re-render，避免同值 set 引发的循环。
  const activePanel = usePanelStore((s) => s.activePanel);
  const sidebarPanel = usePanelStore((s) => s.sidebarPanel);
  const sidebarClosed = usePanelStore((s) => s.sidebarClosed);
  const setActivePanel = usePanelStore((s) => s.setActivePanel);
  const setSidebarPanel = usePanelStore((s) => s.setSidebarPanel);
  const setSidebarClosed = usePanelStore((s) => s.setSidebarClosed);
  const [searchQuery, setSearchQuery] = useState("");
  const [searchResults, setSearchResults] = useState<search.Result[]>([]);
  // focusMap 外置到 useFocusStore（2.8）。各 View 自己订阅 focusId。
  // styleSampleFocusId 语义不同（null=已处理），保留本地 state。
  const focusEntity = useFocusStore((s) => s.focusEntity);
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
  const { theme, toggle: toggleTheme } = useTheme();
  const { isMaximised, setIsMaximised } = useWindowState();
  const {
    sidePanelWidth,
    chatPanelWidth,
    setSidePanelWidth,
    setChatPanelWidth,
  } = useLayoutState();
  // 首次 mount 同步初始化 activeNovelId + activePanel（store 默认 0/"novels"，用 initialNovelId 覆盖）。
  // useLayoutEffect 在 paint 前跑，避免首屏用默认值再切换的闪烁。
  // 注：原 useState(initialNovelId) 同步初值，改 store 后 store 默认 0，必须在这里同步覆盖。
  useLayoutEffect(() => {
    setActiveNovelId(initialNovelId);
    setActivePanel(initialNovelId ? "chapters" : "novels");
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // ── 更新检查 ────────────────────────────────────────────
  const [showUpdate, setShowUpdate] = useState(false);
  const [updateResult, setUpdateResult] =
    useState<updateModels.CheckResult | null>(null);

  // ── 书籍管理弹窗（state 见 useNovelStore，3.2 外置）──────────────────────────────

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

  // 切小说瘦 wrapper（3.7）：switchNovel action 管 activeNovelId + SetActiveNovel 后端；
  // 本地重置 panel/tabTarget/activeContent/selectedGitFile（喂 SidePanel/StatusBar/GitCommitView）。
  // tab 切换由 useEditorTabs 的 novelId effect 接管，不再命令式调 closeAllTabs。
  const switchToNovel = useCallback(async (id: number) => {
    await switchNovel(id);
    setActivePanel("chapters");
    setTabTarget(null);
    setActiveContent("");
    setSelectedGitFile(null);
  }, [switchNovel, setActivePanel]);

  const handleImportedNovel = useCallback(
    async (res: imp.ImportResult) => {
      // 临时：invalidateQueries 触发 useNovels refetch；3.3 mutation 建好后由 onSuccess 接管。
      await queryClient.invalidateQueries({ queryKey: novelKeys.all });
      await switchToNovel(res.novel_id);
    },
    [queryClient, switchToNovel],
  );

  const importNovel = useImportNovel({ app, onImported: handleImportedNovel });

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
    if (novelsLoading) return;
    const exists = novels.find((n) => n.id === activeNovelId);
    if (!exists && novels.length > 0) {
      const first = novels[0];
      setActiveNovelId(first.id);
      setActivePanel("chapters");
      app.SetActiveNovel({ novel_id: first.id });
    } else if (novels.length === 0) {
      setActivePanel("novels");
    }
    // 3.9 fix: 只在 novels 变化时触发（不加 activeNovelId）。
    // 否则新建小说时 switchToNovel 设 activeNovelId=新小说，但 useNovels refetch 未完，
    // novels 旧列表不含新小说 → find 失败 → 误选 novels[0]（旧小说）。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [app, novels]);

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

  // 4b: 接 type 透传给 focusEntity（storyarc 全局搜索区分 arc/node 跳转，其他领域 undefined）。
  function handleSearchNavigateEntity(
    panelId: PanelId,
    entityId: number,
    type?: "arc" | "node",
  ) {
    focusEntity(panelId, entityId, type);
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
      await switchToNovel(n.id);
    } catch (err) {
      console.error(err);
    }
  }

  async function handleCreateNovel() {
    try {
      if (!title.trim()) return;
      // 3.3: 改用 mutation，invalidate 由 onSuccess 接管；switchToNovel + 清表单留 handler。
      const n = await createNovel.mutateAsync({
        title: title.trim(),
        description: description.trim(),
      });
      setTitle("");
      setDescription("");
      setShowCreate(false);
      await switchToNovel(n.id);
    } catch (err) {
      console.error(err);
    }
  }

  // 3.6: 4 个对话框 handler（create-dialog/update/delete/export）已移到 NovelDialogs。
  // handleCreateNovel（SidePanel 内联表单）留此——它不走 dialog，直接 mutateAsync + switchToNovel。

  async function handleSaveCover(novelID: number, file: File) {
    const buf = await file.arrayBuffer();
    await app.SaveCover(novelID, Array.from(new Uint8Array(buf)));
  }

  const activeNovel = novels.find((n) => n.id === activeNovelId);

  return (
    <>
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
            <WindowControls
              platformOS={platformOS}
              isMaximised={isMaximised}
              setIsMaximised={setIsMaximised}
            />
          </div>
        </header>

        <div className="flex-1 flex min-h-0 overflow-hidden">
          <ActivityBar onSelect={handleActivitySelect} />

          {!sidebarClosed && (
            <SidePanel
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
              onSelectNovel={handleSelectNovel}
              onSaveCover={handleSaveCover}
              onImportNovel={() => importNovel.startImport()}
            />
          ) : (
            CONTENT_PANEL_IDS.has(activePanel) && (
              <ContentPanel
                ref={contentRef}
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
              <CharacterListView novelId={activeNovelId} />
            </ErrorBoundary>
          ) : activePanel === "locations" ? (
            <ErrorBoundary>
              <LocationListView novelId={activeNovelId} />
            </ErrorBoundary>
          ) : activePanel === "storyarcs" ? (
            <ErrorBoundary>
              <ArcListView novelId={activeNovelId} />
            </ErrorBoundary>
          ) : activePanel === "timeline" ? (
            <ErrorBoundary>
              <TimelineView novelId={activeNovelId} />
            </ErrorBoundary>
          ) : activePanel === "reader" ? (
            <ErrorBoundary>
              <ReaderView novelId={activeNovelId} />
            </ErrorBoundary>
          ) : activePanel === "preferences" ? (
            <ErrorBoundary>
              <PreferenceView novelId={activeNovelId} />
            </ErrorBoundary>
          ) : activePanel === "novel-settings" ? (
            <ErrorBoundary>
              <NovelSettingView novelId={activeNovelId} />
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

        <NovelDialogs switchToNovel={switchToNovel} />

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
    </>
  );
}

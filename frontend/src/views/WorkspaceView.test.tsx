import { describe, it, expect, vi, beforeEach } from "vitest";
import { render as originalRender, screen, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactElement } from "react";
import WorkspaceView from "./WorkspaceView";
import { useFocusStore } from "@/stores/useFocusStore";
import { useNovelStore } from "@/components/novel/useNovelStore";

// 3.1 useNovels 引入 useQuery，render 需包 QueryClientProvider。
// 每个测试用独立 QueryClient（retry:false 避免重试），无状态残留。
function render(ui: ReactElement) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return originalRender(ui, {
    wrapper: ({ children }) => (
      <QueryClientProvider client={qc}>{children}</QueryClientProvider>
    ),
  });
}

// 3.2: useNovelStore 是全局单例，跨测试用例有状态残留（同 useFocusStore 问题）。
// 顶层 beforeEach 重置所有 5 个字段，避免上个用例的 activeNovelId/showCreateDialog 等影响下个用例。
// initialNovelId 由 useLayoutEffect 在 mount 时同步覆盖，重置 activeNovelId=0 不影响 initialNovelId 测试。
beforeEach(() => {
  useNovelStore.setState({
    activeNovelId: 0,
    editingNovel: null,
    deletingNovel: null,
    showCreateDialog: false,
    exportNovelId: null,
  });
});

// 覆盖 setup.ts 的 Proxy mock（多 vi.mock 交互时 Proxy 会报错），改普通对象
// 3.1 useNovels / 3.3 useCreateNovel 直接 import wailsjs（绕过 useApp），这里要 mock GetNovels/CreateNovel。
// mockGetNovels/mockCreateNovel 用 vi.hoisted 提升，让 vi.mock 工厂能引用（vi.mock 自身被提升到文件顶部）。
const { mockGetNovels, mockCreateNovel } = vi.hoisted(() => ({
  mockGetNovels: vi.fn(),
  mockCreateNovel: vi.fn(),
}));

vi.mock("@/lib/wailsjs/go/app/App", () => ({
  CheckUpdate: vi.fn().mockResolvedValue(null),
  GetNovels: mockGetNovels,
  CreateNovel: mockCreateNovel,
}));

// ── Mock useApp（关键异步方法返回 Promise，避免 .then 报错）──────────
const mockSetActiveNovel = vi.fn();
const mockGetPlatform = vi.fn();
const mockApproveTool = vi.fn();

vi.mock("@/hooks/useApp", () => ({
  useApp: () => ({
    GetNovels: mockGetNovels,
    SetActiveNovel: mockSetActiveNovel,
    GetPlatform: mockGetPlatform,
    ApproveTool: mockApproveTool,
    CreateNovel: mockCreateNovel,
    UpdateNovel: vi.fn(),
    DeleteNovel: vi.fn(),
    ExportNovel: vi.fn(),
    SaveCover: vi.fn(),
  }),
}));

vi.mock("@/hooks/useTheme", () => ({
  useTheme: () => ({ theme: "light", toggle: vi.fn() }),
}));

vi.mock("@/hooks/useLayoutState", () => ({
  useLayoutState: () => ({
    sidePanelWidth: 280,
    chatPanelWidth: 480,
    setSidePanelWidth: vi.fn(),
    setChatPanelWidth: vi.fn(),
  }),
}));

vi.mock("@/hooks/useWindowState", () => ({
  useWindowState: () => ({ isMaximised: false, setIsMaximised: vi.fn() }),
}));

vi.mock("@/hooks/useImportNovel", () => ({
  useImportNovel: ({
    onImported,
  }: {
    onImported: (res: { novel_id: number }) => void;
  }) => ({
    startImport: () => onImported({ novel_id: 3 }),
    dialogProps: { open: false },
    modelKey: "",
    setModelKey: vi.fn(),
    modelOptions: [],
    startLLMImport: vi.fn(),
  }),
}));

// ── ContentPanel ref spy（forwardRef + useImperativeHandle）──────────
// 暴露 4 个方法 spy，让测试能验证 contentRef.current.openFileWithHighlight 等被调用
const contentRefSpies = vi.hoisted(() => ({
  openFile: vi.fn(),
  openFileWithHighlight: vi.fn(),
  clearHighlight: vi.fn(),
  closeAllTabs: vi.fn(),
  openDiffTab: vi.fn(),
  handleDiffApprove: vi.fn().mockResolvedValue(undefined),
  handleDiffReject: vi.fn(),
}));

vi.mock("@/components/content/ContentPanel", async () => {
  const { forwardRef, useImperativeHandle } = await import("react");
  return {
    default: forwardRef<unknown, unknown>((_props, ref) => {
      useImperativeHandle(ref, () => contentRefSpies);
      return <div data-testid="content-panel">content-panel</div>;
    }),
  };
});

// ── Mock 子组件为带 testid 的 stub，测路由不测子组件内部 ──────────────
vi.mock("@/components/shell/ActivityBar", () => ({
  default: ({ onSelect }: { onSelect: (id: string) => void }) => (
    <div data-testid="activity-bar">
      <button onClick={() => onSelect("novels")}>btn-novels</button>
      <button onClick={() => onSelect("chapters")}>btn-chapters</button>
      <button onClick={() => onSelect("characters")}>btn-characters</button>
      <button onClick={() => onSelect("locations")}>btn-locations</button>
      <button onClick={() => onSelect("profile")}>btn-profile</button>
    </div>
  ),
}));

vi.mock("@/components/shell/StatusBar", () => ({
  default: () => <div data-testid="status-bar">status-bar</div>,
}));

// SidePanel mock：接收搜索导航回调，渲染按钮触发（测 handleSearchNavigate* 路径）
vi.mock("@/components/sidebar/SidePanel", () => ({
  default: (props: {
    onSelectNovel?: (n: { id: number; title: string }) => void;
    onSearchNavigateEntity?: (
      panelId: string,
      entityId: number,
    ) => void;
    onSearchNavigateChapter?: (
      filePath: string,
      title: string,
      num: number,
      matchPos: number,
      matchLen: number,
    ) => void;
  }) => (
    <div data-testid="side-panel">
      <button
        onClick={() => props.onSearchNavigateEntity?.("characters", 5)}
      >
        nav-entity-characters
      </button>
      <button
        onClick={() => props.onSearchNavigateEntity?.("locations", 7)}
      >
        nav-entity-locations
      </button>
      <button
        onClick={() => props.onSearchNavigateEntity?.("timeline", 9)}
      >
        nav-entity-timeline
      </button>
      <button
        onClick={() => props.onSearchNavigateEntity?.("storyarcs", 11)}
      >
        nav-entity-storyarcs
      </button>
      <button onClick={() => props.onSearchNavigateEntity?.("reader", 13)}>
        nav-entity-reader
      </button>
      <button
        onClick={() =>
          props.onSearchNavigateChapter?.("path/ch1.md", "第一章", 1, 10, 5)
        }
      >
        nav-chapter-highlight
      </button>
      <button
        onClick={() =>
          props.onSearchNavigateChapter?.("path/ch2.md", "第二章", 2, -1, 0)
        }
      >
        nav-chapter-no-highlight
      </button>
      <button
        onClick={() =>
          props.onSearchNavigateChapter?.("path/ch3.md", "第三章", 3, 0, 5)
        }
      >
        nav-chapter-pos0
      </button>
      <button onClick={() => props.onSelectNovel?.({ id: 2, title: "小说2" })}>
        nav-select-novel
      </button>
    </div>
  ),
}));

// 各 View mock：接收 focusId 类 prop 渲染出来，验证 focusId 传递正确
vi.mock("@/components/character/CharacterListView", () => ({
  default: function CharacterListViewMock() {
    const focusId = useFocusStore((s) => s.focusMap.characters ?? 0);
    return (
      <div data-testid="character-list" data-focusid={focusId}>
        character-list
      </div>
    );
  },
}));
vi.mock("@/components/location/LocationListView", () => ({
  default: function LocationListViewMock() {
    const focusId = useFocusStore((s) => s.focusMap.locations ?? 0);
    return (
      <div data-testid="location-list" data-focusid={focusId}>
        location-list
      </div>
    );
  },
}));
vi.mock("@/components/storyarc/ArcListView", () => ({
  default: function ArcListViewMock() {
    const focusArcId = useFocusStore((s) => s.focusMap.storyarcs ?? 0);
    return (
      <div data-testid="arc-list" data-focusarcid={focusArcId}>
        arc-list
      </div>
    );
  },
}));
vi.mock("@/components/timeline/TimelineView", () => ({
  default: function TimelineViewMock() {
    const focusEntryId = useFocusStore((s) => s.focusMap.timeline ?? 0);
    return (
      <div data-testid="timeline" data-focusentryid={focusEntryId}>
        timeline
      </div>
    );
  },
}));
vi.mock("@/components/reader/ReaderView", () => ({
  default: function ReaderViewMock() {
    const focusId = useFocusStore((s) => s.focusMap.reader ?? 0);
    return (
      <div data-testid="reader" data-focusid={focusId}>
        reader
      </div>
    );
  },
}));
vi.mock("@/components/preference/PreferenceView", () => ({
  default: () => <div data-testid="preference">preference</div>,
}));
vi.mock("@/components/novel-setting/NovelSettingView", () => ({
  default: () => <div data-testid="novel-setting">novel-setting</div>,
}));
vi.mock("@/components/novel/BookshelfView", () => ({
  // 3.2: BookshelfView 内部订阅 useNovelStore.setShowCreateDialog（不再通过 prop 接收 onCreateNovel）。
  // mock 也要订阅，模拟"点新建按钮 → 打开 dialog"行为。
  default: function BookshelfViewMock(props: {
    onSelectNovel?: (n: { id: number; title: string }) => void;
    onImportNovel?: () => void;
  }) {
    const setShowCreateDialog = useNovelStore((s) => s.setShowCreateDialog);
    return (
      <div data-testid="bookshelf">
        <button onClick={() => props.onSelectNovel?.({ id: 2, title: "小说2" })}>
          shelf-select-novel
        </button>
        <button onClick={() => setShowCreateDialog(true)}>shelf-create-novel</button>
        <button onClick={() => props.onImportNovel?.()}>shelf-import-novel</button>
      </div>
    );
  },
}));
vi.mock("@/components/chat/ChatPanel", () => ({
  default: (props: {
    onApprove?: (toolId: string, feedback: string) => void;
    onReject?: (toolId: string, feedback: string) => void;
  }) => (
    <div data-testid="chat-panel">
      <button onClick={() => props.onApprove?.("tool-1", "looks good")}>
        approve-btn
      </button>
      <button onClick={() => props.onReject?.("tool-2", "needs rework")}>
        reject-btn
      </button>
    </div>
  ),
}));
vi.mock("@/components/profile/ProfileView", () => ({
  default: () => <div data-testid="profile">profile</div>,
}));
vi.mock("@/components/git/GitCommitView", () => ({
  default: () => <div data-testid="git">git</div>,
}));
vi.mock("@/components/extract/ExtractWorkspaceView", () => ({
  default: () => <div data-testid="extract">extract</div>,
}));
vi.mock("@/components/shell/GitHubLink", () => ({ default: () => null }));
vi.mock("@/components/settings/SettingsDialog", () => ({ default: () => null }));
vi.mock("@/components/help/HelpDialog", () => ({ default: () => null }));
vi.mock("@/components/novel/NovelEditDialog", () => ({
  default: (props: {
    open?: boolean;
    onSave?: (input: { title: string; description: string; genre: string }) => void;
  }) =>
    props.open ? (
      <div data-testid="novel-edit-dialog">
        <button
          onClick={() =>
            props.onSave?.({ title: "新小说", description: "描述", genre: "" })
          }
        >
          dialog-save-novel
        </button>
      </div>
    ) : null,
}));
vi.mock("@/components/novel/NovelDeleteDialog", () => ({ default: () => null }));
vi.mock("@/components/novel/ImportProgressDialog", () => ({ default: () => null }));
vi.mock("@/components/export/ExportDialog", () => ({ default: () => null }));
vi.mock("@/components/update/UpdateDialog", () => ({ default: () => null }));
vi.mock("@/components/Logo", () => ({ default: () => null }));
// ErrorBoundary 保留真实（简单包裹组件）

describe("WorkspaceView panel switching", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetNovels.mockResolvedValue([]);
    mockGetPlatform.mockResolvedValue({ os: "linux" });
    mockSetActiveNovel.mockResolvedValue(undefined);
    mockApproveTool.mockResolvedValue(undefined);
  });

  it("无小说时默认渲染书架（initialNovelId=0）", async () => {
    render(<WorkspaceView initialNovelId={0} />);
    expect(await screen.findByTestId("bookshelf")).toBeInTheDocument();
  });

  it("有小说时渲染内容面板（initialNovelId 非 0）", async () => {
    mockGetNovels.mockResolvedValue([{ id: 1, title: "测试小说" }]);
    render(<WorkspaceView initialNovelId={1} />);
    expect(await screen.findByTestId("content-panel")).toBeInTheDocument();
  });

  it("点 characters 面板渲染角色列表", async () => {
    mockGetNovels.mockResolvedValue([{ id: 1, title: "测试小说" }]);
    render(<WorkspaceView initialNovelId={1} />);
    await screen.findByTestId("content-panel");
    fireEvent.click(screen.getByText("btn-characters"));
    expect(await screen.findByTestId("character-list")).toBeInTheDocument();
  });

  it("点 locations 面板渲染地点列表", async () => {
    mockGetNovels.mockResolvedValue([{ id: 1, title: "测试小说" }]);
    render(<WorkspaceView initialNovelId={1} />);
    await screen.findByTestId("content-panel");
    fireEvent.click(screen.getByText("btn-locations"));
    expect(await screen.findByTestId("location-list")).toBeInTheDocument();
  });

  it("点 profile 面板渲染个人资料且不渲染 ChatPanel", async () => {
    mockGetNovels.mockResolvedValue([{ id: 1, title: "测试小说" }]);
    render(<WorkspaceView initialNovelId={1} />);
    await screen.findByTestId("content-panel");
    fireEvent.click(screen.getByText("btn-profile"));
    expect(await screen.findByTestId("profile")).toBeInTheDocument();
    expect(screen.queryByTestId("chat-panel")).not.toBeInTheDocument();
  });
});

describe("WorkspaceView search navigation", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useFocusStore.setState({ focusMap: {} });
    mockGetNovels.mockResolvedValue([{ id: 1, title: "测试小说" }]);
    mockGetPlatform.mockResolvedValue({ os: "linux" });
    mockSetActiveNovel.mockResolvedValue(undefined);
    mockApproveTool.mockResolvedValue(undefined);
  });

  it("entity 跳转 characters 设置 focusId 并切面板", async () => {
    render(<WorkspaceView initialNovelId={1} />);
    await screen.findByTestId("content-panel");
    fireEvent.click(screen.getByText("nav-entity-characters"));
    const list = await screen.findByTestId("character-list");
    expect(list).toBeInTheDocument();
    expect(list).toHaveAttribute("data-focusid", "5");
  });

  it("entity 跳转 locations 设置 focusId", async () => {
    render(<WorkspaceView initialNovelId={1} />);
    await screen.findByTestId("content-panel");
    fireEvent.click(screen.getByText("nav-entity-locations"));
    expect(await screen.findByTestId("location-list")).toHaveAttribute(
      "data-focusid",
      "7",
    );
  });

  it("entity 跳转 timeline 设置 focusEntryId", async () => {
    render(<WorkspaceView initialNovelId={1} />);
    await screen.findByTestId("content-panel");
    fireEvent.click(screen.getByText("nav-entity-timeline"));
    expect(await screen.findByTestId("timeline")).toHaveAttribute(
      "data-focusentryid",
      "9",
    );
  });

  it("entity 跳转 storyarcs 设置 focusArcId", async () => {
    render(<WorkspaceView initialNovelId={1} />);
    await screen.findByTestId("content-panel");
    fireEvent.click(screen.getByText("nav-entity-storyarcs"));
    expect(await screen.findByTestId("arc-list")).toHaveAttribute(
      "data-focusarcid",
      "11",
    );
  });

  it("entity 跳转 reader 设置 focusId", async () => {
    render(<WorkspaceView initialNovelId={1} />);
    await screen.findByTestId("content-panel");
    fireEvent.click(screen.getByText("nav-entity-reader"));
    expect(await screen.findByTestId("reader")).toHaveAttribute(
      "data-focusid",
      "13",
    );
  });

  it("chapter 跳转有高亮调 openFileWithHighlight", async () => {
    render(<WorkspaceView initialNovelId={1} />);
    await screen.findByTestId("content-panel");
    fireEvent.click(screen.getByText("nav-chapter-highlight"));
    expect(contentRefSpies.openFileWithHighlight).toHaveBeenCalledWith(
      "path/ch1.md",
      "第一章",
      10,
      5,
    );
    expect(contentRefSpies.openFile).not.toHaveBeenCalled();
  });

  it("chapter 跳转无高亮(matchPos=-1)调 openFile", async () => {
    render(<WorkspaceView initialNovelId={1} />);
    await screen.findByTestId("content-panel");
    fireEvent.click(screen.getByText("nav-chapter-no-highlight"));
    expect(contentRefSpies.openFile).toHaveBeenCalledWith(
      "path/ch2.md",
      "第二章",
    );
    expect(contentRefSpies.openFileWithHighlight).not.toHaveBeenCalled();
  });

  it("chapter 跳转 matchPos=0 仍调 openFileWithHighlight（position 0 合法）", async () => {
    render(<WorkspaceView initialNovelId={1} />);
    await screen.findByTestId("content-panel");
    fireEvent.click(screen.getByText("nav-chapter-pos0"));
    expect(contentRefSpies.openFileWithHighlight).toHaveBeenCalledWith(
      "path/ch3.md",
      "第三章",
      0,
      5,
    );
  });
});

describe("WorkspaceView approval bridge", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetNovels.mockResolvedValue([{ id: 1, title: "测试小说" }]);
    mockGetPlatform.mockResolvedValue({ os: "linux" });
    mockSetActiveNovel.mockResolvedValue(undefined);
    mockApproveTool.mockResolvedValue(undefined);
  });

  it("approve 调 ApproveTool(true) + handleDiffApprove", async () => {
    render(<WorkspaceView initialNovelId={1} />);
    await screen.findByTestId("content-panel");
    fireEvent.click(screen.getByText("approve-btn"));
    await vi.waitFor(() => {
      expect(mockApproveTool).toHaveBeenCalledWith("tool-1", true, "looks good");
    });
    expect(contentRefSpies.handleDiffApprove).toHaveBeenCalledWith("tool-1");
  });

  it("reject 调 ApproveTool(false) + handleDiffReject", async () => {
    render(<WorkspaceView initialNovelId={1} />);
    await screen.findByTestId("content-panel");
    fireEvent.click(screen.getByText("reject-btn"));
    await vi.waitFor(() => {
      expect(mockApproveTool).toHaveBeenCalledWith("tool-2", false, "needs rework");
    });
    expect(contentRefSpies.handleDiffReject).toHaveBeenCalledWith("tool-2");
  });
});

describe("WorkspaceView switchNovel state reset", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetNovels.mockResolvedValue([
      { id: 1, title: "小说1" },
      { id: 2, title: "小说2" },
    ]);
    mockGetPlatform.mockResolvedValue({ os: "linux" });
    mockSetActiveNovel.mockResolvedValue(undefined);
    mockApproveTool.mockResolvedValue(undefined);
    mockCreateNovel.mockResolvedValue({ id: 5, title: "新小说" });
  });

  it("侧栏选小说调 SetActiveNovel + closeAllTabs（ContentPanel 保持挂载）", async () => {
    render(<WorkspaceView initialNovelId={1} />);
    await screen.findByTestId("content-panel");
    fireEvent.click(screen.getByText("nav-select-novel"));
    await vi.waitFor(() => {
      expect(mockSetActiveNovel).toHaveBeenCalledWith({ novel_id: 2 });
    });
    expect(contentRefSpies.closeAllTabs).toHaveBeenCalled();
    // 切小说后 activePanel 仍为 chapters，content-panel 保持可见
    expect(screen.getByTestId("content-panel")).toBeInTheDocument();
  });

  it("书架导入小说回调调 loadNovels + SetActiveNovel", async () => {
    render(<WorkspaceView initialNovelId={1} />);
    await screen.findByTestId("content-panel");
    // 切到书架触发导入（ContentPanel 卸载，closeAllTabs 不该调用）
    fireEvent.click(screen.getByText("btn-novels"));
    expect(await screen.findByTestId("bookshelf")).toBeInTheDocument();
    fireEvent.click(screen.getByText("shelf-import-novel"));
    await vi.waitFor(() => {
      expect(mockSetActiveNovel).toHaveBeenCalledWith({ novel_id: 3 });
    });
    // 导入回调触发 loadNovels 重拉（mount + 导入后至少 2 次）
    expect(mockGetNovels.mock.calls.length).toBeGreaterThanOrEqual(2);
  });

  it("dialog 创建小说调 CreateNovel + SetActiveNovel", async () => {
    mockGetNovels.mockResolvedValue([]);
    render(<WorkspaceView initialNovelId={0} />);
    expect(await screen.findByTestId("bookshelf")).toBeInTheDocument();
    // 打开创建 dialog
    fireEvent.click(screen.getByText("shelf-create-novel"));
    expect(await screen.findByTestId("novel-edit-dialog")).toBeInTheDocument();
    // 保存
    fireEvent.click(screen.getByText("dialog-save-novel"));
    // SetActiveNovel 在 CreateNovel + loadNovels 之后，等它即覆盖全链路
    await vi.waitFor(() => {
      expect(mockSetActiveNovel).toHaveBeenCalledWith({ novel_id: 5 });
    });
    expect(mockCreateNovel).toHaveBeenCalledWith({
      title: "新小说",
      description: "描述",
      genre: "",
    });
  });
});

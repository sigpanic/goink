import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import WorkspaceView from "./WorkspaceView";

// 覆盖 setup.ts 的 Proxy mock（多 vi.mock 交互时 Proxy 会报错），改普通对象
vi.mock("@/lib/wailsjs/go/app/App", () => ({
  CheckUpdate: vi.fn().mockResolvedValue(null),
}));

// ── Mock useApp（关键异步方法返回 Promise，避免 .then 报错）──────────
const mockGetNovels = vi.fn();
const mockSetActiveNovel = vi.fn();
const mockGetPlatform = vi.fn();
const mockApproveTool = vi.fn();

vi.mock("@/hooks/useApp", () => ({
  useApp: () => ({
    GetNovels: mockGetNovels,
    SetActiveNovel: mockSetActiveNovel,
    GetPlatform: mockGetPlatform,
    ApproveTool: mockApproveTool,
    CreateNovel: vi.fn(),
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
  useImportNovel: () => ({
    startImport: vi.fn(),
    dialogProps: { open: false },
    modelKey: "",
    setModelKey: vi.fn(),
    modelOptions: [],
    startLLMImport: vi.fn(),
  }),
}));

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
vi.mock("@/components/sidebar/SidePanel", () => ({
  default: () => <div data-testid="side-panel">side-panel</div>,
}));
vi.mock("@/components/content/ContentPanel", () => ({
  default: () => <div data-testid="content-panel">content-panel</div>,
}));
vi.mock("@/components/character/CharacterListView", () => ({
  default: () => <div data-testid="character-list">character-list</div>,
}));
vi.mock("@/components/location/LocationListView", () => ({
  default: () => <div data-testid="location-list">location-list</div>,
}));
vi.mock("@/components/storyarc/ArcListView", () => ({
  default: () => <div data-testid="arc-list">arc-list</div>,
}));
vi.mock("@/components/timeline/TimelineView", () => ({
  default: () => <div data-testid="timeline">timeline</div>,
}));
vi.mock("@/components/reader/ReaderView", () => ({
  default: () => <div data-testid="reader">reader</div>,
}));
vi.mock("@/components/preference/PreferenceView", () => ({
  default: () => <div data-testid="preference">preference</div>,
}));
vi.mock("@/components/novel-setting/NovelSettingView", () => ({
  default: () => <div data-testid="novel-setting">novel-setting</div>,
}));
vi.mock("@/components/novel/BookshelfView", () => ({
  default: () => <div data-testid="bookshelf">bookshelf</div>,
}));
vi.mock("@/components/chat/ChatPanel", () => ({
  default: () => <div data-testid="chat-panel">chat-panel</div>,
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
vi.mock("@/components/novel/NovelEditDialog", () => ({ default: () => null }));
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

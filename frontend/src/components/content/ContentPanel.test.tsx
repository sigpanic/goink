import { describe, it, expect, vi, beforeEach } from "vitest";
import { render as originalRender, screen, fireEvent, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactElement } from "react";
import ContentPanel, { type ContentPanelHandle } from "./ContentPanel";
import { toastError } from "@/utils/toast";

// 5.2 commit 1: useFileContent 引入 useQueryClient，render 需包 QueryClientProvider。
// 每个测试用独立 QueryClient（retry:false 避免重试），无状态残留。
function render(ui: ReactElement) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return originalRender(ui, {
    wrapper: ({ children }) => (
      <QueryClientProvider client={qc}>{children}</QueryClientProvider>
    ),
  });
}

// Mock toastError
vi.mock("@/utils/toast", async (importOriginal) => {
  const mod = await importOriginal<typeof import("@/utils/toast")>();
  return {
    ...mod,
    toastError: vi.fn(),
  };
});

// Mock child components
vi.mock("./TabBar", () => ({
  default: ({ tabs }: any) => (
    <div data-testid="tab-bar">
      {tabs.map((t: any) => (
        <span key={t.id}>{t.title}</span>
      ))}
    </div>
  ),
}));

vi.mock("./ContentEditor", () => ({
  default: ({ value }: any) => <div data-testid="content-editor">{value}</div>,
}));

vi.mock("./OutlineViewer", () => ({
  default: ({ content }: any) => (
    <div data-testid="outline-viewer">{content}</div>
  ),
}));

vi.mock("./SkillPreview", () => ({
  default: ({ content }: any) => (
    <div data-testid="skill-preview">{content}</div>
  ),
}));

vi.mock("@/components/skill/SkillEditForm", () => ({
  default: ({ content, onSave }: any) => (
    <div data-testid="skill-edit-form">
      {content}
      <button onClick={() => onSave(content)}>save</button>
    </div>
  ),
}));

vi.mock("@/components/Markdown", () => ({
  default: ({ content }: any) => <div data-testid="markdown">{content}</div>,
}));

vi.mock("@monaco-editor/react", () => ({
  DiffEditor: ({ original, modified }: any) => (
    <div data-testid="diff-editor">
      <span>{original}</span>
      <span>{modified}</span>
    </div>
  ),
}));

// Mock useTheme
vi.mock("@/hooks/useTheme", () => ({
  useTheme: () => ({ theme: "light" as const }),
}));

// Mock useEditorTabs
const mockOpenTab = vi.fn();
const mockCloseTab = vi.fn();
const mockCloseAllTabs = vi.fn();
const mockSetActiveTabId = vi.fn();
const mockUpdateTab = vi.fn();
const mockOpenDiffTab = vi.fn();

let mockTabsState: any[] = [];
let mockActiveTabIdState: string | null = null;
let mockInitRefValue = true;

vi.mock("@/hooks/useEditorTabs", () => ({
  useEditorTabs: () => ({
    tabs: mockTabsState,
    activeTab:
      mockTabsState.find((t: any) => t.id === mockActiveTabIdState) ?? null,
    activeTabId: mockActiveTabIdState,
    openTab: mockOpenTab,
    closeTab: mockCloseTab,
    closeAllTabs: mockCloseAllTabs,
    setActiveTabId: mockSetActiveTabId,
    updateTab: mockUpdateTab,
    openDiffTab: mockOpenDiffTab,
    initRef: { current: mockInitRefValue },
  }),
}));

// Mock EventsOn to return an unsubscribe function
vi.mock("@/lib/wailsjs/runtime/runtime", () => ({
  EventsOn: vi.fn(() => vi.fn()),
  EventsOff: vi.fn(),
  EventsEmit: vi.fn(),
  WindowMinimise: vi.fn(),
  WindowToggleMaximise: vi.fn(),
  Quit: vi.fn(),
}));

// 5.2 commit 3: ContentPanel 不再直接 import GetContent，file:changed handler 改走
// qc.invalidateQueries + fetchContent（query 缓存通道）。GetContent mock 不再需要。

// 5.2 commit 1: useFileContent mock（fetchContent 走 query 缓存通道，不经 useApp）
const { mockFetchContent } = vi.hoisted(() => ({
  mockFetchContent: vi.fn(),
}));
vi.mock("./useFileContent", () => ({
  useFileContent: () => ({ fetchContent: mockFetchContent }),
}));

// 5.2 commit 2: useSaveContent mutation mock（替代 useApp.SaveContent）。
// mutateAsync 单参 input（含 novel_id + path + content），对齐 doSave 调用。
const { mockSaveContent } = vi.hoisted(() => ({
  mockSaveContent: vi.fn(),
}));
vi.mock("./useSaveContent", () => ({
  useSaveContent: () => ({ mutateAsync: mockSaveContent }),
}));

// 3.8: ContentPanel 从 useNovelStore 订阅 activeNovelId（替代 prop）。mock 提供固定值 1。
vi.mock("@/components/novel/useNovelStore", () => ({
  useNovelStore: (selector: any) => selector({ activeNovelId: 1 }),
}));

describe("ContentPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockTabsState = [];
    mockActiveTabIdState = null;
    mockInitRefValue = true;
    // 5.2 commit 1: GetContent 走 useFileContent.fetchContent（query 缓存通道）
    mockFetchContent.mockResolvedValue("file content");
    mockSaveContent.mockResolvedValue(undefined);
  });

  it("renders empty state when no tabs", () => {
    render(<ContentPanel />);
    expect(
      screen.getByText("content.selectOrCreateChapter"),
    ).toBeInTheDocument();
  });

  it("renders tab select hint when tabs exist but no active tab", () => {
    mockTabsState = [
      { id: "f1", type: "file", path: "chapters/001.md", title: "Ch1" },
    ];
    mockActiveTabIdState = null;
    render(<ContentPanel />);
    expect(screen.getByText("content.selectTab")).toBeInTheDocument();
  });

  it("shows toastError when save fails", async () => {
    mockSaveContent.mockRejectedValue(new Error("disk full"));
    mockTabsState = [
      {
        id: "f1",
        type: "file",
        path: "skills/test.md",
        title: "Test",
        content: "skill content",
        viewMode: "edit",
        readOnly: false,
      },
    ];
    mockActiveTabIdState = "f1";
    mockUpdateTab.mockImplementation(() => {});

    render(<ContentPanel />);

    // Click the save button in SkillEditForm mock — triggers doSave
    const saveBtn = screen.getByText("save");
    await act(async () => {
      fireEvent.click(saveBtn);
    });

    await vi.waitFor(() => {
      expect(toastError).toHaveBeenCalledWith("common.saveFailed: disk full");
    });
  });

  it("calls GetContent when opening a file via ref", async () => {
    // 5.2 commit 1: GetContent 走 useFileContent.fetchContent
    mockFetchContent.mockResolvedValue("# Chapter 1");
    mockOpenTab.mockImplementation((tab: any) => {
      mockTabsState = [{ ...tab, id: "f1" }];
      mockActiveTabIdState = "f1";
    });

    const ref = { current: null as ContentPanelHandle | null };
    render(<ContentPanel ref={ref} />);

    await act(async () => {
      ref.current?.openFile("chapters/001.md", "Chapter 1");
    });

    expect(mockFetchContent).toHaveBeenCalledWith(1, "chapters/001.md");
  });

  it("opens file with empty content on GetContent failure", async () => {
    // 5.2 commit 1: fetchContent 失败时 tab 塞空内容（保留原 behavior）
    mockFetchContent.mockRejectedValue(new Error("not found"));
    mockOpenTab.mockImplementation((tab: any) => {
      mockTabsState = [{ ...tab, id: "f1" }];
      mockActiveTabIdState = "f1";
    });

    const ref = { current: null as ContentPanelHandle | null };
    render(<ContentPanel ref={ref} />);

    await act(async () => {
      ref.current?.openFile("chapters/001.md", "Chapter 1");
    });

    // Should still open the tab with empty content
    expect(mockOpenTab).toHaveBeenCalledWith(
      expect.objectContaining({ content: "", path: "chapters/001.md" }),
    );
  });

  it("renders content editor for file tab in content viewMode", () => {
    mockTabsState = [
      {
        id: "f1",
        type: "file",
        path: "chapters/001.md",
        title: "Ch1",
        content: "hello world",
        viewMode: "content",
      },
    ];
    mockActiveTabIdState = "f1";

    render(<ContentPanel />);
    expect(screen.getByTestId("content-editor")).toBeInTheDocument();
    expect(screen.getByText("hello world")).toBeInTheDocument();
  });

  it("renders skill preview for skill path in preview viewMode", () => {
    mockTabsState = [
      {
        id: "f1",
        type: "file",
        path: "skills/test.md",
        title: "Test Skill",
        content: "skill content",
        viewMode: "preview",
      },
    ];
    mockActiveTabIdState = "f1";

    render(<ContentPanel />);
    expect(screen.getByTestId("skill-preview")).toBeInTheDocument();
  });

  it("renders diff editor for diff tab", () => {
    mockTabsState = [
      {
        id: "d1",
        type: "diff",
        path: "chapters/001.md",
        title: "Diff",
        original: "old content",
        modified: "new content",
      },
    ];
    mockActiveTabIdState = "d1";

    render(<ContentPanel />);
    expect(screen.getByTestId("diff-editor")).toBeInTheDocument();
  });

  it("calls closeAllTabs via ref", async () => {
    const ref = { current: null as ContentPanelHandle | null };
    render(<ContentPanel ref={ref} />);

    await act(async () => {
      ref.current?.closeAllTabs();
    });

    expect(mockCloseAllTabs).toHaveBeenCalled();
  });
});

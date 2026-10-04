import { describe, it, expect, vi, beforeEach } from "vitest";
import {
  render as originalRender,
  screen,
  fireEvent,
  act,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactElement } from "react";
import ContentPanel, { type ContentPanelHandle } from "./ContentPanel";
import { useEditorTabsStore } from "./useEditorTabsStore";
import type { EditorTab } from "./types";
import { toastError } from "@/utils/toast";
import { EventsOn } from "@/lib/wailsjs/runtime/runtime";

const { mockGetChapters, mockGetContent } = vi.hoisted(() => ({
  mockGetChapters: vi.fn(),
  mockGetContent: vi.fn(),
}));
vi.mock("@/lib/wailsjs/go/app/App", () => ({
  GetChapters: mockGetChapters,
  GetContent: mockGetContent,
}));

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
  default: ({ tabs, onClose }: any) => (
    <div data-testid="tab-bar">
      {tabs.map((t: any) => (
        <span key={t.id}>
          {t.title}
          <button onClick={() => onClose(t.id)}>close-{t.id}</button>
        </span>
      ))}
    </div>
  ),
}));

vi.mock("./ContentEditor", () => ({
  default: ({ value, onChange, onMount }: any) => (
    <div data-testid="content-editor">
      {value}
      <button onClick={() => onChange?.("edited outline")}>edit content</button>
      <button
        onClick={() => {
          let onBlur = () => {};
          onMount(
            {
              onDidBlurEditorText: (callback: () => void) => {
                onBlur = callback;
              },
            },
            {},
          );
          onBlur();
        }}
      >
        blur editor
      </button>
    </div>
  ),
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
  DiffEditor: ({ original, modified, options }: any) => (
    <div
      data-testid="diff-editor"
      data-side-by-side={String(options?.renderSideBySide)}
    >
      <span>{original}</span>
      <span>{modified}</span>
    </div>
  ),
}));

// Mock useThemeStore
vi.mock("@/stores/useThemeStore", () => ({
  useThemeStore: () => ({ theme: "light" as const }),
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

vi.mock("./useEditorTabs", () => ({
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
    mockGetChapters.mockResolvedValue([]);
    mockGetContent.mockResolvedValue("file content");
    useEditorTabsStore.setState({ byNovel: {}, positions: {} });
  });

  it("renders empty state when no tabs", () => {
    render(<ContentPanel />);
    expect(
      screen.getByText("content.selectOrCreateChapter"),
    ).toBeInTheDocument();
  });

  it("renders tab select hint when tabs exist but no active tab", () => {
    mockTabsState = [
      { id: "f1", type: "file", path: "chapters/id_1.md", title: "Ch1" },
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
      ref.current?.openFile("chapters/id_1.md", "Chapter 1");
    });

    expect(mockFetchContent).toHaveBeenCalledWith(1, "chapters/id_1.md");
  });

  it("opens a volume outline as standalone Markdown and saves to its returned path", async () => {
    const path = "volumes/id_10.md";
    mockFetchContent.mockResolvedValue("");
    mockOpenTab.mockImplementation((tab: any) => {
      mockTabsState = [{ ...tab, id: "volume-tab" }];
      mockActiveTabIdState = "volume-tab";
    });
    mockUpdateTab.mockImplementation(
      (id: string, patch: Partial<EditorTab>) => {
        mockTabsState = mockTabsState.map((tab) =>
          tab.id === id ? { ...tab, ...patch } : tab,
        );
      },
    );

    const ref = { current: null as ContentPanelHandle | null };
    const view = render(<ContentPanel ref={ref} />);
    await act(async () => {
      ref.current?.openFile(path, "第一卷 · 卷纲");
    });
    expect(mockFetchContent).toHaveBeenCalledWith(1, path);
    expect(mockGetChapters).not.toHaveBeenCalled();
    expect(mockOpenTab).toHaveBeenCalledWith(
      expect.objectContaining({ path, viewMode: "content", content: "" }),
    );

    view.rerender(<ContentPanel ref={ref} />);
    expect(
      screen.getByRole("button", { name: "content.preview" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "content.outline" }),
    ).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "edit content" }));
    view.rerender(<ContentPanel ref={ref} />);
    await act(async () => {
      fireEvent.keyDown(window, { key: "s", ctrlKey: true });
    });
    expect(mockSaveContent).toHaveBeenCalledWith({
      novel_id: 1,
      path,
      content: "edited outline",
      expected_content: "",
    });

    fireEvent.click(screen.getByRole("button", { name: "content.preview" }));
    view.rerender(<ContentPanel ref={ref} />);
    expect(screen.getByTestId("markdown")).toHaveTextContent("edited outline");
  });

  it("keeps a queued chapter save when its tab closes", async () => {
    mockTabsState = [
      {
        id: "chapter-tab",
        type: "file",
        path: "chapters/id_1.md",
        title: "第一章",
        content: "draft",
        viewMode: "content",
      },
    ];
    mockActiveTabIdState = "chapter-tab";
    const view = render(<ContentPanel />);

    fireEvent.click(screen.getByRole("button", { name: "edit content" }));
    mockTabsState = [];
    mockActiveTabIdState = null;
    view.rerender(<ContentPanel />);

    await vi.waitFor(() =>
      expect(mockSaveContent).toHaveBeenCalledWith({
        novel_id: 1,
        path: "chapters/id_1.md",
        content: "edited outline",
        expected_content: "draft",
      }),
    );
  });

  it("切换工作区页面卸载编辑器后，待保存稿件仍会写入", async () => {
    mockTabsState = [
      {
        id: "chapter-tab",
        type: "file",
        path: "chapters/id_1.md",
        title: "第一章",
        content: "初稿",
        viewMode: "content",
      },
    ];
    mockActiveTabIdState = "chapter-tab";
    const view = render(<ContentPanel />);

    fireEvent.click(screen.getByRole("button", { name: "edit content" }));
    view.unmount();

    await vi.waitFor(() =>
      expect(mockSaveContent).toHaveBeenCalledWith({
        novel_id: 1,
        path: "chapters/id_1.md",
        content: "edited outline",
        expected_content: "初稿",
      }),
    );
  });

  it("编辑器失焦立即保存当前文件", async () => {
    mockTabsState = [
      {
        id: "chapter-tab",
        type: "file",
        path: "chapters/id_1.md",
        title: "第一章",
        content: "初稿",
        viewMode: "content",
      },
    ];
    mockActiveTabIdState = "chapter-tab";
    render(<ContentPanel />);

    fireEvent.click(screen.getByRole("button", { name: "edit content" }));
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "blur editor" }));
    });

    expect(mockSaveContent).toHaveBeenCalledWith({
      novel_id: 1,
      path: "chapters/id_1.md",
      content: "edited outline",
      expected_content: "初稿",
    });
  });

  it("关闭标签要等该文件保存成功", async () => {
    let resolveSave!: () => void;
    mockSaveContent.mockImplementationOnce(
      () =>
        new Promise<void>((resolve) => {
          resolveSave = resolve;
        }),
    );
    mockTabsState = [
      {
        id: "chapter-tab",
        type: "file",
        path: "chapters/id_1.md",
        title: "第一章",
        content: "初稿",
        viewMode: "content",
        isDirty: false,
      },
    ];
    mockActiveTabIdState = "chapter-tab";
    useEditorTabsStore.setState({
      byNovel: {
        "1": { tabs: mockTabsState, activeTabId: "chapter-tab" },
      },
    });
    mockUpdateTab.mockImplementation(
      (id: string, patch: Partial<EditorTab>) => {
        useEditorTabsStore.getState().updateTab(1, id, patch);
        mockTabsState = mockTabsState.map((tab) =>
          tab.id === id ? { ...tab, ...patch } : tab,
        );
      },
    );
    mockCloseTab.mockImplementation((id: string) => {
      useEditorTabsStore.getState().closeTab(1, id);
    });

    render(<ContentPanel />);
    fireEvent.click(screen.getByRole("button", { name: "edit content" }));
    fireEvent.click(screen.getByRole("button", { name: "close-chapter-tab" }));
    fireEvent.click(screen.getByRole("button", { name: "close-chapter-tab" }));

    expect(mockSaveContent).toHaveBeenCalledOnce();
    expect(mockCloseTab).not.toHaveBeenCalled();
    resolveSave();
    await vi.waitFor(() =>
      expect(mockCloseTab).toHaveBeenCalledWith("chapter-tab"),
    );
    expect(mockCloseTab).toHaveBeenCalledOnce();
  });

  it("保存失败时关闭标签仍保留未保存稿件", async () => {
    mockSaveContent.mockRejectedValueOnce(new Error("disk full"));
    mockTabsState = [
      {
        id: "chapter-tab",
        type: "file",
        path: "chapters/id_1.md",
        title: "第一章",
        content: "初稿",
        viewMode: "content",
        isDirty: false,
      },
    ];
    mockActiveTabIdState = "chapter-tab";
    useEditorTabsStore.setState({
      byNovel: {
        "1": { tabs: mockTabsState, activeTabId: "chapter-tab" },
      },
    });
    mockUpdateTab.mockImplementation(
      (id: string, patch: Partial<EditorTab>) => {
        useEditorTabsStore.getState().updateTab(1, id, patch);
        mockTabsState = mockTabsState.map((tab) =>
          tab.id === id ? { ...tab, ...patch } : tab,
        );
      },
    );

    render(<ContentPanel />);
    fireEvent.click(screen.getByRole("button", { name: "edit content" }));
    fireEvent.click(screen.getByRole("button", { name: "close-chapter-tab" }));

    await vi.waitFor(() => expect(toastError).toHaveBeenCalled());
    expect(mockCloseTab).not.toHaveBeenCalled();
    expect(useEditorTabsStore.getState().byNovel["1"].tabs[0].content).toBe(
      "edited outline",
    );
    expect(useEditorTabsStore.getState().byNovel["1"].tabs[0].isDirty).toBe(
      true,
    );
  });

  it("AI 修改脏文件时保留本地稿件并暂停待保存任务", async () => {
    mockTabsState = [
      {
        id: "chapter-tab",
        type: "file",
        path: "chapters/id_1.md",
        title: "第一章",
        content: "本地稿件",
        contentBase: "旧版",
        isDirty: true,
        viewMode: "content",
      },
    ];
    mockActiveTabIdState = "chapter-tab";
    useEditorTabsStore.setState({
      byNovel: { "1": { tabs: mockTabsState, activeTabId: "chapter-tab" } },
    });
    mockUpdateTab.mockImplementation(
      (id: string, patch: Partial<EditorTab>) => {
        useEditorTabsStore.getState().updateTab(1, id, patch);
        mockTabsState = mockTabsState.map((tab) =>
          tab.id === id ? { ...tab, ...patch } : tab,
        );
      },
    );
    const view = render(<ContentPanel />);
    fireEvent.click(screen.getByRole("button", { name: "edit content" }));
    const listener = vi
      .mocked(EventsOn)
      .mock.calls.find(([name]) => name === "file:changed")?.[1];
    expect(listener).toBeDefined();
    mockGetContent.mockResolvedValue("AI 新版");
    await act(async () => {
      await listener?.({ novel_id: 1, path: "chapters/id_1.md" });
    });
    view.rerender(<ContentPanel />);
    expect(useEditorTabsStore.getState().byNovel["1"].tabs[0]).toMatchObject({
      content: "edited outline",
      isDirty: true,
      contentConflict: true,
    });
    expect(screen.getByRole("alert")).toHaveTextContent(
      "content.conflictNotice",
    );
    expect(
      await screen.findByRole("dialog", { name: "content.conflictTitle" }),
    ).toBeInTheDocument();
    expect(await screen.findByTestId("diff-editor")).toHaveAttribute(
      "data-side-by-side",
      "true",
    );
    expect(screen.getByText("AI 新版")).toBeInTheDocument();
    await new Promise((resolve) => setTimeout(resolve, 600));
    expect(mockSaveContent).not.toHaveBeenCalled();

    await act(async () => {
      fireEvent.click(
        screen.getByRole("button", { name: "content.conflictUseDisk" }),
      );
    });
    expect(useEditorTabsStore.getState().byNovel["1"].tabs[0]).toMatchObject({
      content: "AI 新版",
      contentBase: "AI 新版",
      isDirty: false,
      contentConflict: false,
    });
  });

  it("条件写入冲突后可确认用本地稿件覆盖最新磁盘版", async () => {
    mockTabsState = [
      {
        id: "chapter-tab",
        type: "file",
        path: "chapters/id_1.md",
        title: "第一章",
        content: "旧版",
        contentBase: "旧版",
        viewMode: "content",
      },
    ];
    mockActiveTabIdState = "chapter-tab";
    useEditorTabsStore.setState({
      byNovel: { "1": { tabs: mockTabsState, activeTabId: "chapter-tab" } },
    });
    mockUpdateTab.mockImplementation(
      (id: string, patch: Partial<EditorTab>) => {
        useEditorTabsStore.getState().updateTab(1, id, patch);
        mockTabsState = mockTabsState.map((tab) =>
          tab.id === id ? { ...tab, ...patch } : tab,
        );
      },
    );
    mockSaveContent
      .mockRejectedValueOnce(new Error("CONTENT_CONFLICT: changed"))
      .mockResolvedValueOnce(undefined);
    mockGetContent
      .mockResolvedValueOnce("AI 新版")
      .mockResolvedValueOnce("AI 新版")
      .mockResolvedValue("edited outline");
    const view = render(<ContentPanel />);
    fireEvent.click(screen.getByRole("button", { name: "edit content" }));
    await act(async () => {
      fireEvent.keyDown(window, { key: "s", ctrlKey: true });
    });
    view.rerender(<ContentPanel />);
    expect(useEditorTabsStore.getState().byNovel["1"].tabs[0]).toMatchObject({
      content: "edited outline",
      isDirty: true,
      contentConflict: true,
    });
    expect(
      await screen.findByRole("dialog", { name: "content.conflictTitle" }),
    ).toBeInTheDocument();
    await screen.findByTestId("diff-editor");
    await act(async () => {
      fireEvent.click(
        screen.getByRole("button", { name: "content.conflictSaveLocal" }),
      );
    });
    expect(mockSaveContent).toHaveBeenLastCalledWith({
      novel_id: 1,
      path: "chapters/id_1.md",
      content: "edited outline",
      expected_content: "AI 新版",
    });
    expect(useEditorTabsStore.getState().byNovel["1"].tabs[0]).toMatchObject({
      content: "edited outline",
      contentBase: "edited outline",
      isDirty: false,
      contentConflict: false,
    });
  });

  it("查看 diff 后磁盘再次变化时刷新对比并要求重新选择", async () => {
    mockTabsState = [
      {
        id: "chapter-tab",
        type: "file",
        path: "chapters/id_1.md",
        title: "第一章",
        content: "旧版",
        contentBase: "旧版",
        viewMode: "content",
      },
    ];
    mockActiveTabIdState = "chapter-tab";
    useEditorTabsStore.setState({
      byNovel: { "1": { tabs: mockTabsState, activeTabId: "chapter-tab" } },
    });
    mockUpdateTab.mockImplementation(
      (id: string, patch: Partial<EditorTab>) => {
        useEditorTabsStore.getState().updateTab(1, id, patch);
        mockTabsState = mockTabsState.map((tab) =>
          tab.id === id ? { ...tab, ...patch } : tab,
        );
      },
    );
    mockSaveContent
      .mockRejectedValueOnce(new Error("CONTENT_CONFLICT: changed"))
      .mockResolvedValueOnce(undefined);
    mockGetContent
      .mockResolvedValueOnce("AI 版本一")
      .mockResolvedValue("AI 版本二");
    const view = render(<ContentPanel />);
    fireEvent.click(screen.getByRole("button", { name: "edit content" }));
    await act(async () => {
      fireEvent.keyDown(window, { key: "s", ctrlKey: true });
    });
    view.rerender(<ContentPanel />);
    expect(await screen.findByText("AI 版本一")).toBeInTheDocument();

    await act(async () => {
      fireEvent.click(
        screen.getByRole("button", { name: "content.conflictSaveLocal" }),
      );
    });
    expect(mockSaveContent).toHaveBeenCalledOnce();
    expect(screen.getByRole("status")).toHaveTextContent(
      "content.conflictChangedAgain",
    );
    expect(screen.getByText("AI 版本二")).toBeInTheDocument();
    expect(useEditorTabsStore.getState().byNovel["1"].tabs[0].isDirty).toBe(
      true,
    );

    await act(async () => {
      fireEvent.click(
        screen.getByRole("button", { name: "content.conflictSaveLocal" }),
      );
    });
    expect(mockSaveContent).toHaveBeenLastCalledWith({
      novel_id: 1,
      path: "chapters/id_1.md",
      content: "edited outline",
      expected_content: "AI 版本二",
    });
  });

  it("保存本地版刚成功又发生 AI 写入时继续保留双栏选择", async () => {
    mockTabsState = [
      {
        id: "chapter-tab",
        type: "file",
        path: "chapters/id_1.md",
        title: "第一章",
        content: "旧版",
        contentBase: "旧版",
        viewMode: "content",
      },
    ];
    mockActiveTabIdState = "chapter-tab";
    useEditorTabsStore.setState({
      byNovel: { "1": { tabs: mockTabsState, activeTabId: "chapter-tab" } },
    });
    mockUpdateTab.mockImplementation(
      (id: string, patch: Partial<EditorTab>) => {
        useEditorTabsStore.getState().updateTab(1, id, patch);
        mockTabsState = mockTabsState.map((tab) =>
          tab.id === id ? { ...tab, ...patch } : tab,
        );
      },
    );
    mockSaveContent
      .mockRejectedValueOnce(new Error("CONTENT_CONFLICT: changed"))
      .mockResolvedValueOnce(undefined);
    mockGetContent
      .mockResolvedValueOnce("AI 版本一")
      .mockResolvedValueOnce("AI 版本一")
      .mockResolvedValue("AI 版本二");
    const view = render(<ContentPanel />);
    fireEvent.click(screen.getByRole("button", { name: "edit content" }));
    await act(async () => {
      fireEvent.keyDown(window, { key: "s", ctrlKey: true });
    });
    view.rerender(<ContentPanel />);
    expect(await screen.findByText("AI 版本一")).toBeInTheDocument();

    await act(async () => {
      fireEvent.click(
        screen.getByRole("button", { name: "content.conflictSaveLocal" }),
      );
    });
    expect(mockSaveContent).toHaveBeenCalledTimes(2);
    expect(screen.getByRole("status")).toHaveTextContent(
      "content.conflictChangedAgain",
    );
    expect(screen.getByText("AI 版本二")).toBeInTheDocument();
    expect(useEditorTabsStore.getState().byNovel["1"].tabs[0]).toMatchObject({
      content: "edited outline",
      isDirty: true,
      contentConflict: true,
    });
  });

  it("AI 刷新读取期间开始输入时不回填旧请求结果", async () => {
    let resolveRead!: (value: string) => void;
    mockFetchContent.mockImplementationOnce(
      () =>
        new Promise<string>((resolve) => {
          resolveRead = resolve;
        }),
    );
    mockTabsState = [
      {
        id: "chapter-tab",
        type: "file",
        path: "chapters/id_1.md",
        title: "第一章",
        content: "旧版",
        contentBase: "旧版",
        isDirty: false,
        viewMode: "content",
      },
    ];
    mockActiveTabIdState = "chapter-tab";
    useEditorTabsStore.setState({
      byNovel: { "1": { tabs: mockTabsState, activeTabId: "chapter-tab" } },
    });
    mockUpdateTab.mockImplementation(
      (id: string, patch: Partial<EditorTab>) => {
        useEditorTabsStore.getState().updateTab(1, id, patch);
        mockTabsState = mockTabsState.map((tab) =>
          tab.id === id ? { ...tab, ...patch } : tab,
        );
      },
    );
    render(<ContentPanel />);
    const listener = vi
      .mocked(EventsOn)
      .mock.calls.find(([name]) => name === "file:changed")?.[1];
    expect(listener).toBeDefined();
    await act(async () => {
      const refresh = listener?.({ novel_id: 1, path: "chapters/id_1.md" });
      fireEvent.click(screen.getByRole("button", { name: "edit content" }));
      resolveRead("AI 新版");
      await refresh;
    });
    expect(useEditorTabsStore.getState().byNovel["1"].tabs[0]).toMatchObject({
      content: "edited outline",
      isDirty: true,
      contentConflict: true,
    });
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
      ref.current?.openFile("chapters/id_1.md", "Chapter 1");
    });

    // Should still open the tab with empty content
    expect(mockOpenTab).toHaveBeenCalledWith(
      expect.objectContaining({ content: "", path: "chapters/id_1.md" }),
    );
  });

  it("uses the backend outline path when loading and saving an ID-based chapter", async () => {
    mockGetChapters.mockResolvedValue([
      {
        id: 42,
        reading_number: 7,
        title: "重逢",
        file_path: "chapters/id_42.md",
        outline_file_path: "outlines/id_42.md",
      },
    ]);
    mockOpenTab.mockImplementation((tab: any) => {
      mockTabsState = [{ ...tab, id: "f42" }];
      mockActiveTabIdState = "f42";
    });
    mockUpdateTab.mockImplementation((id: string, patch: any) => {
      mockTabsState = mockTabsState.map((tab) =>
        tab.id === id ? { ...tab, ...patch } : tab,
      );
    });

    const ref = { current: null as ContentPanelHandle | null };
    const view = render(<ContentPanel ref={ref} />);
    await act(async () => {
      ref.current?.openFile("chapters/id_42.md", "第七章 重逢");
    });
    expect(mockOpenTab).toHaveBeenCalledWith(
      expect.objectContaining({
        path: "chapters/id_42.md",
        outlinePath: "outlines/id_42.md",
      }),
    );

    view.rerender(<ContentPanel ref={ref} />);
    await act(async () => {
      fireEvent.click(
        screen.getByRole("button", { name: "content.outlineEdit" }),
      );
    });
    expect(mockFetchContent).toHaveBeenCalledWith(1, "outlines/id_42.md");

    view.rerender(<ContentPanel ref={ref} />);
    fireEvent.click(screen.getByRole("button", { name: "edit content" }));
    view.rerender(<ContentPanel ref={ref} />);
    await act(async () => {
      fireEvent.keyDown(window, { key: "s", ctrlKey: true });
    });
    expect(mockSaveContent).toHaveBeenCalledWith({
      novel_id: 1,
      path: "outlines/id_42.md",
      content: "edited outline",
      expected_content: "file content",
    });
  });

  it("does not guess an outline path when chapter metadata is unavailable", async () => {
    mockTabsState = [
      {
        id: "f42",
        type: "file",
        path: "chapters/id_42.md",
        title: "重逢",
        content: "正文",
        viewMode: "content",
      },
    ];
    mockActiveTabIdState = "f42";
    render(<ContentPanel />);

    await act(async () => {
      fireEvent.click(
        screen.getByRole("button", { name: "content.outlineEdit" }),
      );
    });
    expect(mockUpdateTab).not.toHaveBeenCalledWith(
      "f42",
      expect.objectContaining({ viewMode: "outline-edit" }),
    );
    expect(mockFetchContent).not.toHaveBeenCalledWith(
      1,
      expect.stringContaining("outlines/"),
    );
  });

  it("returns an outline diff to its chapter using backend paths", async () => {
    mockGetChapters.mockResolvedValue([
      {
        id: 42,
        reading_number: 7,
        title: "重逢",
        file_path: "chapters/id_42.md",
        outline_file_path: "outlines/id_42.md",
      },
    ]);
    mockTabsState = [
      {
        id: "d42",
        type: "diff",
        path: "outlines/id_42.md",
        toolId: "tool-42",
        title: "大纲修改",
      },
    ];
    mockActiveTabIdState = "d42";
    const ref = { current: null as ContentPanelHandle | null };
    render(<ContentPanel ref={ref} />);

    await act(async () => {
      await ref.current?.handleDiffReject("tool-42");
    });
    await vi.waitFor(() =>
      expect(mockOpenTab).toHaveBeenCalledWith(
        expect.objectContaining({
          path: "chapters/id_42.md",
          outlinePath: "outlines/id_42.md",
          viewMode: "outline",
        }),
      ),
    );
  });

  it("renders content editor for file tab in content viewMode", () => {
    mockTabsState = [
      {
        id: "f1",
        type: "file",
        path: "chapters/id_1.md",
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
        path: "chapters/id_1.md",
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

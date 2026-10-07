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
import { reportAIFileChange } from "./aiFileChanges";
import { volumeKeys } from "@/lib/queryKeys";

const { mockGetChapters, mockGetVolumes, mockGetContent } = vi.hoisted(() => ({
  mockGetChapters: vi.fn(),
  mockGetVolumes: vi.fn(),
  mockGetContent: vi.fn(),
}));
vi.mock("@/lib/wailsjs/go/app/App", () => ({
  GetChapters: mockGetChapters,
  GetVolumes: mockGetVolumes,
  GetContent: mockGetContent,
}));

// 5.2 commit 1: useFileContent 引入 useQueryClient，render 需包 QueryClientProvider。
// 每个测试用独立 QueryClient（retry:false 避免重试），无状态残留。
function render(
  ui: ReactElement,
  qc = new QueryClient({ defaultOptions: { queries: { retry: false } } }),
) {
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
  default: ({ content, onSave, onDraftChange, onCancel }: any) => (
    <div data-testid="skill-edit-form">
      {content}
      <button onClick={() => onDraftChange(`${content}\n本地补充`)}>
        edit skill
      </button>
      <button onClick={() => void onSave(content).catch(() => undefined)}>
        save
      </button>
      <button onClick={onCancel}>cancel skill</button>
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
    mockGetVolumes.mockResolvedValue([]);
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
    useEditorTabsStore.setState({
      byNovel: { "1": { tabs: mockTabsState, activeTabId: "f1" } },
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

    // Click the save button in SkillEditForm mock — triggers doSave
    const saveBtn = screen.getByText("save");
    await act(async () => {
      fireEvent.click(saveBtn);
    });

    await vi.waitFor(() => {
      expect(toastError).toHaveBeenCalledWith("common.saveFailed: disk full");
    });
    expect(mockSaveContent).toHaveBeenCalledWith({
      novel_id: 1,
      path: "skills/test.md",
      content: "skill content",
      expected_content: "skill content",
    });
    expect(useEditorTabsStore.getState().byNovel["1"].tabs[0]).toMatchObject({
      content: "skill content",
      viewMode: "edit",
    });
  });

  it("切走页面期间 AI 修改技能时保留未保存表单稿件", async () => {
    mockGetContent.mockResolvedValue("AI 的技能内容");
    mockTabsState = [
      {
        id: "skill-tab",
        type: "file",
        path: "skills/test.md",
        title: "技能",
        content: "原技能内容",
        contentBase: "原技能内容",
        viewMode: "edit",
      },
    ];
    mockActiveTabIdState = "skill-tab";
    useEditorTabsStore.setState({
      byNovel: { "1": { tabs: mockTabsState, activeTabId: "skill-tab" } },
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
    fireEvent.click(screen.getByRole("button", { name: "edit skill" }));
    expect(useEditorTabsStore.getState().byNovel["1"].tabs[0]).toMatchObject({
      content: "原技能内容\n本地补充",
      isDirty: true,
    });
    view.unmount();
    act(() => reportAIFileChange({ novelId: 1, path: "skills/test.md" }));
    mockTabsState = useEditorTabsStore.getState().byNovel["1"].tabs;

    const reopened = render(<ContentPanel />);
    reopened.rerender(<ContentPanel />);
    expect(useEditorTabsStore.getState().byNovel["1"].tabs[0]).toMatchObject({
      content: "原技能内容\n本地补充",
      isDirty: true,
      contentConflict: true,
    });
    expect(
      await screen.findByRole("dialog", {
        name: "content.conflictTitle",
      }),
    ).toBeInTheDocument();
  });

  it("技能保存进行中切走页面，成功后仍更新原标签", async () => {
    let resolveSave!: () => void;
    mockSaveContent.mockImplementationOnce(
      () =>
        new Promise<void>((resolve) => {
          resolveSave = resolve;
        }),
    );
    mockTabsState = [
      {
        id: "skill-tab",
        type: "file",
        path: "skills/test.md",
        title: "技能",
        content: "原技能内容",
        contentBase: "原技能内容",
        viewMode: "edit",
      },
    ];
    mockActiveTabIdState = "skill-tab";
    useEditorTabsStore.setState({
      byNovel: { "1": { tabs: mockTabsState, activeTabId: "skill-tab" } },
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
    fireEvent.click(screen.getByRole("button", { name: "edit skill" }));
    view.rerender(<ContentPanel />);
    fireEvent.click(screen.getByRole("button", { name: "save" }));
    expect(mockSaveContent).toHaveBeenCalledWith({
      novel_id: 1,
      path: "skills/test.md",
      content: "原技能内容\n本地补充",
      expected_content: "原技能内容",
    });
    view.unmount();
    await act(async () => resolveSave());

    expect(useEditorTabsStore.getState().byNovel["1"].tabs[0]).toMatchObject({
      content: "原技能内容\n本地补充",
      contentBase: "原技能内容\n本地补充",
      isDirty: false,
      viewMode: "preview",
    });
  });

  it("技能旧稿保存进行中再次编辑，新稿不会被旧保存回调清掉", async () => {
    const qc = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    let resolveFirst!: () => void;
    mockSaveContent.mockImplementationOnce(
      () =>
        new Promise<void>((resolve) => {
          resolveFirst = resolve;
        }),
    );
    mockTabsState = [
      {
        id: "skill-tab",
        type: "file",
        path: "skills/test.md",
        title: "技能",
        content: "原技能内容",
        contentBase: "原技能内容",
        viewMode: "edit",
      },
    ];
    mockActiveTabIdState = "skill-tab";
    useEditorTabsStore.setState({
      byNovel: { "1": { tabs: mockTabsState, activeTabId: "skill-tab" } },
    });
    mockUpdateTab.mockImplementation(
      (id: string, patch: Partial<EditorTab>) => {
        useEditorTabsStore.getState().updateTab(1, id, patch);
        mockTabsState = mockTabsState.map((tab) =>
          tab.id === id ? { ...tab, ...patch } : tab,
        );
      },
    );

    const view = render(<ContentPanel />, qc);
    fireEvent.click(screen.getByRole("button", { name: "edit skill" }));
    view.rerender(<ContentPanel />);
    fireEvent.click(screen.getByRole("button", { name: "save" }));
    view.unmount();

    const reopened = render(<ContentPanel />, qc);
    fireEvent.click(screen.getByRole("button", { name: "cancel skill" }));
    expect(toastError).toHaveBeenCalledWith("skill.saving");
    expect(useEditorTabsStore.getState().byNovel["1"].tabs[0].isDirty).toBe(
      true,
    );
    fireEvent.click(screen.getByRole("button", { name: "edit skill" }));
    await act(async () => resolveFirst());
    expect(useEditorTabsStore.getState().byNovel["1"].tabs[0]).toMatchObject({
      content: "原技能内容\n本地补充\n本地补充",
      contentBase: "原技能内容\n本地补充",
      isDirty: true,
      viewMode: "edit",
    });
    reopened.rerender(<ContentPanel />);
    fireEvent.click(screen.getByRole("button", { name: "save" }));
    await vi.waitFor(() => expect(mockSaveContent).toHaveBeenCalledTimes(2));
    expect(mockSaveContent.mock.calls[1][0]).toMatchObject({
      content: "原技能内容\n本地补充\n本地补充",
      expected_content: "原技能内容\n本地补充",
    });
    reopened.unmount();
  });

  it("未保存的技能表单拒绝关闭标签并提示先保存", async () => {
    mockTabsState = [
      {
        id: "skill-tab",
        type: "file",
        path: "skills/test.md",
        title: "技能",
        content: "未保存技能",
        contentBase: "原技能",
        isDirty: true,
        viewMode: "edit",
      },
    ];
    mockActiveTabIdState = "skill-tab";
    useEditorTabsStore.setState({
      byNovel: { "1": { tabs: mockTabsState, activeTabId: "skill-tab" } },
    });
    render(<ContentPanel />);

    fireEvent.click(screen.getByRole("button", { name: "close-skill-tab" }));
    await vi.waitFor(() =>
      expect(toastError).toHaveBeenCalledWith("content.saveSkillBeforeClose"),
    );
    expect(mockCloseTab).not.toHaveBeenCalled();
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

  it("打开文件期间收到 AI 事件时采用事件后的磁盘内容", async () => {
    let resolveRead!: (value: string) => void;
    mockFetchContent.mockImplementationOnce(
      () =>
        new Promise<string>((resolve) => {
          resolveRead = resolve;
        }),
    );
    mockGetContent.mockResolvedValue("AI 新版");
    const ref = { current: null as ContentPanelHandle | null };
    render(<ContentPanel ref={ref} />);

    await act(async () => ref.current?.openFile("goink.md", "故事状态"));
    act(() => reportAIFileChange({ novelId: 1, path: "goink.md" }));
    await act(async () => resolveRead("旧版"));

    expect(mockGetContent).toHaveBeenCalledWith(1, "goink.md");
    expect(mockOpenTab).toHaveBeenCalledWith(
      expect.objectContaining({ path: "goink.md", content: "AI 新版" }),
    );
  });

  it("连续 AI 事件使较早的读取结果不能回填干净标签", async () => {
    let resolveOldRead!: (value: string) => void;
    mockGetContent
      .mockImplementationOnce(
        () =>
          new Promise<string>((resolve) => {
            resolveOldRead = resolve;
          }),
      )
      .mockResolvedValueOnce("第二次 AI 修改");
    mockTabsState = [
      {
        id: "volume-tab",
        type: "file",
        path: "volumes/id_2.md",
        title: "卷纲",
        content: "原文",
        contentBase: "原文",
        viewMode: "content",
      },
    ];
    mockActiveTabIdState = "volume-tab";
    useEditorTabsStore.setState({
      byNovel: { "1": { tabs: mockTabsState, activeTabId: "volume-tab" } },
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

    act(() => reportAIFileChange({ novelId: 1, path: "volumes/id_2.md" }));
    act(() => reportAIFileChange({ novelId: 1, path: "volumes/id_2.md" }));
    await act(async () => resolveOldRead("第一次 AI 修改"));

    await vi.waitFor(() =>
      expect(useEditorTabsStore.getState().byNovel["1"].tabs[0]).toMatchObject({
        content: "第二次 AI 修改",
        contentBase: "第二次 AI 修改",
        contentNeedsRefresh: false,
      }),
    );
  });

  it("编辑器隐藏期间收到 AI 事件，重新显示时刷新干净标签", async () => {
    mockGetContent.mockResolvedValue("隐藏期间的新稿");
    mockTabsState = [
      {
        id: "chapter-tab",
        type: "file",
        path: "chapters/id_1.md",
        title: "第一章",
        content: "旧稿",
        contentBase: "旧稿",
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
    view.unmount();
    act(() => reportAIFileChange({ novelId: 1, path: "chapters/id_1.md" }));
    mockTabsState = useEditorTabsStore.getState().byNovel["1"].tabs;

    render(<ContentPanel />);
    await vi.waitFor(() =>
      expect(useEditorTabsStore.getState().byNovel["1"].tabs[0]).toMatchObject({
        content: "隐藏期间的新稿",
        contentNeedsRefresh: false,
      }),
    );
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

  it.each(["approve", "reject"])(
    "%s 卷纲审批后，从元数据取得卷名而不使用路径标题",
    async (action) => {
      const path = "volumes/id_10.md";
      mockGetVolumes.mockResolvedValue([
        { outline_file_path: path, name: "第一卷" },
      ]);
      mockTabsState = [
        {
          id: "volume-diff",
          type: "diff",
          path,
          toolId: "tool-volume",
          title: `diff: ${path}`,
        },
      ];
      const ref = { current: null as ContentPanelHandle | null };
      render(<ContentPanel ref={ref} />);
      await act(async () => {
        if (action === "approve")
          await ref.current?.handleDiffApprove("tool-volume", path);
        else await ref.current?.handleDiffReject("tool-volume");
      });
      await vi.waitFor(() =>
        expect(mockOpenTab).toHaveBeenCalledWith(
          expect.objectContaining({
            path,
            title: "sidebar.volumeOutlineTitle",
            viewMode: "content",
          }),
        ),
      );
      expect(mockGetVolumes).toHaveBeenCalledWith(1);
      expect(mockGetChapters).not.toHaveBeenCalled();
      expect(mockCloseTab).toHaveBeenCalledWith("volume-diff");
    },
  );

  it.each([false, true])(
    "卷纲审批先打开再补齐缺失缓存，关闭状态=%s",
    async (closeBeforeLoaded) => {
      let resolveMetadata!: (value: unknown[]) => void;
      mockGetVolumes.mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveMetadata = resolve;
          }),
      );
      const qc = new QueryClient({
        defaultOptions: { queries: { retry: false, staleTime: 30_000 } },
      });
      qc.setQueryData(volumeKeys.list(1), []);
      mockOpenDiffTab.mockImplementationOnce((data) =>
        useEditorTabsStore.getState().openDiffTab(1, data),
      );
      const ref = { current: null as ContentPanelHandle | null };
      render(<ContentPanel ref={ref} />, qc);
      act(() =>
        ref.current?.openDiffTab({
          path: "volumes/id_10.md",
          title: "路径标题",
          toolId: "volume-tool",
          diff: "",
          original: "旧卷纲",
          modified: "新卷纲",
          changeType: "full_replace",
          reason: "",
        }),
      );
      const store = useEditorTabsStore.getState();
      const id = store.byNovel["1"].activeTabId!;
      expect(store.byNovel["1"].tabs[0]).toMatchObject({
        id,
        original: "旧卷纲",
        modified: "新卷纲",
      });
      await vi.waitFor(() => expect(resolveMetadata).toBeTypeOf("function"));
      if (closeBeforeLoaded) act(() => store.closeTab(1, id));
      await act(async () =>
        resolveMetadata([
          { outline_file_path: "volumes/id_10.md", name: "第一卷" },
        ]),
      );
      if (closeBeforeLoaded) {
        expect(useEditorTabsStore.getState().byNovel["1"].tabs).toEqual([]);
        expect(mockUpdateTab).not.toHaveBeenCalledWith(
          id,
          expect.objectContaining({ title: expect.any(String) }),
        );
      } else {
        expect(mockUpdateTab).toHaveBeenCalledWith(id, {
          title: "diff: sidebar.volumeOutlineTitle",
        });
      }
      expect(mockOpenDiffTab).toHaveBeenCalledOnce();
    },
  );

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
    mockGetContent.mockResolvedValue("AI 新版");
    await act(async () => {
      reportAIFileChange({ novelId: 1, path: "chapters/id_1.md" });
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
    mockGetContent.mockImplementationOnce(
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
    await act(async () => {
      reportAIFileChange({ novelId: 1, path: "chapters/id_1.md" });
      fireEvent.click(screen.getByRole("button", { name: "edit content" }));
      resolveRead("AI 新版");
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
    await vi.waitFor(() =>
      expect(mockOpenTab).toHaveBeenCalledWith(
        expect.objectContaining({ content: "", path: "chapters/id_1.md" }),
      ),
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
      useEditorTabsStore.getState().openTab(1, { ...tab, id: "f42" });
    });
    mockUpdateTab.mockImplementation((id: string, patch: any) => {
      useEditorTabsStore.getState().updateTab(1, id, patch);
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

  it("opens a newly created outline by its physical path", async () => {
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
        id: "d-new",
        type: "diff",
        path: "outlines/new.md",
        toolId: "tool-new",
        title: "新建大纲",
      },
    ];
    mockActiveTabIdState = "d-new";
    const ref = { current: null as ContentPanelHandle | null };
    render(<ContentPanel ref={ref} />);

    await act(async () => {
      await ref.current?.handleDiffApprove("tool-new", "outlines/id_42.md");
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
    expect(mockCloseTab).toHaveBeenCalledWith("d-new");
    expect(mockFetchContent).not.toHaveBeenCalledWith(1, "outlines/new.md");
  });

  it("opens a newly created chapter by its physical path", async () => {
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
        id: "d-new",
        type: "diff",
        path: "chapters/3/new.md",
        toolId: "tool-new",
        title: "新建章节",
      },
    ];
    mockActiveTabIdState = "d-new";
    const ref = { current: null as ContentPanelHandle | null };
    render(<ContentPanel ref={ref} />);

    await act(async () => {
      await ref.current?.handleDiffApprove("tool-new", "chapters/id_42.md");
    });
    await vi.waitFor(() =>
      expect(mockOpenTab).toHaveBeenCalledWith(
        expect.objectContaining({
          path: "chapters/id_42.md",
          viewMode: "content",
        }),
      ),
    );
    expect(mockFetchContent).not.toHaveBeenCalledWith(1, "chapters/3/new.md");
  });

  it("uses the latest diff tab when automatic approval finishes before rerender", async () => {
    mockGetChapters.mockResolvedValue([
      {
        id: 42,
        reading_number: 7,
        title: "重逢",
        file_path: "chapters/id_42.md",
        outline_file_path: "outlines/id_42.md",
      },
    ]);
    const ref = { current: null as ContentPanelHandle | null };
    render(<ContentPanel ref={ref} />);
    useEditorTabsStore.getState().openDiffTab(1, {
      path: "outlines/new.md",
      toolId: "tool-new",
      title: "新建大纲",
      diff: "",
      original: "",
      modified: "# 大纲",
      changeType: "full_replace",
      reason: "",
    });

    await act(async () => {
      await ref.current?.handleDiffApprove("tool-new", "outlines/id_42.md");
    });
    expect(mockCloseTab).toHaveBeenCalled();
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

  it.each([
    "outlines/new.md",
    "outlines/3/new.md",
    "chapters/new.md",
    "chapters/3/new.md",
  ])("closes a rejected new-file diff without opening %s", async (path) => {
    mockTabsState = [
      {
        id: "d-new",
        type: "diff",
        path,
        toolId: "tool-new",
        title: "新建文件",
      },
    ];
    mockActiveTabIdState = "d-new";
    const ref = { current: null as ContentPanelHandle | null };
    render(<ContentPanel ref={ref} />);

    await act(async () => {
      await ref.current?.handleDiffReject("tool-new");
    });
    expect(mockCloseTab).toHaveBeenCalledWith("d-new");
    expect(mockOpenTab).not.toHaveBeenCalled();
    expect(mockFetchContent).not.toHaveBeenCalledWith(1, path);
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

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, act } from "@testing-library/react";
import ContentPanel, { type ContentPanelHandle } from "./ContentPanel";
import { toastError } from "@/utils/toast";

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

// Mock useApp
const mockGetContent = vi.fn();
const mockSaveContent = vi.fn();

vi.mock("@/hooks/useApp", () => ({
  useApp: () => ({
    GetContent: mockGetContent,
    SaveContent: mockSaveContent,
  }),
}));

describe("ContentPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockTabsState = [];
    mockActiveTabIdState = null;
    mockInitRefValue = true;
    mockGetContent.mockResolvedValue("file content");
    mockSaveContent.mockResolvedValue(undefined);
  });

  it("renders empty state when no tabs", () => {
    render(<ContentPanel novelId={1} />);
    expect(
      screen.getByText("content.selectOrCreateChapter"),
    ).toBeInTheDocument();
  });

  it("renders tab select hint when tabs exist but no active tab", () => {
    mockTabsState = [
      { id: "f1", type: "file", path: "chapters/001.md", title: "Ch1" },
    ];
    mockActiveTabIdState = null;
    render(<ContentPanel novelId={1} />);
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

    render(<ContentPanel novelId={1} />);

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
    mockGetContent.mockResolvedValue("# Chapter 1");
    mockOpenTab.mockImplementation((tab: any) => {
      mockTabsState = [{ ...tab, id: "f1" }];
      mockActiveTabIdState = "f1";
    });

    const ref = { current: null as ContentPanelHandle | null };
    render(<ContentPanel ref={ref} novelId={1} />);

    await act(async () => {
      ref.current?.openFile("chapters/001.md", "Chapter 1");
    });

    expect(mockGetContent).toHaveBeenCalledWith(1, "chapters/001.md");
  });

  it("opens file with empty content on GetContent failure", async () => {
    mockGetContent.mockRejectedValue(new Error("not found"));
    mockOpenTab.mockImplementation((tab: any) => {
      mockTabsState = [{ ...tab, id: "f1" }];
      mockActiveTabIdState = "f1";
    });

    const ref = { current: null as ContentPanelHandle | null };
    render(<ContentPanel ref={ref} novelId={1} />);

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

    render(<ContentPanel novelId={1} />);
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

    render(<ContentPanel novelId={1} />);
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

    render(<ContentPanel novelId={1} />);
    expect(screen.getByTestId("diff-editor")).toBeInTheDocument();
  });

  it("calls closeAllTabs via ref", async () => {
    const ref = { current: null as ContentPanelHandle | null };
    render(<ContentPanel ref={ref} novelId={1} />);

    await act(async () => {
      ref.current?.closeAllTabs();
    });

    expect(mockCloseAllTabs).toHaveBeenCalled();
  });
});

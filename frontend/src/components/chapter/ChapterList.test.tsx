import { describe, it, expect, vi, beforeEach } from "vitest";
import { render as originalRender, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactElement } from "react";
import ChapterList from "./ChapterList";
import { toastError } from "@/utils/toast";

// 5.2 commit 1: useChapters 引入 useQuery，render 需包 QueryClientProvider。
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

// 5.2 commit 1: useChapters mock（query 部分，直接 import wailsjs 不经 useApp）。
const { mockUseChapters } = vi.hoisted(() => ({
  mockUseChapters: vi.fn(),
}));
vi.mock("./useChapters", () => ({
  useChapters: mockUseChapters,
}));

// Mock useApp（CreateChapter/UpdateChapterTitle 仍走 useApp，commit 2 迁 mutation）
const mockCreateChapter = vi.fn();
const mockUpdateChapterTitle = vi.fn();

vi.mock("@/hooks/useApp", () => ({
  useApp: () => ({
    CreateChapter: mockCreateChapter,
    UpdateChapterTitle: mockUpdateChapterTitle,
  }),
}));

// Mock EventsOn
vi.mock("@/lib/wailsjs/runtime/runtime", () => ({
  EventsOn: vi.fn(() => vi.fn()),
}));

describe("ChapterList", () => {
  const defaultProps = {
    novelId: 1,
    target: null as { path: string; title: string } | null,
    onSelectChapter: vi.fn(),
    onSelectGoink: vi.fn(),
    onExportNovel: vi.fn(),
  };

  beforeEach(() => {
    vi.clearAllMocks();
    // 5.2 commit 1: 默认返回空数组（query data 兜底）
    mockUseChapters.mockReturnValue({
      data: [],
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });
  });

  it("renders empty state when no chapters", async () => {
    render(<ChapterList {...defaultProps} />);
    expect(await screen.findByText("sidebar.noChapters")).toBeInTheDocument();
  });

  it("shows loadFailed when query errors", async () => {
    // 5.2 commit 1: query 错误走中间件 toast，组件 isError 内连显示固定文案（对齐 ReaderList）
    mockUseChapters.mockReturnValue({
      data: [],
      isLoading: false,
      isError: true,
      refetch: vi.fn(),
    });
    render(<ChapterList {...defaultProps} />);
    expect(await screen.findByText("chapter.loadFailed")).toBeInTheDocument();
  });

  it("shows toastError when rename fails", async () => {
    const user = userEvent.setup();
    const chapters = [
      {
        id: 1,
        chapter_number: 1,
        title: "Chapter One",
        file_path: "chapters/001.md",
        word_count: 0,
      },
    ];
    mockUseChapters.mockReturnValue({
      data: chapters,
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    });
    mockUpdateChapterTitle.mockRejectedValue(new Error("rename failed"));

    render(<ChapterList {...defaultProps} />);

    // Wait for chapter block to appear, then expand it
    const blockBtn = await screen.findByText("sidebar.chapterN");
    await user.click(blockBtn);

    // Now the chapter should be visible, find the edit (pencil) button
    const pencilBtn = document
      .querySelector("button svg.lucide-pencil")
      ?.closest("button");
    expect(pencilBtn).toBeTruthy();
    await user.click(pencilBtn!);

    // Edit the title input
    const titleInput = screen.getByDisplayValue("Chapter One");
    await user.clear(titleInput);
    await user.type(titleInput, "New Title");

    // Trigger commit by pressing Enter
    await user.type(titleInput, "{Enter}");

    // Verify toastError was called
    await vi.waitFor(() => {
      expect(toastError).toHaveBeenCalledWith(
        "common.saveFailed: rename failed",
      );
    });
  });

  it("creates a chapter successfully", async () => {
    const user = userEvent.setup();
    // 5.2 commit 1: data 走 useChapters mock（beforeEach 已默认空数组）
    mockCreateChapter.mockResolvedValue(undefined);

    render(<ChapterList {...defaultProps} />);

    // Click the + button to show create form
    const addBtns = screen.getAllByRole("button");
    // Find the button with Plus icon
    const plusBtn = addBtns.find((btn) => btn.querySelector("svg.lucide-plus"));
    if (plusBtn) {
      await user.click(plusBtn);
    }

    // Type chapter title
    const titleInput = await screen.findByPlaceholderText(
      "sidebar.chapterTitle",
    );
    await user.type(titleInput, "My Chapter");

    // Click add button
    const addBtn = screen.getByText("sidebar.add");
    await user.click(addBtn);

    await vi.waitFor(() => {
      expect(mockCreateChapter).toHaveBeenCalledWith({
        novel_id: 1,
        title: "My Chapter",
      });
    });
  });
});

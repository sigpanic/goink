import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import ChapterList from "./ChapterList";
import { toastError } from "@/lib/utils";

// Mock toastError
vi.mock("@/lib/utils", async (importOriginal) => {
  const mod = await importOriginal<typeof import("@/lib/utils")>();
  return {
    ...mod,
    toastError: vi.fn(),
  };
});

// Mock useApp
const mockGetChapters = vi.fn();
const mockCreateChapter = vi.fn();
const mockUpdateChapterTitle = vi.fn();

vi.mock("@/hooks/useApp", () => ({
  useApp: () => ({
    GetChapters: mockGetChapters,
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
    mockGetChapters.mockResolvedValue([]);
  });

  it("renders empty state when no chapters", async () => {
    render(<ChapterList {...defaultProps} />);
    expect(await screen.findByText("sidebar.noChapters")).toBeInTheDocument();
  });

  it("shows load error when GetChapters fails", async () => {
    mockGetChapters.mockRejectedValue(new Error("db error"));
    render(<ChapterList {...defaultProps} />);
    expect(await screen.findByText("db error")).toBeInTheDocument();
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
    mockGetChapters.mockResolvedValue(chapters);
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
    mockGetChapters.mockResolvedValue([]);
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

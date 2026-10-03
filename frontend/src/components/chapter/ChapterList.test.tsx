import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  render as originalRender,
  screen,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactElement } from "react";
import ChapterList from "./ChapterList";
import { useEditorStore } from "@/stores/useEditorStore";
import { useEditorTabsStore } from "@/components/content/useEditorTabsStore";
import { toastError } from "@/utils/toast";

const {
  mockUseChapters,
  mockUseVolumes,
  mockPlaceChapter,
  mockUpdateChapterTitle,
} = vi.hoisted(() => ({
  mockUseChapters: vi.fn(),
  mockUseVolumes: vi.fn(),
  mockPlaceChapter: vi.fn(),
  mockUpdateChapterTitle: vi.fn(),
}));

vi.mock("@/utils/toast", async (importOriginal) => {
  const mod = await importOriginal<typeof import("@/utils/toast")>();
  return { ...mod, toastError: vi.fn() };
});
vi.mock("./useChapters", () => ({ useChapters: mockUseChapters }));
vi.mock("@/components/volume/useVolumes", () => ({
  useVolumes: mockUseVolumes,
}));
vi.mock("@/components/chapter-management/useChapterStructureMutations", () => ({
  useChapterStructureMutations: () => ({
    place: { isPending: false, mutateAsync: mockPlaceChapter },
  }),
}));
vi.mock("./useUpdateChapterTitle", () => ({
  useUpdateChapterTitle: () => ({ mutateAsync: mockUpdateChapterTitle }),
}));
vi.mock("@/lib/wailsjs/runtime/runtime", () => ({
  EventsOn: vi.fn(() => vi.fn()),
}));

function render(ui: ReactElement) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return originalRender(ui, {
    wrapper: ({ children }) => (
      <QueryClientProvider client={qc}>{children}</QueryClientProvider>
    ),
  });
}

function makeChapter(id: number, number: number, volumeId: number | null) {
  return {
    id,
    novel_id: 1,
    volume_id: volumeId,
    reading_number: number,
    title: `Chapter ${number}`,
    file_path: `chapters/id_${id}.md`,
    outline_file_path: `outlines/id_${id}.md`,
    word_count: 0,
  };
}

const firstVolume = {
  id: 10,
  name: "第一卷",
  sort_order: 1,
  outline_file_path: "volumes/id_10.md",
};
const secondVolume = {
  id: 20,
  name: "第二卷",
  sort_order: 2,
  outline_file_path: "volumes/id_20.md",
};
const defaultProps = {
  novelId: 1,
  onSelectChapter: vi.fn(),
  onSelectGoink: vi.fn(),
  onSelectVolumeOutline: vi.fn(),
  onExportNovel: vi.fn(),
};

describe("ChapterList", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useEditorStore.getState().setTabTarget(null);
    useEditorTabsStore.setState({ byNovel: {}, positions: {} });
    mockUseChapters.mockReturnValue({
      data: [],
      isPending: false,
      isError: false,
      refetch: vi.fn(),
    });
    mockUseVolumes.mockReturnValue({
      data: [],
      isPending: false,
      isError: false,
      refetch: vi.fn(),
    });
  });

  it("keeps an unassigned group available when the novel has no volumes", () => {
    render(<ChapterList {...defaultProps} />);
    expect(
      screen.getByText("chapterManagement.unassigned"),
    ).toBeInTheDocument();
    expect(screen.getByText("sidebar.noChapters")).toBeInTheDocument();
    expect(screen.getByText("sidebar.storyStatus")).toBeInTheDocument();
  });

  it("shows a retry action when the chapter query fails", async () => {
    const retryChapters = vi.fn();
    const retryVolumes = vi.fn();
    mockUseChapters.mockReturnValue({
      data: [],
      isPending: false,
      isError: true,
      refetch: retryChapters,
    });
    mockUseVolumes.mockReturnValue({
      data: [],
      isPending: false,
      isError: false,
      refetch: retryVolumes,
    });
    render(<ChapterList {...defaultProps} />);
    expect(screen.getByText("chapter.loadFailed")).toBeInTheDocument();
    await userEvent
      .setup()
      .click(screen.getByRole("button", { name: "common.retry" }));
    expect(retryChapters).toHaveBeenCalledOnce();
    expect(retryVolumes).toHaveBeenCalledOnce();
  });

  it("orders groups by volume and keeps unassigned chapters last", async () => {
    mockUseVolumes.mockReturnValue({
      data: [secondVolume, firstVolume],
      isPending: false,
      isError: false,
      refetch: vi.fn(),
    });
    mockUseChapters.mockReturnValue({
      data: [
        makeChapter(3, 3, null),
        makeChapter(2, 2, 20),
        makeChapter(1, 1, 10),
      ],
      isPending: false,
      isError: false,
      refetch: vi.fn(),
    });
    render(<ChapterList {...defaultProps} />);

    const groups = screen.getAllByRole("region");
    expect(groups.map((group) => group.getAttribute("aria-label"))).toEqual([
      "第一卷",
      "第二卷",
      "chapterManagement.unassigned",
    ]);
    await userEvent
      .setup()
      .click(within(groups[0]).getByRole("button", { name: /第一卷/ }));
    expect(within(groups[0]).getByText("Chapter 1")).toBeInTheDocument();
    expect(within(groups[0]).queryByText("Chapter 2")).not.toBeInTheDocument();
  });

  it("opens a volume outline from its group and reveals the active outline group", async () => {
    mockUseVolumes.mockReturnValue({
      data: [firstVolume, secondVolume],
      isPending: false,
      isError: false,
      refetch: vi.fn(),
    });
    useEditorTabsStore.getState().openTab(1, {
      type: "file",
      path: secondVolume.outline_file_path,
      title: "第二卷 · 卷纲",
    });
    const user = userEvent.setup();
    render(<ChapterList {...defaultProps} />);

    const secondGroup = screen.getAllByRole("region")[1];
    const outlineButton = within(secondGroup).getByRole("button", {
      name: "sidebar.openVolumeOutline",
    });
    expect(outlineButton).toHaveClass("bg-primary/10");
    await user.click(outlineButton);
    expect(defaultProps.onSelectVolumeOutline).toHaveBeenCalledWith(
      "volumes/id_20.md",
      "第二卷",
    );
  });

  it("selects one range within a long volume and reveals the active chapter", async () => {
    const chapters = Array.from({ length: 205 }, (_, index) =>
      makeChapter(index + 1, index + 1, 10),
    );
    mockUseVolumes.mockReturnValue({
      data: [firstVolume],
      isPending: false,
      isError: false,
      refetch: vi.fn(),
    });
    mockUseChapters.mockReturnValue({
      data: chapters,
      isPending: false,
      isError: false,
      refetch: vi.fn(),
    });
    useEditorStore.getState().setTabTarget({
      path: "chapters/id_5.md",
      title: "Chapter 5",
    });
    const user = userEvent.setup();
    render(<ChapterList {...defaultProps} />);

    expect(screen.getByText("Chapter 5")).toBeInTheDocument();
    expect(screen.queryByText("Chapter 205")).not.toBeInTheDocument();
    await user.click(
      screen.getByRole("button", { name: "sidebar.chapterSection" }),
    );
    await user.click(screen.getAllByRole("option")[2]);
    expect(screen.getByText("Chapter 205")).toBeInTheDocument();
    expect(screen.queryByText("Chapter 5")).not.toBeInTheDocument();
  });

  it("creates in the chosen group and opens the new chapter", async () => {
    const created = makeChapter(42, 2, 10);
    mockPlaceChapter.mockResolvedValue(created);
    mockUseVolumes.mockReturnValue({
      data: [firstVolume, secondVolume],
      isPending: false,
      isError: false,
      refetch: vi.fn(),
    });
    const user = userEvent.setup();
    render(<ChapterList {...defaultProps} />);

    const firstGroup = screen.getAllByRole("region")[0];
    await user.click(
      within(firstGroup).getByRole("button", {
        name: "chapterManagement.addToGroup",
      }),
    );
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", {
        name: "chapterManagement.chapterPosition",
      }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "chapterManagement.targetVolume" }),
    ).toHaveTextContent("第一卷");
    await user.type(screen.getByRole("textbox"), "New chapter");
    await user.click(screen.getByRole("button", { name: "sidebar.add" }));

    expect(mockPlaceChapter).toHaveBeenCalledWith({
      novel_id: 1,
      title: "New chapter",
      target_volume_id: 10,
    });
    expect(defaultProps.onSelectChapter).toHaveBeenCalledWith(created);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("closes the dropdown before the dialog and restores focus on Escape", async () => {
    mockUseVolumes.mockReturnValue({
      data: [firstVolume],
      isPending: false,
      isError: false,
      refetch: vi.fn(),
    });
    const user = userEvent.setup();
    render(<ChapterList {...defaultProps} />);
    const createButton = screen.getByRole("button", {
      name: "chapterManagement.createChapter",
    });
    await user.click(createButton);
    expect(screen.getByRole("textbox")).toHaveFocus();
    await user.click(
      screen.getByRole("button", { name: "chapterManagement.targetVolume" }),
    );
    expect(screen.getByRole("listbox")).toBeInTheDocument();
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(createButton).toHaveFocus();
  });

  it("defaults the top create action to the active chapter's volume", async () => {
    mockUseVolumes.mockReturnValue({
      data: [firstVolume, secondVolume],
      isPending: false,
      isError: false,
      refetch: vi.fn(),
    });
    mockUseChapters.mockReturnValue({
      data: [makeChapter(1, 1, 10)],
      isPending: false,
      isError: false,
      refetch: vi.fn(),
    });
    useEditorStore.getState().setTabTarget({
      path: "chapters/id_1.md",
      title: "Chapter 1",
    });
    render(<ChapterList {...defaultProps} />);
    await userEvent
      .setup()
      .click(
        screen.getByRole("button", { name: "chapterManagement.createChapter" }),
      );
    expect(
      screen.getByRole("button", { name: "chapterManagement.targetVolume" }),
    ).toHaveTextContent("第一卷");
  });

  it("defaults the top create action to the last volume without an active chapter", async () => {
    mockUseVolumes.mockReturnValue({
      data: [firstVolume, secondVolume],
      isPending: false,
      isError: false,
      refetch: vi.fn(),
    });
    render(<ChapterList {...defaultProps} />);
    await userEvent
      .setup()
      .click(
        screen.getByRole("button", { name: "chapterManagement.createChapter" }),
      );
    expect(
      screen.getByRole("button", { name: "chapterManagement.targetVolume" }),
    ).toHaveTextContent("第二卷");
  });

  it("creates in the unassigned group when the novel has no volumes", async () => {
    const created = makeChapter(7, 1, null);
    mockPlaceChapter.mockResolvedValue(created);
    const user = userEvent.setup();
    render(<ChapterList {...defaultProps} />);

    await user.click(
      screen.getByRole("button", { name: "chapterManagement.createChapter" }),
    );
    expect(
      screen.getByRole("button", { name: "chapterManagement.targetVolume" }),
    ).toHaveTextContent("chapterManagement.unassigned");
    await user.type(screen.getByRole("textbox"), "First chapter");
    await user.click(screen.getByRole("button", { name: "sidebar.add" }));
    expect(mockPlaceChapter).toHaveBeenCalledWith({
      novel_id: 1,
      title: "First chapter",
    });
    expect(defaultProps.onSelectChapter).toHaveBeenCalledWith(created);
  });

  it("keeps the rename control and reports a failed rename", async () => {
    mockUseChapters.mockReturnValue({
      data: [makeChapter(42, 1, null)],
      isPending: false,
      isError: false,
      refetch: vi.fn(),
    });
    mockUpdateChapterTitle.mockRejectedValue(new Error("rename failed"));
    const user = userEvent.setup();
    render(<ChapterList {...defaultProps} />);

    await user.click(
      screen.getByRole("button", { name: "sidebar.renameChapter" }),
    );
    const titleInput = screen.getByRole("textbox", {
      name: "sidebar.chapterTitle",
    });
    await user.clear(titleInput);
    await user.type(titleInput, "New Title{Enter}");

    await vi.waitFor(() => {
      expect(mockUpdateChapterTitle).toHaveBeenCalledWith({
        chapterID: 42,
        title: "New Title",
      });
      expect(toastError).toHaveBeenCalledWith(
        "common.saveFailed: rename failed",
      );
    });
  });

  it("cancels an inline rename without saving it", async () => {
    mockUseChapters.mockReturnValue({
      data: [makeChapter(42, 1, null)],
      isPending: false,
      isError: false,
      refetch: vi.fn(),
    });
    const user = userEvent.setup();
    render(<ChapterList {...defaultProps} />);

    await user.click(
      screen.getByRole("button", { name: "sidebar.renameChapter" }),
    );
    const titleInput = screen.getByRole("textbox", {
      name: "sidebar.chapterTitle",
    });
    await user.clear(titleInput);
    await user.type(titleInput, "Discarded{Escape}");
    expect(screen.getByText("Chapter 1")).toBeInTheDocument();
    expect(mockUpdateChapterTitle).not.toHaveBeenCalled();
  });
});

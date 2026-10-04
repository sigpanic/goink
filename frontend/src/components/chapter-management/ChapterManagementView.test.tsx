import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { contentKeys } from "@/lib/queryKeys";
import { useEditorTabsStore } from "@/components/content/useEditorTabsStore";
import ChapterManagementView from "./ChapterManagementView";

const {
  mockGetChapters,
  mockGetVolumes,
  mockPlaceVolume,
  mockUpdateVolume,
  mockDeleteVolume,
  mockPlaceChapter,
  mockDeleteChapter,
} = vi.hoisted(() => ({
  mockGetChapters: vi.fn(),
  mockGetVolumes: vi.fn(),
  mockPlaceVolume: vi.fn(),
  mockUpdateVolume: vi.fn(),
  mockDeleteVolume: vi.fn(),
  mockPlaceChapter: vi.fn(),
  mockDeleteChapter: vi.fn(),
}));

vi.mock("@/lib/wailsjs/go/app/App", () => ({
  GetChapters: mockGetChapters,
  GetVolumes: mockGetVolumes,
  PlaceVolume: mockPlaceVolume,
  UpdateVolume: mockUpdateVolume,
  DeleteVolume: mockDeleteVolume,
  PlaceChapter: mockPlaceChapter,
  DeleteChapter: mockDeleteChapter,
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (
      key: string,
      options?: {
        n?: number;
        count?: number;
        start?: number;
        end?: number;
        name?: string;
        title?: string;
        defaultValue?: string;
      },
    ) => {
      if (key === "sidebar.chapterN") return `Ch.${options?.n}`;
      if (key === "sidebar.wordCount") return `${options?.count} words`;
      if (key === "sidebar.chapterCountShort")
        return `${options?.count} chapters`;
      if (key === "sidebar.chapterRange")
        return `Ch.${options?.start}-Ch.${options?.end}`;
      if (options?.name) return `${key} ${options.name}`;
      if (options?.title) return `${key} ${options.title}`;
      if (options?.defaultValue) return options.defaultValue;
      return key;
    },
  }),
}));

function renderView(novelId = 1) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      <ChapterManagementView novelId={novelId} />
    </QueryClientProvider>,
  );
  return qc;
}

function fireDragAt(
  target: Element,
  type: "dragover" | "drop",
  dataTransfer: { dropEffect: string },
  clientY: number,
  clientX = 0,
) {
  const event = new MouseEvent(type, {
    bubbles: true,
    cancelable: true,
    clientY,
    clientX,
  });
  Object.defineProperty(event, "dataTransfer", { value: dataTransfer });
  fireEvent(target, event);
}

beforeEach(() => {
  vi.clearAllMocks();
  mockGetChapters.mockResolvedValue([]);
  mockGetVolumes.mockResolvedValue([]);
  mockPlaceVolume.mockResolvedValue({});
  mockUpdateVolume.mockResolvedValue(undefined);
  mockDeleteVolume.mockResolvedValue(undefined);
  mockPlaceChapter.mockResolvedValue({});
  mockDeleteChapter.mockResolvedValue({ deleted: true, references: [] });
  useEditorTabsStore.getState().closeAllTabs(1);
});

describe("ChapterManagementView", () => {
  it("按卷顺序展示后端阅读编号，未分卷始终在最后", async () => {
    mockGetVolumes.mockResolvedValue([
      { id: 20, name: "第二卷", sort_order: 2 },
      { id: 10, name: "第一卷", sort_order: 1 },
      { id: 30, name: "空卷", sort_order: 3 },
    ]);
    mockGetChapters.mockResolvedValue([
      {
        id: 3,
        volume_id: null,
        reading_number: 3,
        title: "尾声",
        word_count: 120,
      },
      {
        id: 2,
        volume_id: 20,
        reading_number: 2,
        title: "转折",
        word_count: 220,
      },
      {
        id: 1,
        volume_id: 10,
        reading_number: 1,
        title: "开篇",
        word_count: 320,
      },
    ]);

    renderView();
    await screen.findByText("开篇");

    const headings = screen.getAllByRole("heading", { level: 2 });
    expect(headings.map((heading) => heading.textContent)).toEqual([
      "chapterManagement.volumes",
      "第一卷1 chapters",
      "第二卷1 chapters",
      "空卷0 chapters",
      "chapterManagement.unassigned1 chapters",
    ]);
    const firstVolume = screen
      .getByRole("heading", { name: /第一卷/ })
      .closest("section");
    expect(firstVolume).not.toBeNull();
    expect(within(firstVolume!).getByText("开篇")).toBeInTheDocument();
    expect(within(firstVolume!).getByText("Ch.1")).toBeInTheDocument();
    expect(within(firstVolume!).getByText("320 words")).toBeInTheDocument();
    const emptyVolume = screen
      .getByRole("heading", { name: /空卷/ })
      .closest("section");
    expect(emptyVolume).not.toBeNull();
    expect(
      within(emptyVolume!).getByText("chapterManagement.emptyGroup"),
    ).toBeInTheDocument();
    expect(screen.getByText("尾声")).toBeInTheDocument();
  });

  it("无卷无章节时仍显示未分卷组与空状态", async () => {
    renderView();
    expect(
      await screen.findByText("chapterManagement.noVolumes"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("chapterManagement.noChapters"),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: /chapterManagement.unassigned/ }),
    ).toBeInTheDocument();
  });

  it("长卷默认收起，展开后按百章分段，卷轨道可定位展开", async () => {
    mockGetVolumes.mockResolvedValue([
      { id: 1, name: "长卷", sort_order: 1 },
      { id: 2, name: "短卷", sort_order: 2 },
    ]);
    mockGetChapters.mockResolvedValue([
      ...Array.from({ length: 205 }, (_, index) => ({
        id: index + 1,
        volume_id: 1,
        reading_number: index + 1,
        title: `长章${index + 1}`,
        word_count: 100,
      })),
      {
        id: 206,
        volume_id: 2,
        reading_number: 206,
        title: "短章",
        word_count: 100,
      },
    ]);

    renderView();
    await screen.findByText("短章");
    const longVolume = screen.getByRole("heading", { name: /长卷/ });
    expect(
      within(longVolume).getByRole("button", { expanded: false }),
    ).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByText("长章1")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "长卷" }));
    expect(
      within(longVolume).getByRole("button", { expanded: true }),
    ).toHaveAttribute("aria-expanded", "true");
    expect(
      screen.getByRole("button", { name: "Ch.1-Ch.100" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Ch.101-Ch.200" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Ch.201-Ch.205" }),
    ).toBeInTheDocument();
    expect(screen.queryByText("长章101")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Ch.101-Ch.200" }));
    expect(screen.getByText("长章101")).toBeInTheDocument();
    expect(screen.queryByText("长章1")).not.toBeInTheDocument();
  });

  it("读取失败可重试", async () => {
    mockGetVolumes.mockRejectedValueOnce(new Error("temporary failure"));
    renderView();
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "chapterManagement.loadFailed",
    );
    mockGetVolumes.mockResolvedValue([]);
    screen.getByRole("button", { name: "chapterManagement.retry" }).click();
    expect(
      await screen.findByText("chapterManagement.noVolumes"),
    ).toBeInTheDocument();
    expect(mockGetVolumes).toHaveBeenCalledTimes(2);
  });

  it("可在指定卷前新建卷，成功后刷新卷和章节", async () => {
    mockGetVolumes.mockResolvedValue([
      { id: 10, name: "第一卷", sort_order: 1 },
    ]);
    renderView();
    await screen.findByRole("button", { name: "第一卷" });
    fireEvent.click(
      screen.getByRole("button", { name: "chapterManagement.createVolume" }),
    );
    fireEvent.change(screen.getByLabelText("chapterManagement.volumeName"), {
      target: { value: "序章" },
    });
    fireEvent.click(
      screen.getByRole("button", { name: "chapterManagement.volumePosition" }),
    );
    fireEvent.click(
      screen.getByRole("option", {
        name: "chapterManagement.beforeVolume 第一卷",
      }),
    );
    expect(mockPlaceVolume).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "common.save" }));
    await waitFor(() =>
      expect(mockPlaceVolume).toHaveBeenCalledWith({
        novel_id: 1,
        name: "序章",
        before_volume_id: 10,
      }),
    );
    await waitFor(() => expect(mockGetChapters).toHaveBeenCalledTimes(2));
    expect(mockGetVolumes).toHaveBeenCalledTimes(2);
  });

  it("支持重命名，且有章节的卷不能从前端删除", async () => {
    const outlinePath = "volumes/id_10.md";
    mockGetVolumes.mockResolvedValue([
      { id: 10, name: "第一卷", sort_order: 1, outline_file_path: outlinePath },
    ]);
    mockGetChapters.mockResolvedValue([
      {
        id: 1,
        volume_id: 10,
        reading_number: 1,
        title: "开篇",
        word_count: 10,
      },
    ]);
    useEditorTabsStore.getState().openTab(1, {
      type: "file",
      path: outlinePath,
      title: "sidebar.volumeOutlineTitle 第一卷",
    });
    renderView();
    await screen.findByText("开篇");
    fireEvent.click(
      screen.getByRole("button", {
        name: "chapterManagement.renameVolume 第一卷",
      }),
    );
    fireEvent.change(screen.getByLabelText("chapterManagement.volumeName"), {
      target: { value: "新版第一卷" },
    });
    fireEvent.click(screen.getByRole("button", { name: "common.save" }));
    await waitFor(() =>
      expect(mockUpdateVolume).toHaveBeenCalledWith(1, 10, "新版第一卷"),
    );
    await waitFor(() =>
      expect(useEditorTabsStore.getState().byNovel["1"]?.tabs[0]?.title).toBe(
        "sidebar.volumeOutlineTitle 新版第一卷",
      ),
    );
    fireEvent.click(
      screen.getByRole("button", {
        name: "chapterManagement.deleteVolume 第一卷",
      }),
    );
    expect(mockDeleteVolume).not.toHaveBeenCalled();
    expect(
      screen.queryByText("chapterManagement.deleteVolumeConfirm 第一卷"),
    ).not.toBeInTheDocument();
  });

  it("空卷确认后删除；拖拽仅传源卷和锚点卷 ID", async () => {
    mockGetVolumes.mockResolvedValue([
      { id: 10, name: "第一卷", sort_order: 1 },
      { id: 20, name: "第二卷", sort_order: 2 },
      { id: 30, name: "第三卷", sort_order: 3 },
    ]);
    renderView();
    await screen.findByRole("button", { name: "第三卷" });
    const transfer = {
      effectAllowed: "none",
      dropEffect: "none",
      setData: vi.fn(),
    };
    const source = screen.getByRole("button", {
      name: "chapterManagement.dragVolume 第三卷",
    });
    fireEvent.dragStart(source, { dataTransfer: transfer });
    fireEvent.dragOver(screen.getByRole("button", { name: "第一卷" }), {
      dataTransfer: transfer,
    });
    fireEvent.drop(screen.getByRole("button", { name: "第一卷" }), {
      dataTransfer: transfer,
    });
    await waitFor(() =>
      expect(mockPlaceVolume).toHaveBeenCalledWith({
        novel_id: 1,
        source_volume_id: 30,
        before_volume_id: 10,
      }),
    );
    fireEvent.dragStart(
      screen.getByRole("button", {
        name: "chapterManagement.dragVolume 第一卷",
      }),
      { dataTransfer: transfer },
    );
    fireEvent.dragOver(screen.getByText("chapterManagement.moveToEnd"), {
      dataTransfer: transfer,
    });
    fireEvent.drop(screen.getByText("chapterManagement.moveToEnd"), {
      dataTransfer: transfer,
    });
    await waitFor(() =>
      expect(mockPlaceVolume).toHaveBeenCalledWith({
        novel_id: 1,
        source_volume_id: 10,
      }),
    );
    fireEvent.click(
      screen.getByRole("button", {
        name: "chapterManagement.deleteVolume 第二卷",
      }),
    );
    expect(
      screen.getByText("chapterManagement.deleteVolumeConfirm 第二卷"),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "common.confirm" }));
    await waitFor(() => expect(mockDeleteVolume).toHaveBeenCalledWith(1, 20));
  });

  it("deleting a volume closes its outline tab and clears cached content", async () => {
    const outlinePath = "volumes/id_10.md";
    mockGetVolumes.mockResolvedValue([
      { id: 10, name: "第一卷", sort_order: 1, outline_file_path: outlinePath },
    ]);
    useEditorTabsStore.getState().openTab(1, {
      type: "file",
      path: outlinePath,
      title: "第一卷 · 卷纲",
      isDirty: true,
    });
    const qc = renderView();
    qc.setQueryData(contentKeys.detail(1, outlinePath), "draft");
    await screen.findByRole("button", { name: "第一卷" });

    fireEvent.click(
      screen.getByRole("button", {
        name: "chapterManagement.deleteVolume 第一卷",
      }),
    );
    expect(
      screen.getByText("chapterManagement.deleteVolumeConfirmUnsaved 第一卷"),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "common.confirm" }));
    await waitFor(() => {
      expect(mockDeleteVolume).toHaveBeenCalledWith(1, 10);
      expect(useEditorTabsStore.getState().byNovel["1"]?.tabs).toEqual([]);
      expect(
        qc.getQueryData(contentKeys.detail(1, outlinePath)),
      ).toBeUndefined();
    });
  });

  it("卷标签右半区可把第一卷移到第二位，并显示落点和成功反馈", async () => {
    const initial = [
      { id: 10, name: "第一卷", sort_order: 1 },
      { id: 20, name: "第二卷", sort_order: 2 },
      { id: 30, name: "第三卷", sort_order: 3 },
    ];
    mockGetVolumes
      .mockResolvedValueOnce(initial)
      .mockResolvedValue([
        { ...initial[1], sort_order: 1 },
        { ...initial[0], sort_order: 2 },
        initial[2],
      ]);
    renderView();
    await screen.findByRole("button", { name: "第三卷" });
    const targetChip = screen
      .getByRole("button", { name: "第二卷" })
      .closest("[data-volume-id]")!;
    vi.spyOn(targetChip, "getBoundingClientRect").mockReturnValue({
      top: 0,
      bottom: 40,
      left: 0,
      right: 100,
      width: 100,
      height: 40,
      x: 0,
      y: 0,
      toJSON: () => ({}),
    });
    const transfer = {
      effectAllowed: "none",
      dropEffect: "none",
      setData: vi.fn(),
    };
    fireEvent.dragStart(
      screen.getByRole("button", {
        name: "chapterManagement.dragVolume 第一卷",
      }),
      { dataTransfer: transfer },
    );
    fireDragAt(targetChip, "dragover", transfer, 20, 75);
    expect(targetChip).toHaveClass("volume-chip-drop-after");
    fireDragAt(targetChip, "drop", transfer, 20, 75);
    await waitFor(() =>
      expect(mockPlaceVolume).toHaveBeenCalledWith({
        novel_id: 1,
        source_volume_id: 10,
        before_volume_id: 30,
      }),
    );
    await waitFor(() => {
      const movedChip = screen
        .getByRole("button", { name: "第一卷" })
        .closest<HTMLElement>("[data-volume-id]")!;
      expect(movedChip).toHaveClass("volume-chip-moved");
      expect(within(movedChip).getByRole("status")).toHaveTextContent(
        "chapterManagement.volumeMoveSuccess 第一卷",
      );
    });
  });

  it("卷标签之间的空隙也能接住拖拽", async () => {
    mockGetVolumes.mockResolvedValue([
      { id: 10, name: "第一卷", sort_order: 1 },
      { id: 20, name: "第二卷", sort_order: 2 },
      { id: 30, name: "第三卷", sort_order: 3 },
    ]);
    renderView();
    await screen.findByRole("button", { name: "第三卷" });
    const chips = [10, 20, 30].map((id, index) => {
      const chip = document.querySelector<HTMLElement>(
        `[data-volume-id="${id}"]`,
      )!;
      const left = index * 120;
      vi.spyOn(chip, "getBoundingClientRect").mockReturnValue({
        top: 0,
        bottom: 40,
        left,
        right: left + 100,
        width: 100,
        height: 40,
        x: left,
        y: 0,
        toJSON: () => ({}),
      });
      return chip;
    });
    const rail = chips[0].parentElement!;
    const transfer = {
      effectAllowed: "none",
      dropEffect: "none",
      setData: vi.fn(),
    };
    fireEvent.dragStart(
      screen.getByRole("button", {
        name: "chapterManagement.dragVolume 第三卷",
      }),
      { dataTransfer: transfer },
    );
    fireDragAt(rail, "dragover", transfer, 20, 110);
    expect(chips[1]).toHaveClass("volume-chip-drop-before");
    fireDragAt(rail, "drop", transfer, 20, 110);
    await waitFor(() =>
      expect(mockPlaceVolume).toHaveBeenCalledWith({
        novel_id: 1,
        source_volume_id: 30,
        before_volume_id: 20,
      }),
    );
  });

  it("可在章节后插入，并在空卷末尾新建章节", async () => {
    mockGetVolumes.mockResolvedValue([
      { id: 10, name: "第一卷", sort_order: 1 },
      { id: 20, name: "空卷", sort_order: 2 },
    ]);
    mockGetChapters.mockResolvedValue([
      {
        id: 1,
        volume_id: 10,
        reading_number: 1,
        title: "开篇",
        word_count: 10,
      },
      {
        id: 2,
        volume_id: 10,
        reading_number: 2,
        title: "转折",
        word_count: 10,
      },
    ]);
    renderView();
    await screen.findByText("开篇");
    fireEvent.click(
      screen.getByRole("button", {
        name: "chapterManagement.chapterActions 开篇",
      }),
    );
    fireEvent.click(
      screen.getByRole("button", {
        name: "chapterManagement.insertAfterChapter",
      }),
    );
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "chapterManagement.chapterPosition" }),
    ).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("chapterManagement.chapterTitle"), {
      target: { value: "插入章" },
    });
    fireEvent.click(screen.getByRole("button", { name: "common.save" }));
    await waitFor(() =>
      expect(mockPlaceChapter).toHaveBeenCalledWith({
        novel_id: 1,
        title: "插入章",
        target_volume_id: 10,
        before_chapter_id: 2,
      }),
    );
    await waitFor(() =>
      expect(
        screen.queryByLabelText("chapterManagement.chapterTitle"),
      ).not.toBeInTheDocument(),
    );

    fireEvent.click(
      screen.getByRole("button", { name: "chapterManagement.addToGroup 空卷" }),
    );
    fireEvent.change(screen.getByLabelText("chapterManagement.chapterTitle"), {
      target: { value: "卷首" },
    });
    fireEvent.click(screen.getByRole("button", { name: "common.save" }));
    await waitFor(() =>
      expect(mockPlaceChapter).toHaveBeenCalledWith({
        novel_id: 1,
        title: "卷首",
        target_volume_id: 20,
      }),
    );
  });

  it("在百章分段边界后插入时使用下一段首章作为锚点", async () => {
    mockGetVolumes.mockResolvedValue([{ id: 10, name: "长卷", sort_order: 1 }]);
    mockGetChapters.mockResolvedValue(
      Array.from({ length: 101 }, (_, index) => ({
        id: index + 1,
        volume_id: 10,
        reading_number: index + 1,
        title: `章节${index + 1}`,
        word_count: 10,
      })),
    );
    renderView();
    await screen.findByRole("button", { name: "长卷" });
    fireEvent.click(screen.getByRole("button", { name: "长卷" }));
    fireEvent.click(screen.getByRole("button", { name: "Ch.1-Ch.100" }));
    fireEvent.click(
      screen.getByRole("button", {
        name: "chapterManagement.chapterActions 章节100",
      }),
    );
    fireEvent.click(
      screen.getByRole("button", {
        name: "chapterManagement.insertAfterChapter",
      }),
    );
    fireEvent.change(screen.getByLabelText("chapterManagement.chapterTitle"), {
      target: { value: "边界插入" },
    });
    fireEvent.click(screen.getByRole("button", { name: "common.save" }));
    await waitFor(() =>
      expect(mockPlaceChapter).toHaveBeenCalledWith({
        novel_id: 1,
        title: "边界插入",
        target_volume_id: 10,
        before_chapter_id: 101,
      }),
    );
  });

  it("可选择锚点移动，并拖拽到卷标签、章节前和未分卷组", async () => {
    mockGetVolumes.mockResolvedValue([
      { id: 10, name: "第一卷", sort_order: 1 },
      { id: 20, name: "第二卷", sort_order: 2 },
    ]);
    mockGetChapters.mockResolvedValue([
      {
        id: 1,
        volume_id: 10,
        reading_number: 1,
        title: "开篇",
        word_count: 10,
      },
      {
        id: 2,
        volume_id: 20,
        reading_number: 2,
        title: "第二章",
        word_count: 10,
      },
      {
        id: 3,
        volume_id: null,
        reading_number: 3,
        title: "尾声",
        word_count: 10,
      },
    ]);
    renderView();
    await screen.findByText("尾声");
    fireEvent.click(
      screen.getByRole("button", {
        name: "chapterManagement.chapterActions 开篇",
      }),
    );
    fireEvent.click(
      screen.getByRole("button", { name: "chapterManagement.moveChapter" }),
    );
    fireEvent.click(
      screen.getByRole("button", { name: "chapterManagement.targetVolume" }),
    );
    fireEvent.click(
      screen.getByRole("option", { name: "chapterManagement.unassigned" }),
    );
    fireEvent.click(
      screen.getByRole("button", { name: "chapterManagement.chapterPosition" }),
    );
    fireEvent.click(
      screen.getByRole("option", {
        name: "chapterManagement.beforeChapter 尾声",
      }),
    );
    fireEvent.click(screen.getByRole("button", { name: "common.save" }));
    await waitFor(() =>
      expect(mockPlaceChapter).toHaveBeenCalledWith({
        novel_id: 1,
        source_chapter_id: 1,
        before_chapter_id: 3,
      }),
    );
    await waitFor(() =>
      expect(
        screen.queryByLabelText("chapterManagement.targetVolume"),
      ).not.toBeInTheDocument(),
    );

    const transfer = {
      effectAllowed: "none",
      dropEffect: "none",
      setData: vi.fn(),
    };
    fireEvent.dragStart(
      screen.getByRole("button", {
        name: "chapterManagement.dragChapter 尾声",
      }),
      { dataTransfer: transfer },
    );
    fireEvent.dragOver(screen.getByRole("button", { name: "第二卷" }), {
      dataTransfer: transfer,
    });
    fireEvent.drop(screen.getByRole("button", { name: "第二卷" }), {
      dataTransfer: transfer,
    });
    await waitFor(() =>
      expect(mockPlaceChapter).toHaveBeenCalledWith({
        novel_id: 1,
        source_chapter_id: 3,
        target_volume_id: 20,
      }),
    );

    fireEvent.dragStart(
      screen.getByRole("button", {
        name: "chapterManagement.dragChapter 开篇",
      }),
      { dataTransfer: transfer },
    );
    const targetRow = screen.getByText("第二章").closest("li");
    expect(targetRow).not.toBeNull();
    fireEvent.dragOver(targetRow!, { dataTransfer: transfer });
    fireEvent.drop(targetRow!, { dataTransfer: transfer });
    await waitFor(() =>
      expect(mockPlaceChapter).toHaveBeenCalledWith({
        novel_id: 1,
        source_chapter_id: 1,
        target_volume_id: 20,
        before_chapter_id: 2,
      }),
    );

    fireEvent.dragStart(
      screen.getByRole("button", {
        name: "chapterManagement.dragChapter 第二章",
      }),
      { dataTransfer: transfer },
    );
    fireEvent.dragOver(
      screen.getByRole("button", { name: "chapterManagement.unassigned" }),
      { dataTransfer: transfer },
    );
    fireEvent.drop(
      screen.getByRole("button", { name: "chapterManagement.unassigned" }),
      { dataTransfer: transfer },
    );
    await waitFor(() =>
      expect(mockPlaceChapter).toHaveBeenCalledWith({
        novel_id: 1,
        source_chapter_id: 2,
      }),
    );
  });

  it("落在章节下半区时插到该章后，并高亮移动成功的章节", async () => {
    const initial = [
      {
        id: 1,
        volume_id: 10,
        reading_number: 1,
        title: "第一章",
        word_count: 10,
      },
      {
        id: 2,
        volume_id: 10,
        reading_number: 2,
        title: "第二章",
        word_count: 10,
      },
      {
        id: 3,
        volume_id: 10,
        reading_number: 3,
        title: "第三章",
        word_count: 10,
      },
    ];
    mockGetVolumes.mockResolvedValue([
      { id: 10, name: "第一卷", sort_order: 1 },
    ]);
    mockGetChapters
      .mockResolvedValueOnce(initial)
      .mockResolvedValue([
        { ...initial[1], reading_number: 1 },
        { ...initial[0], reading_number: 2 },
        initial[2],
      ]);
    renderView();
    await screen.findByText("第三章");
    const targetRow = screen.getByText("第二章").closest("li")!;
    vi.spyOn(targetRow, "getBoundingClientRect").mockReturnValue({
      top: 0,
      bottom: 100,
      left: 0,
      right: 300,
      width: 300,
      height: 100,
      x: 0,
      y: 0,
      toJSON: () => ({}),
    });
    const transfer = {
      effectAllowed: "none",
      dropEffect: "none",
      setData: vi.fn(),
    };
    fireEvent.dragStart(
      screen.getByRole("button", {
        name: "chapterManagement.dragChapter 第一章",
      }),
      { dataTransfer: transfer },
    );
    fireDragAt(targetRow, "dragover", transfer, 75);
    expect(targetRow).toHaveClass("chapter-row-drop-after");
    fireDragAt(targetRow, "drop", transfer, 75);
    await waitFor(() =>
      expect(mockPlaceChapter).toHaveBeenCalledWith({
        novel_id: 1,
        source_chapter_id: 1,
        target_volume_id: 10,
        before_chapter_id: 3,
      }),
    );
    await waitFor(() => {
      const movedRow = screen.getByText("第一章").closest("li")!;
      expect(movedRow).toHaveClass("chapter-row-moved");
      expect(within(movedRow).getByRole("status")).toHaveTextContent(
        "chapterManagement.moveSuccess",
      );
      expect(
        screen
          .getAllByText(/章$/)
          .filter((node) => node.closest("li"))
          .map((node) => node.textContent),
      ).toEqual(["第二章", "第一章", "第三章"]);
    });
  });

  it("落在章节上半区时插到该章前", async () => {
    mockGetVolumes.mockResolvedValue([
      { id: 10, name: "第一卷", sort_order: 1 },
    ]);
    mockGetChapters.mockResolvedValue([
      {
        id: 1,
        volume_id: 10,
        reading_number: 1,
        title: "第一章",
        word_count: 10,
      },
      {
        id: 2,
        volume_id: 10,
        reading_number: 2,
        title: "第二章",
        word_count: 10,
      },
      {
        id: 3,
        volume_id: 10,
        reading_number: 3,
        title: "第三章",
        word_count: 10,
      },
    ]);
    renderView();
    await screen.findByText("第三章");
    const targetRow = screen.getByText("第二章").closest("li")!;
    vi.spyOn(targetRow, "getBoundingClientRect").mockReturnValue({
      top: 0,
      bottom: 100,
      left: 0,
      right: 300,
      width: 300,
      height: 100,
      x: 0,
      y: 0,
      toJSON: () => ({}),
    });
    const transfer = {
      effectAllowed: "none",
      dropEffect: "none",
      setData: vi.fn(),
    };
    fireEvent.dragStart(
      screen.getByRole("button", {
        name: "chapterManagement.dragChapter 第三章",
      }),
      { dataTransfer: transfer },
    );
    fireDragAt(targetRow, "dragover", transfer, 25);
    expect(targetRow).toHaveClass("chapter-row-drop-before");
    fireDragAt(targetRow, "drop", transfer, 25);
    await waitFor(() =>
      expect(mockPlaceChapter).toHaveBeenCalledWith({
        novel_id: 1,
        source_chapter_id: 3,
        target_volume_id: 10,
        before_chapter_id: 2,
      }),
    );
  });

  it("删除被引用的章节展示来源清单，保留编辑标签", async () => {
    mockGetChapters.mockResolvedValue([
      {
        id: 1,
        volume_id: null,
        reading_number: 1,
        title: "开篇",
        file_path: "chapters/id_1.md",
        outline_file_path: "outlines/id_1.md",
        word_count: 10,
      },
    ]);
    mockDeleteChapter.mockResolvedValue({
      deleted: false,
      references: [{ kind: "story_arc", id: 9, label: "主线节点" }],
    });
    const tabId = useEditorTabsStore
      .getState()
      .openTab(1, { type: "file", path: "chapters/id_1.md", title: "开篇" });
    renderView();
    await screen.findByText("开篇");
    fireEvent.click(
      screen.getByRole("button", {
        name: "chapterManagement.chapterActions 开篇",
      }),
    );
    fireEvent.click(screen.getByRole("button", { name: "common.delete" }));
    fireEvent.click(screen.getByRole("button", { name: "common.confirm" }));
    await waitFor(() =>
      expect(screen.getByRole("alert")).toHaveTextContent("主线节点"),
    );
    expect(screen.getByRole("alert")).toHaveTextContent("ID 9");
    expect(
      useEditorTabsStore
        .getState()
        .byNovel["1"].tabs.some((tab) => tab.id === tabId),
    ).toBe(true);
    expect(mockGetChapters).toHaveBeenCalledTimes(1);
  });

  it("删除成功后刷新章节并关闭正文、大纲标签及内容缓存", async () => {
    mockGetChapters
      .mockResolvedValueOnce([
        {
          id: 1,
          volume_id: null,
          reading_number: 1,
          title: "开篇",
          file_path: "chapters/id_1.md",
          outline_file_path: "outlines/id_1.md",
          word_count: 10,
        },
      ])
      .mockResolvedValue([]);
    const store = useEditorTabsStore.getState();
    store.openTab(1, {
      type: "file",
      path: "chapters/id_1.md",
      outlinePath: "outlines/id_1.md",
      title: "开篇",
      isDirty: true,
    });
    store.openTab(1, { type: "file", path: "outlines/id_1.md", title: "大纲" });
    const qc = renderView();
    qc.setQueryData(contentKeys.detail(1, "chapters/id_1.md"), "正文");
    qc.setQueryData(contentKeys.detail(1, "outlines/id_1.md"), "大纲");
    await screen.findByText("开篇");
    fireEvent.click(
      screen.getByRole("button", {
        name: "chapterManagement.chapterActions 开篇",
      }),
    );
    fireEvent.click(screen.getByRole("button", { name: "common.delete" }));
    expect(
      screen.getByText("chapterManagement.deleteChapterConfirmUnsaved 开篇"),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "common.confirm" }));
    await waitFor(() => expect(mockDeleteChapter).toHaveBeenCalledWith(1, 1));
    await waitFor(() =>
      expect(screen.queryByText("开篇")).not.toBeInTheDocument(),
    );
    expect(useEditorTabsStore.getState().byNovel["1"].tabs).toHaveLength(0);
    expect(
      qc.getQueryData(contentKeys.detail(1, "chapters/id_1.md")),
    ).toBeUndefined();
    expect(
      qc.getQueryData(contentKeys.detail(1, "outlines/id_1.md")),
    ).toBeUndefined();
  });
});

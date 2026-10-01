import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { EventsOn } from "@/lib/wailsjs/runtime/runtime";
import ChapterManagementView from "./ChapterManagementView";

const { mockGetChapters, mockGetVolumes } = vi.hoisted(() => ({
  mockGetChapters: vi.fn(),
  mockGetVolumes: vi.fn(),
}));

vi.mock("@/lib/wailsjs/go/app/App", () => ({
  GetChapters: mockGetChapters,
  GetVolumes: mockGetVolumes,
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (
      key: string,
      options?: { n?: number; count?: number; start?: number; end?: number },
    ) => {
      if (key === "sidebar.chapterN") return `Ch.${options?.n}`;
      if (key === "sidebar.wordCount") return `${options?.count} words`;
      if (key === "sidebar.chapterCountShort")
        return `${options?.count} chapters`;
      if (key === "sidebar.chapterRange")
        return `Ch.${options?.start}-Ch.${options?.end}`;
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

beforeEach(() => {
  vi.clearAllMocks();
  mockGetChapters.mockResolvedValue([]);
  mockGetVolumes.mockResolvedValue([]);
  vi.mocked(EventsOn).mockReturnValue(vi.fn());
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
    expect(within(longVolume).getByRole("button")).toHaveAttribute(
      "aria-expanded",
      "false",
    );
    expect(screen.queryByText("长章1")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "长卷" }));
    expect(within(longVolume).getByRole("button")).toHaveAttribute(
      "aria-expanded",
      "true",
    );
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

  it("章节文件变化后重新读取当前小说章节", async () => {
    let onFileChanged:
      ((data: { novel_id: number; path: string }) => void) | undefined;
    vi.mocked(EventsOn).mockImplementation((_name, callback) => {
      onFileChanged = callback;
      return vi.fn();
    });
    renderView();
    await screen.findByText("chapterManagement.noVolumes");
    expect(mockGetChapters).toHaveBeenCalledTimes(1);

    act(() => onFileChanged?.({ novel_id: 2, path: "chapters/2.md" }));
    expect(mockGetChapters).toHaveBeenCalledTimes(1);
    act(() => onFileChanged?.({ novel_id: 1, path: "chapters/1.md" }));
    await waitFor(() => expect(mockGetChapters).toHaveBeenCalledTimes(2));
  });
});

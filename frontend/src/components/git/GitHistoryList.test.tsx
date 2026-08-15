import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactElement } from "react";
import GitHistoryList from "./GitHistoryList";
import { useGitStore } from "./useGitStore";
import { toastError } from "@/utils/toast";
import { installQueryErrorToast } from "@/lib/queryErrorToast";

// Mock toastError（捕获中间件调用）
vi.mock("@/utils/toast", async (importOriginal) => {
  const mod = await importOriginal<typeof import("@/utils/toast")>();
  return {
    ...mod,
    toastError: vi.fn(),
  };
});

// Mock 子组件 GitCommitTooltip（纯展示，不渲染内部）
vi.mock("./GitCommitTooltip", () => ({
  default: () => <div data-testid="commit-tooltip" />,
}));

// Mock useTimeAgo 返回固定字符串避免定时器干扰
vi.mock("@/hooks/useTimeAgo", () => ({
  useTimeAgo: () => () => "刚刚",
}));

// Mock react-intersection-observer：默认 inView=false（用 vi.hoisted 让测试中可改）
const { mockSetInView } = vi.hoisted(() => ({
  mockSetInView: vi.fn(),
}));
vi.mock("react-intersection-observer", () => ({
  useInView: () => {
    // 默认 inView=false，测试中通过 mockSetInView 改 inView
    let inView = false;
    mockSetInView.mockImplementation((v: boolean) => {
      inView = v;
    });
    return {
      ref: vi.fn(),
      inView,
    };
  },
}));

// Mock wailsjs App：覆盖 GetCommitLog/GetCommitFileList/GetFileDiff
const { mockGetCommitLog, mockGetCommitFileList, mockGetFileDiff, mockI18n } =
  vi.hoisted(() => ({
    mockGetCommitLog: vi.fn(),
    mockGetCommitFileList: vi.fn(),
    mockGetFileDiff: vi.fn(),
    // 中间件用 i18n.exists/t，mock 让 exists 返回 true + t 返回 key 本身
    mockI18n: {
      exists: vi.fn().mockReturnValue(true),
      t: vi.fn().mockImplementation((key: string) => key),
    },
  }));

vi.mock("@/lib/wailsjs/go/app/App", () => ({
  GetCommitLog: mockGetCommitLog,
  GetCommitFileList: mockGetCommitFileList,
  GetFileDiff: mockGetFileDiff,
}));

vi.mock("@/i18n", () => ({ default: mockI18n }));

let qc: QueryClient;
let unsub: () => void;

function renderWithProvider(ui: ReactElement) {
  qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  unsub = installQueryErrorToast(qc);
  return render(ui, {
    wrapper: ({ children }) => (
      <QueryClientProvider client={qc}>{children}</QueryClientProvider>
    ),
  });
}

const commit1 = {
  hash: "abc123",
  shortHash: "abc123",
  message: "first commit",
  time: new Date("2026-08-10").getTime(),
  authorName: "Alice",
  authorEmail: "alice@example.com",
  filesChanged: 2,
  insertions: 10,
  deletions: 5,
};
const commit2 = {
  hash: "def456",
  shortHash: "def456",
  message: "second commit",
  time: new Date("2026-08-11").getTime(),
  authorName: "Bob",
  authorEmail: "bob@example.com",
  filesChanged: 1,
  insertions: 3,
  deletions: 0,
};

describe("GitHistoryList", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // 3.8 后续：useGitStore 是全局单例，重置避免跨用例残留。
    useGitStore.setState({ selectedGitFile: null });
    mockGetCommitLog.mockResolvedValue([commit1, commit2]);
    mockGetCommitFileList.mockResolvedValue({
      commit: commit1,
      files: [],
    });
    mockGetFileDiff.mockResolvedValue({
      oldPath: "",
      newPath: "chapters/001.md",
      hunks: [],
    } as any);
  });

  afterEach(() => {
    unsub();
    qc.clear();
  });

  it("renders commits list after load", async () => {
    renderWithProvider(<GitHistoryList novelId={1} />);
    expect(await screen.findByText("first commit")).toBeInTheDocument();
    expect(screen.getByText("second commit")).toBeInTheDocument();
  });

  it("shows inline error + retry button when GetCommitLog fails", async () => {
    mockGetCommitLog.mockRejectedValueOnce(new Error("network timeout"));
    renderWithProvider(<GitHistoryList novelId={1} />);
    // 中间件触发 toast
    await vi.waitFor(() => {
      expect(toastError).toHaveBeenCalledWith(
        expect.stringContaining("git.commitsLoadFailed"),
      );
    });
    // 重试按钮显示
    expect(await screen.findByText("git.retry")).toBeInTheDocument();
    // 点击重试触发 refetch
    mockGetCommitLog.mockResolvedValueOnce([commit1]);
    fireEvent.click(screen.getByText("git.retry"));
    expect(await screen.findByText("first commit")).toBeInTheDocument();
  });

  it("shows noCommits when commit list is empty", async () => {
    mockGetCommitLog.mockResolvedValueOnce([]);
    renderWithProvider(<GitHistoryList novelId={1} />);
    expect(await screen.findByText("git.noCommits")).toBeInTheDocument();
  });

  it("expands commit and loads file list", async () => {
    mockGetCommitFileList.mockResolvedValueOnce({
      commit: commit1,
      files: [
        {
          path: "chapters/001.md",
          changeType: "added",
          oldPath: "",
        },
      ],
    });
    renderWithProvider(<GitHistoryList novelId={1} />);
    const item = await screen.findByText("first commit");
    fireEvent.click(item);
    expect(await screen.findByText("chapters/001.md")).toBeInTheDocument();
  });

  it("shows expandCommitFailed inline when GetCommitFileList fails", async () => {
    mockGetCommitFileList.mockRejectedValueOnce(new Error("git error"));
    renderWithProvider(<GitHistoryList novelId={1} />);
    const item = await screen.findByText("first commit");
    fireEvent.click(item);
    expect(
      await screen.findByText("git.expandCommitFailed"),
    ).toBeInTheDocument();
  });

  it("loads file diff and writes useGitStore when file selected", async () => {
    mockGetCommitFileList.mockResolvedValueOnce({
      commit: commit1,
      files: [
        {
          path: "chapters/001.md",
          changeType: "modified",
          oldPath: "",
        },
      ],
    });
    const diff = {
      oldPath: "",
      newPath: "chapters/001.md",
      hunks: [],
    };
    mockGetFileDiff.mockResolvedValueOnce(diff as any);
    renderWithProvider(<GitHistoryList novelId={1} />);
    const item = await screen.findByText("first commit");
    fireEvent.click(item);
    // 3.8 后续：自动选第一个文件，触发 useFileDiff refetch → 写 useGitStore（GitCommitView 订阅）
    await vi.waitFor(() => {
      expect(useGitStore.getState().selectedGitFile).toEqual(
        expect.objectContaining({ newPath: "chapters/001.md" }),
      );
    });
  });

  it("calls toastError via middleware when file diff fails", async () => {
    mockGetCommitFileList.mockResolvedValueOnce({
      commit: commit1,
      files: [
        {
          path: "chapters/001.md",
          changeType: "added",
          oldPath: "",
        },
      ],
    });
    mockGetFileDiff.mockRejectedValueOnce(new Error("diff fetch failed"));
    renderWithProvider(<GitHistoryList novelId={1} />);
    const item = await screen.findByText("first commit");
    fireEvent.click(item);
    // 中间件接管 file-diff 错误
    await vi.waitFor(() => {
      expect(toastError).toHaveBeenCalledWith(
        expect.stringContaining("git.fileDiffLoadFailed"),
      );
    });
  });

  it("refetches commits when novelId changes", async () => {
    const { rerender } = renderWithProvider(<GitHistoryList novelId={1} />);
    await screen.findByText("first commit");
    expect(mockGetCommitLog).toHaveBeenCalledWith(1, 50, "");
    // 切 novelId
    mockGetCommitLog.mockResolvedValueOnce([commit2]);
    rerender(
      <QueryClientProvider client={qc}>
        <GitHistoryList novelId={2} />
      </QueryClientProvider>,
    );
    await vi.waitFor(() => {
      expect(mockGetCommitLog).toHaveBeenCalledWith(2, 50, "");
    });
  });
});

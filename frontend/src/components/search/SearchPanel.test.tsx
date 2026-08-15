import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactElement } from "react";
import SearchPanel from "./SearchPanel";
import { useSearchStore } from "@/stores/useSearchStore";
import { toastError } from "@/utils/toast";
import { installQueryErrorToast } from "@/lib/queryErrorToast";

// Mock toastError（捕获调用，验证中间件触发）
vi.mock("@/utils/toast", async (importOriginal) => {
  const mod = await importOriginal<typeof import("@/utils/toast")>();
  return { ...mod, toastError: vi.fn() };
});

// Mock SearchAll（wailsjs）
const { mockSearchAll, mockI18n } = vi.hoisted(() => ({
  mockSearchAll: vi.fn(),
  mockI18n: {
    exists: vi.fn().mockReturnValue(true),
    t: vi.fn().mockImplementation((key: string) => key),
  },
}));

vi.mock("@/lib/wailsjs/go/app/App", async (importOriginal) => {
  const mod = await importOriginal<typeof import("@/lib/wailsjs/go/app/App")>();
  return { ...mod, SearchAll: mockSearchAll };
});

// Mock @/i18n：中间件 import i18n，让 exists/t 可控（返回 key 本身）
vi.mock("@/i18n", () => ({ default: mockI18n }));

let qc: QueryClient;
let unsub: () => void;

// 每个测试用独立 QueryClient（retry:false 避免重试延迟）+ 安装中间件让 GET 错误真实触发 toastError。
function renderWithProvider(ui: ReactElement) {
  qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  unsub = installQueryErrorToast(qc);
  return render(ui, {
    wrapper: ({ children }) => (
      <QueryClientProvider client={qc}>{children}</QueryClientProvider>
    ),
  });
}

const mockNavigateEntity = vi.fn();
const mockNavigateChapter = vi.fn();

function renderPanel(novelId = 1) {
  return renderWithProvider(
    <SearchPanel
      novelId={novelId}
      onNavigateEntity={mockNavigateEntity}
      onNavigateChapter={mockNavigateChapter}
    />,
  );
}

describe("SearchPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useSearchStore.setState({ query: "" });
    mockSearchAll.mockResolvedValue([]);
  });

  afterEach(() => {
    unsub();
    qc.clear();
  });

  it("shows input keyword hint when query empty", () => {
    renderPanel();
    expect(screen.getByText("search.inputKeyword")).toBeInTheDocument();
  });

  it("clears results during debounce then displays after fetch", async () => {
    // 5.5 commit 1：debounce 期间 isDebouncing=true → data=[] → 显示「无搜索结果」（规则 7 清空行为）。
    // debounce 300ms 后 fetch → 显示结果。
    mockSearchAll.mockResolvedValue([
      { type: "character", id: 1, title: "张三", panel_id: "characters" },
    ]);
    renderPanel();
    const input = screen.getByPlaceholderText("search.searchPlaceholder");
    fireEvent.change(input, { target: { value: "张" } });
    // debounce 期间立即检查：显示「无搜索结果」（清空行为）
    expect(screen.getByText("search.noResults")).toBeInTheDocument();
    // debounce 300ms 后 fetch → 显示结果
    expect(await screen.findByText("张三")).toBeInTheDocument();
    expect(mockSearchAll).toHaveBeenCalledWith(1, "张");
  });

  it("shows toastError and inline error when SearchAll fails", async () => {
    // 5.5 commit 1：GET 错误由中间件接管（queryErrorToast.ts），不再 silent catch。
    mockSearchAll.mockRejectedValue(new Error("network timeout"));
    renderPanel();
    fireEvent.change(screen.getByPlaceholderText("search.searchPlaceholder"), {
      target: { value: "张" },
    });
    // 中间件 fire toastError
    await vi.waitFor(() => {
      expect(toastError).toHaveBeenCalledWith(
        "search.loadFailed: network timeout",
      );
    });
    // inline 错误显示（五态渲染的 isError 分支）
    // findByText 轮询等待 error 状态 re-render（getByText 同步查会竞态）
    expect(await screen.findByText("search.loadFailed")).toBeInTheDocument();
  });

  it("shows noResults when search returns empty", async () => {
    mockSearchAll.mockResolvedValue([]);
    renderPanel();
    fireEvent.change(screen.getByPlaceholderText("search.searchPlaceholder"), {
      target: { value: "无匹配" },
    });
    expect(await screen.findByText("search.noResults")).toBeInTheDocument();
  });

  it("navigates entity on click", async () => {
    mockSearchAll.mockResolvedValue([
      { type: "character", id: 5, title: "李四", panel_id: "characters" },
    ]);
    renderPanel();
    fireEvent.change(screen.getByPlaceholderText("search.searchPlaceholder"), {
      target: { value: "李" },
    });
    const item = await screen.findByText("李四");
    fireEvent.click(item);
    // 4b: character 类型 focusType=undefined
    expect(mockNavigateEntity).toHaveBeenCalledWith("characters", 5, undefined);
  });

  it("navigates chapter on click", async () => {
    mockSearchAll.mockResolvedValue([
      {
        type: "chapter",
        id: 0,
        title: "第一章",
        panel_id: "",
        file_path: "chapters/001.md",
        chapter_num: 1,
      },
    ]);
    renderPanel();
    fireEvent.change(screen.getByPlaceholderText("search.searchPlaceholder"), {
      target: { value: "第一" },
    });
    const item = await screen.findByText("第一章");
    fireEvent.click(item);
    expect(mockNavigateChapter).toHaveBeenCalled();
  });

  it("does not fetch when novelId is 0", () => {
    // enabled 守卫：novelId=0 不 fetch
    useSearchStore.setState({ query: "张" });
    renderPanel(0);
    expect(mockSearchAll).not.toHaveBeenCalled();
  });
});

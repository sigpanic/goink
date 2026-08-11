import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactElement } from "react";
import PatternExtractView from "./PatternExtractView";
import { toastError } from "@/utils/toast";
import { installQueryErrorToast } from "@/lib/queryErrorToast";

// Mock toastError（捕获调用）
vi.mock("@/utils/toast", async (importOriginal) => {
  const mod = await importOriginal<typeof import("@/utils/toast")>();
  return {
    ...mod,
    toastError: vi.fn(),
  };
});

// Mock child components
vi.mock("./ChapterRangeInput", () => ({
  default: () => <div data-testid="chapter-range-input" />,
}));

vi.mock("./PatternSessionView", () => ({
  default: ({ title }: any) => (
    <div data-testid="pattern-session-view">{title}</div>
  ),
}));

vi.mock("@/components/chat/PopSelect", () => ({
  default: ({ value, options, onChange }: any) => (
    <select
      value={value}
      onChange={(e) => onChange(e.target.value)}
      data-testid="pop-select"
    >
      {options?.map((o: any) => (
        <option key={o.value} value={o.value}>
          {o.label}
        </option>
      ))}
    </select>
  ),
}));

// useNovels mock（PatternExtractView 用 useNovels 拉 novels 列表 + refetch）
vi.mock("@/components/novel/useNovels", () => ({
  useNovels: () => ({ data: [], refetch: vi.fn() }),
}));

// 5.3 pattern commit 1: chapters/models/settings 走 query（useChapters/useModels/useSettings）。
// mock wailsjs App：覆盖 query（GetChapters/GetModels/GetSettings）供 hook 调用。
// 用 vi.hoisted 提升，让 vi.mock 工厂能引用。
const { mockGetChapters, mockGetModels, mockGetSettings, mockI18n } =
  vi.hoisted(() => ({
    mockGetChapters: vi.fn(),
    mockGetModels: vi.fn(),
    mockGetSettings: vi.fn(),
    // 中间件用 i18n.exists/t，mock 让 exists 返回 true + t 返回 key 本身（对齐现有断言文案）。
    mockI18n: {
      exists: vi.fn().mockReturnValue(true),
      t: vi.fn().mockImplementation((key: string) => key),
    },
  }));

// mock wailsjs App：覆盖 PatternExtractView 使用的 wailsjs 函数。
// query（GetChapters/GetModels/GetSettings）供 useChapters/useModels/useSettings hook 调用。
vi.mock("@/lib/wailsjs/go/app/App", async (importOriginal) => {
  const mod =
    await importOriginal<typeof import("@/lib/wailsjs/go/app/App")>();
  return {
    ...mod,
    GetChapters: mockGetChapters,
    GetModels: mockGetModels,
    GetSettings: mockGetSettings,
  };
});

// mock @/i18n：中间件 import i18n，让 exists/t 可控（返回 key 本身，对齐组件 t 的 fallback 文案）。
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

describe("PatternExtractView", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetChapters.mockResolvedValue([]);
    mockGetModels.mockResolvedValue([]);
    mockGetSettings.mockResolvedValue({ selected_model_key: "" });
  });

  afterEach(() => {
    unsub();
    qc.clear();
  });

  it("renders empty state when no chapters", async () => {
    renderWithProvider(<PatternExtractView currentNovelId={1} />);
    expect(
      await screen.findByText("extract.noChaptersYet"),
    ).toBeInTheDocument();
  });

  it("displays chapter cards", async () => {
    mockGetChapters.mockResolvedValue([
      { id: 1, chapter_number: 1, title: "Ch1", word_count: 100 },
      { id: 2, chapter_number: 2, title: "Ch2", word_count: 200 },
      { id: 3, chapter_number: 3, title: "Ch3", word_count: 300 },
      { id: 4, chapter_number: 4, title: "Ch4", word_count: 400 },
      { id: 5, chapter_number: 5, title: "Ch5", word_count: 500 },
      { id: 6, chapter_number: 6, title: "Ch6", word_count: 600 },
    ]);
    mockGetModels.mockResolvedValue([
      {
        Key: "openai/gpt-4",
        ModelName: "GPT-4",
        ProviderName: "openai",
        ModelID: "gpt-4",
      },
    ]);
    mockGetSettings.mockResolvedValue({ selected_model_key: "openai/gpt-4" });

    renderWithProvider(<PatternExtractView currentNovelId={1} />);
    expect(await screen.findByText(/Ch1/)).toBeInTheDocument();
    expect(screen.getByText(/Ch6/)).toBeInTheDocument();
  });

  it("shows toastError when load chapters fails", async () => {
    // GET 错误由中间件接管：GetChapters 抛错 → QueryCache error 事件 → 中间件 fire toastError。
    mockGetChapters.mockRejectedValue(new Error("network timeout"));
    renderWithProvider(<PatternExtractView currentNovelId={1} />);
    await vi.waitFor(() => {
      expect(toastError).toHaveBeenCalledWith(
        "chapter.loadFailed: network timeout",
      );
    });
  });

  it("restores selected model from settings", async () => {
    mockGetModels.mockResolvedValue([
      {
        Key: "openai/gpt-4",
        ModelName: "GPT-4",
        ProviderName: "openai",
        ModelID: "gpt-4",
      },
      {
        Key: "anthropic/claude",
        ModelName: "Claude",
        ProviderName: "anthropic",
        ModelID: "claude",
      },
    ]);
    mockGetSettings.mockResolvedValue({
      selected_model_key: "anthropic/claude",
    });

    renderWithProvider(<PatternExtractView currentNovelId={1} />);
    // model PopSelect 是 toolbar 第二个 pop-select（第一个是 novel 选择）。
    await vi.waitFor(() => {
      const modelSelect = screen.getAllByTestId("pop-select")[1];
      expect(modelSelect).toHaveValue("anthropic/claude");
    });
  });

  it("disables extract button when fewer than 5 chapters", async () => {
    mockGetChapters.mockResolvedValue([
      { id: 1, chapter_number: 1, title: "Ch1", word_count: 100 },
      { id: 2, chapter_number: 2, title: "Ch2", word_count: 200 },
      { id: 3, chapter_number: 3, title: "Ch3", word_count: 300 },
      { id: 4, chapter_number: 4, title: "Ch4", word_count: 400 },
    ]);
    mockGetModels.mockResolvedValue([
      {
        Key: "openai/gpt-4",
        ModelName: "GPT-4",
        ProviderName: "openai",
        ModelID: "gpt-4",
      },
    ]);
    mockGetSettings.mockResolvedValue({ selected_model_key: "openai/gpt-4" });

    renderWithProvider(<PatternExtractView currentNovelId={1} />);
    const extractBtn = await screen.findByText("extract.startExtract");
    expect(extractBtn.closest("button")).toBeDisabled();
  });

  it("switches to session view when extract clicked", async () => {
    const user = userEvent.setup();
    mockGetChapters.mockResolvedValue([
      { id: 1, chapter_number: 1, title: "Ch1", word_count: 100 },
      { id: 2, chapter_number: 2, title: "Ch2", word_count: 200 },
      { id: 3, chapter_number: 3, title: "Ch3", word_count: 300 },
      { id: 4, chapter_number: 4, title: "Ch4", word_count: 400 },
      { id: 5, chapter_number: 5, title: "Ch5", word_count: 500 },
      { id: 6, chapter_number: 6, title: "Ch6", word_count: 600 },
    ]);
    mockGetModels.mockResolvedValue([
      {
        Key: "openai/gpt-4",
        ModelName: "GPT-4",
        ProviderName: "openai",
        ModelID: "gpt-4",
      },
    ]);
    mockGetSettings.mockResolvedValue({ selected_model_key: "openai/gpt-4" });

    renderWithProvider(<PatternExtractView currentNovelId={1} />);
    const extractBtn = await screen.findByText("extract.startExtract");
    await user.click(extractBtn);
    expect(
      await screen.findByTestId("pattern-session-view"),
    ).toBeInTheDocument();
  });
});

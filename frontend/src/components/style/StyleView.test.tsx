import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactElement } from "react";
import StyleView from "./StyleView";
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
vi.mock("./StyleSampleCard", () => ({
  default: ({ sample, selected, onToggle, onDelete }: any) => (
    <div data-testid={`card-${sample.id}`}>
      <span>{sample.name}</span>
      <button onClick={onToggle}>{selected ? "deselect" : "select"}</button>
      <button onClick={onDelete}>delete</button>
    </div>
  ),
}));

vi.mock("@/components/Markdown", () => ({
  default: ({ content }: any) => <div>{content}</div>,
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

vi.mock("@/components/shared/TagInput", () => ({
  default: () => null,
}));

// 3.9: StyleView 改用 useNovels query（不再走 useApp.GetNovels）。mock 返回空数组。
vi.mock("@/components/novel/useNovels", () => ({
  useNovels: () => ({ data: [] }),
}));

// 5.3 commit 4: model 走 useModels/useSettings query（共享 5.1 chat 缓存）+ SaveContent 走 useSaveContent mutation（5.2），废弃 splitModelKey。
// mock wailsjs App：覆盖 query（List/GetStyleSample + GetModels/GetSettings 供 useModels/useSettings 调用）+ mutation（Create/Update/Delete + SaveContent 供 useSaveContent 调用）+ 直接调用（ExtractStyle/CancelExtract）。
// 用 vi.hoisted 提升，让 vi.mock 工厂能引用。
const {
  mockListStyleSamples,
  mockGetStyleSample,
  mockCreateStyleSample,
  mockDeleteStyleSample,
  mockUpdateStyleSample,
  mockGetModels,
  mockGetSettings,
  mockI18n,
} = vi.hoisted(() => ({
  mockListStyleSamples: vi.fn(),
  mockGetStyleSample: vi.fn(),
  mockCreateStyleSample: vi.fn(),
  mockDeleteStyleSample: vi.fn(),
  mockUpdateStyleSample: vi.fn(),
  mockGetModels: vi.fn(),
  mockGetSettings: vi.fn(),
  // 中间件用 i18n.exists/t，mock 让 exists 返回 true + t 返回 key 本身（对齐现有断言文案）。
  mockI18n: {
    exists: vi.fn().mockReturnValue(true),
    t: vi.fn().mockImplementation((key: string) => key),
  },
}));

// mock wailsjs App：覆盖所有 StyleView 使用的 wailsjs 函数。
// query（List/GetStyleSample/GetModels/GetSettings）+ mutation（Create/Update/Delete/SaveContent）+ 直接调用（ExtractStyle/CancelExtract）。
// useModels/useSettings/useSaveContent hook 内部 import wailsjs 函数，mock 后 hook 自动调到。
vi.mock("@/lib/wailsjs/go/app/App", async (importOriginal) => {
  const mod =
    await importOriginal<typeof import("@/lib/wailsjs/go/app/App")>();
  return {
    ...mod,
    ListStyleSamples: mockListStyleSamples,
    GetStyleSample: mockGetStyleSample,
    CreateStyleSample: mockCreateStyleSample,
    UpdateStyleSample: mockUpdateStyleSample,
    DeleteStyleSample: mockDeleteStyleSample,
    GetModels: mockGetModels,
    GetSettings: mockGetSettings,
    ExtractStyle: vi.fn(),
    CancelExtract: vi.fn(),
    SaveContent: vi.fn(),
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

describe("StyleView", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockListStyleSamples.mockResolvedValue({
      items: [],
      total: 0,
      total_pages: 0,
    });
    mockGetModels.mockResolvedValue([]);
    mockGetSettings.mockResolvedValue({ selected_model_key: "" });
  });

  afterEach(() => {
    unsub();
    qc.clear();
  });

  it("renders empty state when no samples", async () => {
    renderWithProvider(<StyleView />);
    expect(
      await screen.findByText("styleSample.noStyleSamples"),
    ).toBeInTheDocument();
  });

  it("displays sample cards", async () => {
    mockListStyleSamples.mockResolvedValue({
      items: [
        {
          id: 1,
          name: "Suspense",
          content: "...",
          tags: [],
          is_global: true,
          novel_id: 0,
        },
        {
          id: 2,
          name: "Dialogue",
          content: "...",
          tags: [],
          is_global: true,
          novel_id: 0,
        },
      ],
      total: 2,
      total_pages: 1,
    });
    renderWithProvider(<StyleView />);
    expect(await screen.findByText("Suspense")).toBeInTheDocument();
    expect(screen.getByText("Dialogue")).toBeInTheDocument();
  });

  it("shows toastError when load fails", async () => {
    // GET 错误由中间件接管：ListStyleSamples 抛错 → QueryCache error 事件 → 中间件 fire toastError。
    mockListStyleSamples.mockRejectedValue(new Error("network timeout"));
    renderWithProvider(<StyleView />);
    await vi.waitFor(() => {
      expect(toastError).toHaveBeenCalledWith(
        "styleSample.loadFailed: network timeout",
      );
    });
  });

  it("shows toastError when delete fails", async () => {
    mockListStyleSamples.mockResolvedValue({
      items: [
        {
          id: 1,
          name: "Suspense",
          content: "...",
          tags: [],
          is_global: true,
          novel_id: 0,
        },
      ],
      total: 1,
      total_pages: 1,
    });
    // mutation 错误由 confirmDelete 的 try/catch + toastError（组件级，不走全局中间件）。
    mockDeleteStyleSample.mockRejectedValue(new Error("db error"));

    renderWithProvider(<StyleView />);
    expect(await screen.findByText("Suspense")).toBeInTheDocument();

    const deleteBtn = screen.getByText("delete");
    fireEvent.click(deleteBtn);

    // 删除按钮现在弹出 ConfirmDialog，需点确认才执行删除
    const confirmBtn = await screen.findByText("common.delete");
    fireEvent.click(confirmBtn);

    await vi.waitFor(() => {
      expect(toastError).toHaveBeenCalledWith(
        "styleSample.deleteFailed: db error",
      );
    });
  });

  it("switches to adding phase when add button clicked", async () => {
    const user = userEvent.setup();
    renderWithProvider(<StyleView />);
    expect(
      await screen.findByText("styleSample.noStyleSamples"),
    ).toBeInTheDocument();

    const addBtn = screen.getByText("styleSample.addSample");
    await user.click(addBtn);

    // Should show the add form with name input
    expect(
      screen.getByPlaceholderText("styleSample.sampleNamePlaceholder"),
    ).toBeInTheDocument();
  });

  it("shows toastError when openDetail fails", async () => {
    mockListStyleSamples.mockResolvedValue({
      items: [
        {
          id: 1,
          name: "Suspense",
          content: "...",
          tags: [],
          is_global: true,
          novel_id: 0,
        },
      ],
      total: 1,
      total_pages: 1,
    });
    // detail query 抛错 → 中间件 fire toastError（不再组件级 try/catch）。
    mockGetStyleSample.mockRejectedValue(new Error("not found"));

    renderWithProvider(<StyleView />);
    expect(await screen.findByText("Suspense")).toBeInTheDocument();

    // focusId 触发 openDetail → setDetailId → useStyleSample fetch → error → 中间件 fire
    renderWithProvider(<StyleView focusId={1} onFocusHandled={vi.fn()} />);
    await vi.waitFor(() => {
      expect(toastError).toHaveBeenCalledWith(
        "styleSample.loadFailed: not found",
      );
    });
  });
});

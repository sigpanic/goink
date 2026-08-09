import { describe, it, expect, vi, beforeEach } from "vitest";
import {
  render as originalRender,
  screen,
  fireEvent,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactElement } from "react";
import LocationList from "./LocationList";

// 4.2.1: useLocations 引入 useQuery，render 需包 QueryClientProvider。
// 每个测试用独立 QueryClient（retry:false 避免重试），无状态残留。
function render(ui: ReactElement) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return originalRender(ui, {
    wrapper: ({ children }) => (
      <QueryClientProvider client={qc}>{children}</QueryClientProvider>
    ),
  });
}

// 4.2.1: locations 数据走 useLocations query（不再走 useApp.GetLocations）。
// mockUseLocations 用 vi.hoisted 提升，让 vi.mock 工厂能引用（vi.mock 自身被提升到文件顶部）。
const { mockUseLocations, mockSetDeletingLocationId } = vi.hoisted(() => ({
  mockUseLocations: vi.fn(),
  mockSetDeletingLocationId: vi.fn(),
}));

vi.mock("@/components/location/useLocations", () => ({
  useLocations: mockUseLocations,
}));

// 4.2.2: 删除合并 —— LocationList 只 dispatch setDeletingLocationId，
// ConfirmDialog + 执行集中在 LocationListView。mock store 的 selector 取 setter 断言被调。
vi.mock("@/components/location/useLocationStore", () => ({
  useLocationStore: (
    selector: (s: {
      setDeletingLocationId: ReturnType<typeof vi.fn>;
    }) => unknown,
  ) => selector({ setDeletingLocationId: mockSetDeletingLocationId }),
}));

describe("LocationList", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // 默认返回空数组（避免 undefined 报错）
    mockUseLocations.mockReturnValue({
      data: [],
      isLoading: false,
      isError: false,
    });
  });

  it("renders empty state when no locations", async () => {
    render(<LocationList novelId={1} />);
    // useLocations mock 同步返回 data，但 render 仍需 await 等待 React 完成
    expect(await screen.findByText("location.noLocations")).toBeInTheDocument();
  });

  it("shows locationsLoadFailed when isError", async () => {
    mockUseLocations.mockReturnValue({
      data: [],
      isLoading: false,
      isError: true,
    });
    render(<LocationList novelId={1} />);
    expect(
      await screen.findByText("location.locationsLoadFailed"),
    ).toBeInTheDocument();
  });

  it("displays location tree", async () => {
    mockUseLocations.mockReturnValue({
      data: [
        {
          id: 1,
          name: "Castle",
          parent_location_id: null,
          location_type: "building",
        },
        {
          id: 2,
          name: "Throne Room",
          parent_location_id: 1,
          location_type: "room",
        },
      ],
      isLoading: false,
      isError: false,
    });
    render(<LocationList novelId={1} />);
    expect(await screen.findByText("Castle")).toBeInTheDocument();
    expect(await screen.findByText("Throne Room")).toBeInTheDocument();
  });

  // 4.2.2: 删除合并后 LocationList 只 dispatch setDeletingLocationId，
  // 不再挂 ConfirmDialog / 执行删除。断言 dispatch 被调（测"调用什么 action"原则，2.2）。
  // 删除执行 + toast 失败的覆盖在 LocationListView。
  it("dispatches setDeletingLocationId on delete click", async () => {
    mockUseLocations.mockReturnValue({
      data: [
        { id: 1, name: "Castle", parent_location_id: null, location_type: "" },
      ],
      isLoading: false,
      isError: false,
    });
    render(<LocationList novelId={1} />);
    expect(await screen.findByText("Castle")).toBeInTheDocument();

    const deleteBtn = screen.getByTitle("location.delete");
    fireEvent.click(deleteBtn);

    await vi.waitFor(() => {
      expect(mockSetDeletingLocationId).toHaveBeenCalledWith(1);
    });
  });
});

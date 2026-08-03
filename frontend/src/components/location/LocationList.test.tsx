import { describe, it, expect, vi, beforeEach } from "vitest";
import { render as originalRender, screen, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactElement } from "react";
import LocationList from "./LocationList";
import { toastError } from "@/utils/toast";

// Mock toastError
vi.mock("@/utils/toast", async (importOriginal) => {
  const mod = await importOriginal<typeof import("@/utils/toast")>();
  return {
    ...mod,
    toastError: vi.fn(),
  };
});

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
const { mockUseLocations } = vi.hoisted(() => ({
  mockUseLocations: vi.fn(),
}));

vi.mock("@/components/location/useLocations", () => ({
  useLocations: mockUseLocations,
}));

// DeleteLocation 仍走 useApp（commit 3 再 mutation 化），mock useApp 只保留 DeleteLocation。
const mockDeleteLocation = vi.fn();

vi.mock("@/hooks/useApp", () => ({
  useApp: () => ({
    DeleteLocation: mockDeleteLocation,
  }),
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
    expect(
      await screen.findByText("location.noLocations"),
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

  it("shows toastError when delete fails", async () => {
    mockUseLocations.mockReturnValue({
      data: [
        { id: 1, name: "Castle", parent_location_id: null, location_type: "" },
      ],
      isLoading: false,
      isError: false,
    });
    mockDeleteLocation.mockRejectedValue(new Error("has children"));

    render(<LocationList novelId={1} />);
    expect(await screen.findByText("Castle")).toBeInTheDocument();

    const deleteBtn = screen.getByTitle("location.delete");
    fireEvent.click(deleteBtn);

    // 删除按钮现在弹出 ConfirmDialog，需点确认才执行删除
    const confirmBtn = await screen.findByText("common.delete");
    fireEvent.click(confirmBtn);

    await vi.waitFor(() => {
      expect(toastError).toHaveBeenCalledWith(
        "location.deleteFailed: has children",
      );
    });
  });

  it("does not call DeleteLocation when confirm is cancelled", async () => {
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

    // ConfirmDialog 弹出后点取消（common.cancel），不应执行删除
    const cancelBtn = await screen.findByText("common.cancel");
    fireEvent.click(cancelBtn);

    expect(mockDeleteLocation).not.toHaveBeenCalled();
  });
});

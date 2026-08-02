import { describe, it, expect, vi, beforeEach } from "vitest";
import { render as originalRender, screen, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactElement } from "react";
import CharacterList from "./CharacterList";
import { toastError } from "@/utils/toast";

// Mock toastError
vi.mock("@/utils/toast", async (importOriginal) => {
  const mod = await importOriginal<typeof import("@/utils/toast")>();
  return {
    ...mod,
    toastError: vi.fn(),
  };
});

// 4.1.1: useCharacters 引入 useQuery，render 需包 QueryClientProvider。
// 每个测试用独立 QueryClient（retry:false 避免重试），无状态残留。
function render(ui: ReactElement) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return originalRender(ui, {
    wrapper: ({ children }) => (
      <QueryClientProvider client={qc}>{children}</QueryClientProvider>
    ),
  });
}

// 4.1.1: characters 数据走 useCharacters query（不再走 useApp.GetCharacters）。
// mockUseCharacters 用 vi.hoisted 提升，让 vi.mock 工厂能引用（vi.mock 自身被提升到文件顶部）。
const { mockUseCharacters } = vi.hoisted(() => ({
  mockUseCharacters: vi.fn(),
}));

vi.mock("@/components/character/useCharacters", () => ({
  useCharacters: mockUseCharacters,
}));

// Mock useApp — DeleteCharacter 仍走 useApp（mockGetCharacters 已不需要）
const mockDeleteCharacter = vi.fn();

vi.mock("@/hooks/useApp", () => ({
  useApp: () => ({
    DeleteCharacter: mockDeleteCharacter,
  }),
}));

describe("CharacterList", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // 默认返回空数组（避免 undefined 报错）
    mockUseCharacters.mockReturnValue({
      data: [],
      isLoading: false,
      isError: false,
    });
    mockDeleteCharacter.mockResolvedValue(undefined);
  });

  it("renders empty state when no characters", async () => {
    render(<CharacterList novelId={1} />);
    // useCharacters mock 同步返回 data，但 render 仍需 await 等待 React 完成
    expect(
      await screen.findByText("character.noCharacters"),
    ).toBeInTheDocument();
  });

  it("displays character list", async () => {
    mockUseCharacters.mockReturnValue({
      data: [
        { id: 1, name: "Alice" },
        { id: 2, name: "Bob" },
      ],
      isLoading: false,
      isError: false,
    });
    render(<CharacterList novelId={1} />);
    expect(await screen.findByText("Alice")).toBeInTheDocument();
    expect(screen.getByText("Bob")).toBeInTheDocument();
  });

  it("filters characters by search", async () => {
    const user = userEvent.setup();
    mockUseCharacters.mockReturnValue({
      data: [
        { id: 1, name: "Alice" },
        { id: 2, name: "Bob" },
      ],
      isLoading: false,
      isError: false,
    });
    render(<CharacterList novelId={1} />);
    expect(await screen.findByText("Alice")).toBeInTheDocument();

    const searchInput = screen.getByPlaceholderText(
      "character.searchCharacter",
    );
    await user.type(searchInput, "ali");

    expect(screen.getByText("Alice")).toBeInTheDocument();
    expect(screen.queryByText("Bob")).not.toBeInTheDocument();
  });

  it("calls DeleteCharacter on confirm and reloads", async () => {
    mockUseCharacters.mockReturnValue({
      data: [{ id: 1, name: "Alice" }],
      isLoading: false,
      isError: false,
    });
    mockDeleteCharacter.mockResolvedValue(undefined);

    render(<CharacterList novelId={1} />);
    expect(await screen.findByText("Alice")).toBeInTheDocument();

    const deleteBtn = screen.getByTitle("character.delete");
    fireEvent.click(deleteBtn);

    // 删除按钮现在弹出 ConfirmDialog，需点确认才执行删除
    const confirmBtn = await screen.findByText("common.delete");
    fireEvent.click(confirmBtn);

    await vi.waitFor(() => {
      expect(mockDeleteCharacter).toHaveBeenCalledWith(1, 1);
    });
  });

  it("shows toastError when delete fails", async () => {
    mockUseCharacters.mockReturnValue({
      data: [{ id: 1, name: "Alice" }],
      isLoading: false,
      isError: false,
    });
    mockDeleteCharacter.mockRejectedValue(new Error("network error"));

    render(<CharacterList novelId={1} />);
    expect(await screen.findByText("Alice")).toBeInTheDocument();

    const deleteBtn = screen.getByTitle("character.delete");
    fireEvent.click(deleteBtn);

    const confirmBtn = await screen.findByText("common.delete");
    fireEvent.click(confirmBtn);

    await vi.waitFor(() => {
      expect(toastError).toHaveBeenCalledWith(
        "character.deleteFailed: network error",
      );
    });
  });
});

import { describe, it, expect, vi, beforeEach } from "vitest";
import {
  render as originalRender,
  screen,
  fireEvent,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactElement } from "react";
import CharacterList from "./CharacterList";

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
const { mockUseCharacters, mockSetDeletingCharacterId } = vi.hoisted(() => ({
  mockUseCharacters: vi.fn(),
  mockSetDeletingCharacterId: vi.fn(),
}));

vi.mock("@/components/character/useCharacters", () => ({
  useCharacters: mockUseCharacters,
}));

// 4.1.2: 删除合并 —— CharacterList 只 dispatch setDeletingCharacterId，
// ConfirmDialog + 执行集中在 CharacterListView。mock store 的 selector 取 setter 断言被调。
vi.mock("@/components/character/useCharacterStore", () => ({
  useCharacterStore: (
    selector: (s: {
      setDeletingCharacterId: ReturnType<typeof vi.fn>;
    }) => unknown,
  ) => selector({ setDeletingCharacterId: mockSetDeletingCharacterId }),
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
  });

  it("renders empty state when no characters", async () => {
    render(<CharacterList novelId={1} />);
    // useCharacters mock 同步返回 data，但 render 仍需 await 等待 React 完成
    expect(
      await screen.findByText("character.noCharacters"),
    ).toBeInTheDocument();
  });

  it("shows charsLoadFailed when isError", async () => {
    mockUseCharacters.mockReturnValue({
      data: [],
      isLoading: false,
      isError: true,
    });
    render(<CharacterList novelId={1} />);
    expect(
      await screen.findByText("character.charsLoadFailed"),
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

  // 4.1.2: 删除合并后 CharacterList 只 dispatch setDeletingCharacterId，
  // 不再挂 ConfirmDialog / 执行删除。断言 dispatch 被调（测"调用什么 action"原则，2.2）。
  // 删除执行 + toast 失败的覆盖在 CharacterListView（本次未加测试，靠手测点覆盖）。
  it("dispatches setDeletingCharacterId on delete click", async () => {
    mockUseCharacters.mockReturnValue({
      data: [{ id: 1, name: "Alice" }],
      isLoading: false,
      isError: false,
    });
    render(<CharacterList novelId={1} />);
    expect(await screen.findByText("Alice")).toBeInTheDocument();

    const deleteBtn = screen.getByTitle("character.delete");
    fireEvent.click(deleteBtn);

    await vi.waitFor(() => {
      expect(mockSetDeletingCharacterId).toHaveBeenCalledWith(1);
    });
  });
});

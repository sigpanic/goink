import { describe, it, expect, vi, beforeEach } from "vitest";
import {
  render as originalRender,
  screen,
  fireEvent,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactElement } from "react";
import CharacterList from "./CharacterList";
import { useCharacterGroupStore } from "./useCharacterGroupStore";
import { useFocusStore } from "@/stores/useFocusStore";

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
const {
  mockUseCharacters,
  mockSetDeletingCharacterId,
  mockUseGroups,
  mockUseMembers,
} = vi.hoisted(() => ({
  mockUseCharacters: vi.fn(),
  mockSetDeletingCharacterId: vi.fn(),
  mockUseGroups: vi.fn(),
  mockUseMembers: vi.fn(),
}));

vi.mock("@/components/character/useCharacters", () => ({
  useCharacters: mockUseCharacters,
}));

vi.mock("./useCharacterGroups", () => ({
  useCharacterGroups: mockUseGroups,
  useCharacterGroupMembers: mockUseMembers,
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
    useCharacterGroupStore.setState({
      selectedGroups: {},
      viewTabs: {},
      editingGroups: {},
      deletingGroups: {},
    });
    useFocusStore.setState({ focusMap: {} });
    mockUseGroups.mockReturnValue({
      data: [],
      isLoading: false,
      isError: false,
    });
    mockUseMembers.mockReturnValue({
      data: [],
      isLoading: false,
      isError: false,
    });
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

  function groupedData() {
    mockUseCharacters.mockReturnValue({
      data: [
        { id: 1, name: "Alice" },
        { id: 2, name: "Bob" },
      ],
      isLoading: false,
      isError: false,
    });
    mockUseGroups.mockReturnValue({
      data: [
        { id: 10, name: "Heroes", member_count: 1 },
        { id: 20, name: "Guild", member_count: 1 },
      ],
      isLoading: false,
      isError: false,
    });
    mockUseMembers.mockReturnValue({
      data: [
        { character_id: 1, group_id: 10 },
        { character_id: 1, group_id: 20 },
      ],
      isLoading: false,
      isError: false,
    });
  }

  it("shows a multi-group character in each expanded group and keeps totals unique", async () => {
    groupedData();
    render(<CharacterList novelId={1} />);
    const heroes = screen.getByRole("region", { name: "Heroes" });
    const guild = screen.getByRole("region", { name: "Guild" });
    const ungrouped = screen.getByRole("region", {
      name: "characterGroup.ungrouped",
    });
    expect(within(heroes).getByText("Alice")).toBeInTheDocument();
    expect(within(guild).queryByText("Alice")).not.toBeInTheDocument();
    fireEvent.click(
      within(guild).getByRole("button", { name: "characterGroup.toggle" }),
    );
    fireEvent.click(
      within(ungrouped).getByRole("button", { name: "characterGroup.toggle" }),
    );
    expect(screen.getAllByText("Alice")).toHaveLength(2);
    expect(within(ungrouped).getByText("Bob")).toBeInTheDocument();
    expect(screen.getByText("character.character (2)")).toBeInTheDocument();
  });

  it("temporarily expands search matches and restores collapsed groups when cleared", async () => {
    const user = userEvent.setup();
    groupedData();
    render(<CharacterList novelId={1} />);
    const heroes = screen.getByRole("region", { name: "Heroes" });
    const toggle = within(heroes).getByRole("button", {
      name: "characterGroup.toggle",
    });
    fireEvent.click(toggle);
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    const search = screen.getByPlaceholderText("character.searchCharacter");
    await user.type(search, "ali");
    expect(screen.getAllByText("Alice")).toHaveLength(2);
    expect(screen.queryByText("Bob")).not.toBeInTheDocument();
    await user.clear(search);
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByText("Alice")).not.toBeInTheDocument();
  });

  it("separates group navigation from collapsing and focuses a member in that group", () => {
    groupedData();
    render(<CharacterList novelId={1} />);
    const heroes = screen.getByRole("region", { name: "Heroes" });
    fireEvent.click(within(heroes).getByRole("button", { name: "Heroes 1" }));
    expect(useCharacterGroupStore.getState().selectedGroups[1]).toBe(10);
    expect(within(heroes).getByText("Alice")).toBeInTheDocument();
    fireEvent.click(within(heroes).getByRole("button", { name: "Alice" }));
    expect(useFocusStore.getState().focusMap.characters?.id).toBe(1);
    expect(useCharacterGroupStore.getState().viewTabs[1]).toBe("list");
  });

  it("keeps empty groups visible and reports membership loading failures", () => {
    mockUseGroups.mockReturnValue({
      data: [{ id: 10, name: "Empty", member_count: 0 }],
      isLoading: false,
      isError: false,
    });
    const view = render(<CharacterList novelId={1} />);
    expect(screen.getByRole("region", { name: "Empty" })).toBeInTheDocument();
    expect(screen.getByText("characterGroup.empty")).toBeInTheDocument();
    mockUseMembers.mockReturnValue({
      data: [],
      isLoading: false,
      isError: true,
    });
    view.rerender(<CharacterList novelId={1} />);
    expect(screen.getByText("character.charsLoadFailed")).toBeInTheDocument();
    expect(
      screen.queryByRole("region", { name: "characterGroup.ungrouped" }),
    ).not.toBeInTheDocument();
  });

  it("keeps group selections and expansion isolated when switching novels", () => {
    groupedData();
    const view = render(<CharacterList novelId={1} />);
    const heroes = screen.getByRole("region", { name: "Heroes" });
    fireEvent.click(within(heroes).getByRole("button", { name: "Heroes 1" }));
    fireEvent.click(
      within(heroes).getByRole("button", { name: "characterGroup.toggle" }),
    );
    view.rerender(<CharacterList novelId={2} />);
    expect(
      screen.getByRole("button", { name: "characterGroup.all 2" }),
    ).toHaveAttribute("aria-pressed", "true");
    expect(
      within(screen.getByRole("region", { name: "Heroes" })).getByText("Alice"),
    ).toBeInTheDocument();
    view.rerender(<CharacterList novelId={1} />);
    expect(
      within(screen.getByRole("region", { name: "Heroes" })).queryByText(
        "Alice",
      ),
    ).not.toBeInTheDocument();
  });
});

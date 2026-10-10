import { describe, it, expect, vi, beforeEach } from "vitest";
import {
  render as originalRender,
  screen,
  fireEvent,
  waitFor,
  act,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactElement } from "react";
import CharacterListView from "./CharacterListView";
import { useCharacterStore } from "./useCharacterStore";
import { useCharacterGroupStore } from "./useCharacterGroupStore";
import { toastError } from "@/utils/toast";

// Mock toastError
vi.mock("@/utils/toast", async (importOriginal) => {
  const mod = await importOriginal<typeof import("@/utils/toast")>();
  return { ...mod, toastError: vi.fn() };
});

// 4.1.2: create/update/delete mutation 全 mock；useCharacters mock。
// useApp 已不再用于 CRUD（mutation 直接 import wailsjs），删 useApp mock。
const {
  mockUseCharacters,
  mockMutateAsync,
  mockCreateMutateAsync,
  mockUpdateMutateAsync,
  mockUseGroups,
  mockUseMembers,
  mockCreateGroup,
  mockUpdateGroup,
  mockDeleteGroup,
  mockMemberships,
  mockFocus,
} = vi.hoisted(() => ({
  mockUseCharacters: vi.fn(),
  mockMutateAsync: vi.fn(),
  mockCreateMutateAsync: vi.fn(),
  mockUpdateMutateAsync: vi.fn(),
  mockUseGroups: vi.fn(),
  mockUseMembers: vi.fn(),
  mockCreateGroup: vi.fn(),
  mockUpdateGroup: vi.fn(),
  mockDeleteGroup: vi.fn(),
  mockMemberships: vi.fn(),
  mockFocus: vi.fn(),
}));

vi.mock("@/components/character/useCharacters", () => ({
  useCharacters: mockUseCharacters,
}));

vi.mock("./useCharacterGroups", () => ({
  useCharacterGroups: mockUseGroups,
  useCharacterGroupMembers: mockUseMembers,
  useCharacterGroupMutations: () => ({
    create: { mutateAsync: mockCreateGroup, isPending: false },
    update: { mutateAsync: mockUpdateGroup, isPending: false },
    remove: { mutateAsync: mockDeleteGroup, isPending: false },
    memberships: { mutateAsync: mockMemberships, isPending: false },
  }),
}));

beforeEach(() => {
  mockUseGroups.mockReturnValue({ data: [], isLoading: false, isError: false });
  mockUseMembers.mockReturnValue({
    data: [],
    isLoading: false,
    isError: false,
  });
  mockFocus.mockReturnValue(undefined);
  useCharacterGroupStore.setState({
    selectedGroups: {},
    viewTabs: {},
    editingGroups: {},
    deletingGroups: {},
  });
});

vi.mock("@/components/character/useDeleteCharacter", () => ({
  useDeleteCharacter: () => ({
    mutateAsync: mockMutateAsync,
    isPending: false,
  }),
}));

vi.mock("@/components/character/useCreateCharacter", () => ({
  useCreateCharacter: () => ({
    mutateAsync: mockCreateMutateAsync,
    isPending: false,
  }),
}));

vi.mock("@/components/character/useUpdateCharacter", () => ({
  useUpdateCharacter: () => ({
    mutateAsync: mockUpdateMutateAsync,
    isPending: false,
  }),
}));

vi.mock("@/hooks/useFocusWithNonce", () => ({
  useFocusWithNonce: mockFocus,
}));

// Mock CharacterGraph 避免 import @antv/g6（proxy 兼容问题）。
vi.mock("@/components/character/CharacterGraph", () => ({
  default: () => null,
}));

function render(ui: ReactElement) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return originalRender(ui, {
    wrapper: ({ children }) => (
      <QueryClientProvider client={qc}>{children}</QueryClientProvider>
    ),
  });
}

describe("CharacterListView delete", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useCharacterStore.setState({ deletingCharacterId: null });
    mockUseCharacters.mockReturnValue({
      data: [{ id: 1, name: "Alice", description: "", abilities: "[]" }],
      isLoading: false,
      isError: false,
    });
    mockMutateAsync.mockResolvedValue(undefined);
  });

  it("shows confirm dialog when delete clicked", async () => {
    render(<CharacterListView novelId={1} />);
    expect(await screen.findByText("Alice")).toBeInTheDocument();

    fireEvent.click(screen.getByTitle("character.delete"));

    expect(await screen.findByText("common.confirmDelete")).toBeInTheDocument();
  });

  it("calls deleteMutation.mutateAsync with id on confirm", async () => {
    render(<CharacterListView novelId={1} />);
    expect(await screen.findByText("Alice")).toBeInTheDocument();

    fireEvent.click(screen.getByTitle("character.delete"));
    fireEvent.click(await screen.findByText("common.delete"));

    await waitFor(() => {
      expect(mockMutateAsync).toHaveBeenCalledWith(1);
    });
  });

  it("closes dialog after successful delete", async () => {
    render(<CharacterListView novelId={1} />);
    expect(await screen.findByText("Alice")).toBeInTheDocument();

    fireEvent.click(screen.getByTitle("character.delete"));
    fireEvent.click(await screen.findByText("common.delete"));

    await waitFor(() => {
      expect(mockMutateAsync).toHaveBeenCalledWith(1);
    });
    await waitFor(() => {
      expect(
        screen.queryByText("common.confirmDelete"),
      ).not.toBeInTheDocument();
    });
  });

  it("shows toastError with specific message when delete fails", async () => {
    mockMutateAsync.mockRejectedValue(new Error("network error"));
    render(<CharacterListView novelId={1} />);
    expect(await screen.findByText("Alice")).toBeInTheDocument();

    fireEvent.click(screen.getByTitle("character.delete"));
    fireEvent.click(await screen.findByText("common.delete"));

    await waitFor(() => {
      expect(toastError).toHaveBeenCalledWith(
        "character.deleteFailed: network error",
      );
    });
  });
});

// 4.1.2: create mutation 测试。点 newCharacter → 填 name → save → mutateAsync 被调。
describe("CharacterListView create", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useCharacterStore.setState({ deletingCharacterId: null });
    mockUseCharacters.mockReturnValue({
      data: [{ id: 1, name: "Alice", description: "", abilities: "[]" }],
      isLoading: false,
      isError: false,
    });
    mockCreateMutateAsync.mockResolvedValue(undefined);
  });

  it("shows create form when new character clicked", async () => {
    render(<CharacterListView novelId={1} />);
    expect(await screen.findByText("Alice")).toBeInTheDocument();

    fireEvent.click(screen.getByText("character.newCharacter"));

    // 表单出现（name 输入框 placeholder: character.characterName）
    expect(
      screen.getByPlaceholderText("character.characterName"),
    ).toBeInTheDocument();
  });

  it("calls createMutation.mutateAsync with form payload on save", async () => {
    const user = userEvent.setup();
    render(<CharacterListView novelId={1} />);
    expect(await screen.findByText("Alice")).toBeInTheDocument();

    fireEvent.click(screen.getByText("character.newCharacter"));
    const nameInput = screen.getByPlaceholderText("character.characterName");
    await user.type(nameInput, "Bob");
    fireEvent.click(screen.getByText("common.save"));

    await waitFor(() => {
      expect(mockCreateMutateAsync).toHaveBeenCalledWith({
        name: "Bob",
        description: "",
        abilities: "[]",
      });
    });
  });

  it("shows toastError with specific message when create fails", async () => {
    const user = userEvent.setup();
    mockCreateMutateAsync.mockRejectedValue(new Error("network error"));
    render(<CharacterListView novelId={1} />);
    expect(await screen.findByText("Alice")).toBeInTheDocument();

    fireEvent.click(screen.getByText("character.newCharacter"));
    const nameInput = screen.getByPlaceholderText("character.characterName");
    await user.type(nameInput, "Bob");
    fireEvent.click(screen.getByText("common.save"));

    await waitFor(() => {
      expect(toastError).toHaveBeenCalledWith(
        "character.createFailed: network error",
      );
    });
  });
});

// 4.1.2: update mutation 测试。点 edit → 改 name → save → mutateAsync 被调。
describe("CharacterListView update", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useCharacterStore.setState({ deletingCharacterId: null });
    mockUseCharacters.mockReturnValue({
      data: [
        { id: 1, name: "Alice", description: "old desc", abilities: "[]" },
      ],
      isLoading: false,
      isError: false,
    });
    mockUpdateMutateAsync.mockResolvedValue(undefined);
  });

  it("shows edit form with character data when edit clicked", async () => {
    render(<CharacterListView novelId={1} />);
    expect(await screen.findByText("Alice")).toBeInTheDocument();

    fireEvent.click(screen.getByTitle("common.edit"));

    // 编辑表单出现，name input 有值 Alice
    expect(screen.getByDisplayValue("Alice")).toBeInTheDocument();
  });

  it("calls updateMutation.mutateAsync with id and payload on save", async () => {
    const user = userEvent.setup();
    render(<CharacterListView novelId={1} />);
    expect(await screen.findByText("Alice")).toBeInTheDocument();

    fireEvent.click(screen.getByTitle("common.edit"));
    const nameInput = screen.getByDisplayValue("Alice");
    await user.clear(nameInput);
    await user.type(nameInput, "Alice2");
    fireEvent.click(screen.getByText("common.save"));

    await waitFor(() => {
      expect(mockUpdateMutateAsync).toHaveBeenCalledWith({
        id: 1,
        input: { name: "Alice2", description: "old desc", abilities: "[]" },
      });
    });
  });

  it("shows toastError with specific message when update fails", async () => {
    const user = userEvent.setup();
    mockUpdateMutateAsync.mockRejectedValue(new Error("network error"));
    render(<CharacterListView novelId={1} />);
    expect(await screen.findByText("Alice")).toBeInTheDocument();

    fireEvent.click(screen.getByTitle("common.edit"));
    const nameInput = screen.getByDisplayValue("Alice");
    await user.clear(nameInput);
    await user.type(nameInput, "Alice2");
    fireEvent.click(screen.getByText("common.save"));

    await waitFor(() => {
      expect(toastError).toHaveBeenCalledWith(
        "character.updateFailed: network error",
      );
    });
  });
});

describe("CharacterListView groups", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useCharacterStore.setState({ deletingCharacterId: null });
    mockUseCharacters.mockReturnValue({
      data: [
        { id: 1, name: "Alice", description: "", abilities: "[]" },
        { id: 2, name: "Bob", description: "", abilities: "[]" },
        { id: 3, name: "Carol", description: "", abilities: "[]" },
      ],
      isLoading: false,
      isError: false,
    });
    mockUseGroups.mockReturnValue({
      data: [
        {
          id: 10,
          name: "Heroes",
          description: "Traveling companions",
          member_count: 2,
        },
        { id: 20, name: "Guild", description: "Organization", member_count: 2 },
        { id: 30, name: "Observers", description: "", member_count: 2 },
      ],
      isLoading: false,
      isError: false,
    });
    mockUseMembers.mockReturnValue({
      data: [
        { character_id: 1, group_id: 10 },
        { character_id: 1, group_id: 20 },
        { character_id: 1, group_id: 30 },
        { character_id: 2, group_id: 10 },
        { character_id: 2, group_id: 30 },
        { character_id: 3, group_id: 20 },
      ],
      isLoading: false,
      isError: false,
    });
    mockMemberships.mockResolvedValue(undefined);
    mockUpdateGroup.mockResolvedValue(undefined);
    mockDeleteGroup.mockResolvedValue(undefined);
    Object.defineProperty(HTMLElement.prototype, "scrollIntoView", {
      configurable: true,
      value: vi.fn(),
    });
  });

  it("filters the shared character list and shows the group description", () => {
    useCharacterGroupStore.getState().selectGroup(1, 10);
    render(<CharacterListView novelId={1} />);
    expect(screen.getByText("Traveling companions")).toBeInTheDocument();
    expect(screen.getByText("Alice")).toBeInTheDocument();
    expect(screen.getByText("Bob")).toBeInTheDocument();
    expect(screen.queryByText("Carol")).not.toBeInTheDocument();
    act(() => useCharacterGroupStore.getState().selectGroup(1, null));
    expect(screen.getByText("Carol")).toBeInTheDocument();
    expect(screen.getAllByText("Alice")).toHaveLength(1);
    act(() => useCharacterGroupStore.getState().selectGroup(1, 0));
    expect(screen.getByText("characterGroup.empty")).toBeInTheDocument();
  });

  it("creates a group with a description and selects it", async () => {
    const user = userEvent.setup();
    mockCreateGroup.mockResolvedValue({ id: 40 });
    render(<CharacterListView novelId={1} />);
    await user.click(
      screen.getByRole("button", { name: "characterGroup.create" }),
    );
    const form = screen.getByRole("form", { name: "characterGroup.create" });
    await user.type(
      within(form).getByLabelText("characterGroup.name"),
      "Villains",
    );
    await user.type(
      within(form).getByLabelText("characterGroup.description"),
      "Opposing forces",
    );
    await user.click(within(form).getByRole("button", { name: "common.save" }));
    await waitFor(() =>
      expect(mockCreateGroup).toHaveBeenCalledWith({
        name: "Villains",
        description: "Opposing forces",
      }),
    );
    expect(useCharacterGroupStore.getState().selectedGroups[1]).toBe(40);
    expect(screen.queryByRole("form")).not.toBeInTheDocument();
  });

  it("renames a group and allows clearing its description", async () => {
    const user = userEvent.setup();
    useCharacterGroupStore.getState().selectGroup(1, 10);
    render(<CharacterListView novelId={1} />);
    await user.click(screen.getByTitle("characterGroup.edit"));
    const form = screen.getByRole("form", { name: "characterGroup.edit" });
    const name = within(form).getByLabelText("characterGroup.name");
    await user.clear(name);
    await user.type(name, "Companions");
    await user.clear(within(form).getByLabelText("characterGroup.description"));
    await user.click(within(form).getByRole("button", { name: "common.save" }));
    await waitFor(() =>
      expect(mockUpdateGroup).toHaveBeenCalledWith({
        id: 10,
        input: { name: "Companions", description: "" },
      }),
    );
  });

  it("keeps a failed group edit open with its draft and error", async () => {
    const user = userEvent.setup();
    mockUpdateGroup.mockRejectedValue(new Error("duplicate group"));
    useCharacterGroupStore.getState().editGroup(1, 10);
    render(<CharacterListView novelId={1} />);
    const form = screen.getByRole("form", { name: "characterGroup.edit" });
    await user.click(within(form).getByRole("button", { name: "common.save" }));
    expect(await within(form).findByRole("alert")).toHaveTextContent(
      "duplicate group",
    );
    expect(
      within(form).getByLabelText("characterGroup.description"),
    ).toHaveValue("Traveling companions");
  });

  it("deletes the selected group and returns to all characters", async () => {
    useCharacterGroupStore.getState().selectGroup(1, 10);
    render(<CharacterListView novelId={1} />);
    fireEvent.click(screen.getByTitle("characterGroup.delete"));
    fireEvent.click(screen.getByRole("button", { name: "common.confirm" }));
    await waitFor(() => expect(mockDeleteGroup).toHaveBeenCalledWith(10));
    await waitFor(() =>
      expect(useCharacterGroupStore.getState().selectedGroups[1]).toBeNull(),
    );
    expect(screen.getByText("Carol")).toBeInTheDocument();
    expect(mockMutateAsync).not.toHaveBeenCalled();
  });

  it("changes only touched groups for every selected character", async () => {
    const user = userEvent.setup();
    useCharacterGroupStore.getState().selectGroup(1, 10);
    render(<CharacterListView novelId={1} />);
    await user.click(
      screen.getByRole("checkbox", { name: "characterGroup.selectAll" }),
    );
    await user.click(
      screen.getByRole("button", { name: "characterGroup.batchMemberships" }),
    );
    const dialog = screen.getByRole("dialog", {
      name: "characterGroup.adjustMemberships",
    });
    const guild = within(dialog).getByRole("checkbox", { name: /Guild/ });
    expect(guild).toHaveAttribute("aria-checked", "mixed");
    await user.click(guild);
    await user.click(within(dialog).getByRole("checkbox", { name: /Heroes/ }));
    await user.click(
      within(dialog).getByRole("button", { name: "common.save" }),
    );
    await waitFor(() =>
      expect(mockMemberships).toHaveBeenCalledWith({
        character_ids: [1, 2],
        add_group_ids: [20],
        remove_group_ids: [10],
      }),
    );
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("retains membership changes on failure so the user can retry", async () => {
    const user = userEvent.setup();
    mockMemberships.mockRejectedValueOnce(new Error("save failed"));
    render(<CharacterListView novelId={1} />);
    await user.click(
      screen.getAllByTitle("characterGroup.adjustMemberships")[0],
    );
    const dialog = screen.getByRole("dialog");
    await user.click(within(dialog).getByRole("checkbox", { name: /Heroes/ }));
    await user.click(
      within(dialog).getByRole("button", { name: "common.save" }),
    );
    expect(await within(dialog).findByRole("alert")).toHaveTextContent(
      "save failed",
    );
    expect(
      within(dialog).getByRole("checkbox", { name: /Heroes/ }),
    ).not.toBeChecked();
    await user.click(
      within(dialog).getByRole("button", { name: "common.save" }),
    );
    await waitFor(() =>
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
    );
    expect(mockMemberships).toHaveBeenCalledTimes(2);
  });

  it("excludes hidden selected characters after switching the group filter", () => {
    useCharacterGroupStore.getState().selectGroup(1, 10);
    render(<CharacterListView novelId={1} />);
    fireEvent.click(
      screen.getAllByRole("checkbox", {
        name: "characterGroup.selectCharacter",
      })[1],
    );
    act(() => useCharacterGroupStore.getState().selectGroup(1, 20));
    expect(
      screen.getByRole("button", { name: "characterGroup.batchMemberships" }),
    ).toBeDisabled();
  });

  it("reveals a focused character hidden by the filter without overriding later group choices", async () => {
    useCharacterGroupStore.getState().selectGroup(1, 10);
    const view = render(<CharacterListView novelId={1} />);
    mockFocus.mockReturnValue({ id: 3, nonce: 1 });
    view.rerender(<CharacterListView novelId={1} />);
    await waitFor(() => expect(screen.getByText("Carol")).toBeInTheDocument());
    expect(HTMLElement.prototype.scrollIntoView).toHaveBeenCalled();
    act(() => useCharacterGroupStore.getState().selectGroup(1, 10));
    expect(screen.queryByText("Carol")).not.toBeInTheDocument();
    expect(useCharacterGroupStore.getState().selectedGroups[1]).toBe(10);
  });
});

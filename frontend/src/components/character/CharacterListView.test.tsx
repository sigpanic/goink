import { describe, it, expect, vi, beforeEach } from "vitest";
import { render as originalRender, screen, fireEvent, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactElement } from "react";
import CharacterListView from "./CharacterListView";
import { useCharacterStore } from "./useCharacterStore";
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
} = vi.hoisted(() => ({
  mockUseCharacters: vi.fn(),
  mockMutateAsync: vi.fn(),
  mockCreateMutateAsync: vi.fn(),
  mockUpdateMutateAsync: vi.fn(),
}));

vi.mock("@/components/character/useCharacters", () => ({
  useCharacters: mockUseCharacters,
}));

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
  useFocusWithNonce: () => undefined,
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

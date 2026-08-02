import { describe, it, expect, vi, beforeEach } from "vitest";
import { render as originalRender, screen, fireEvent, waitFor } from "@testing-library/react";
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

// 4.1.2: 删除合并测试。CharacterListView 挂唯一 ConfirmDialog + 执行删除（mutateAsync）。
// useCharacters / useDeleteCharacter mock；useCharacterStore 用真实 store（beforeEach reset）。
const { mockUseCharacters, mockMutateAsync } = vi.hoisted(() => ({
  mockUseCharacters: vi.fn(),
  mockMutateAsync: vi.fn(),
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

// useApp 仍用于 create/update（删除测试不触发），mock 空实现避免报错。
vi.mock("@/hooks/useApp", () => ({
  useApp: () => ({
    CreateCharacter: vi.fn(),
    UpdateCharacter: vi.fn(),
  }),
}));

vi.mock("@/stores/useFocusStore", () => ({
  useFocusStore: () => 0,
}));

// Mock CharacterGraph 避免 import @antv/g6（proxy 兼容问题），删除测试不涉及 graph。
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

    // ConfirmDialog 弹出（标题 common.confirmDelete）
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
    // Dialog 关闭：标题 common.confirmDelete 消失
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

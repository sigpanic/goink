import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import CharacterList from "./CharacterList";
import { toastError } from "@/lib/utils";

// Mock toastError
vi.mock("@/lib/utils", async (importOriginal) => {
  const mod = await importOriginal<typeof import("@/lib/utils")>();
  return {
    ...mod,
    toastError: vi.fn(),
  };
});

// Mock useApp — each test can override the mock return value
const mockGetCharacters = vi.fn();
const mockDeleteCharacter = vi.fn();

vi.mock("@/hooks/useApp", () => ({
  useApp: () => ({
    GetCharacters: mockGetCharacters,
    DeleteCharacter: mockDeleteCharacter,
  }),
}));

// Mock confirm
const mockConfirm = vi.fn();
vi.stubGlobal("confirm", mockConfirm);

describe("CharacterList", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetCharacters.mockResolvedValue([]);
    mockConfirm.mockReturnValue(false);
  });

  it("renders empty state when no characters", async () => {
    render(<CharacterList novelId={1} />);
    // Wait for load to complete
    expect(
      await screen.findByText("character.noCharacters"),
    ).toBeInTheDocument();
  });

  it("displays character list", async () => {
    mockGetCharacters.mockResolvedValue([
      { id: 1, name: "Alice" },
      { id: 2, name: "Bob" },
    ]);
    render(<CharacterList novelId={1} />);
    expect(await screen.findByText("Alice")).toBeInTheDocument();
    expect(screen.getByText("Bob")).toBeInTheDocument();
  });

  it("filters characters by search", async () => {
    const user = userEvent.setup();
    mockGetCharacters.mockResolvedValue([
      { id: 1, name: "Alice" },
      { id: 2, name: "Bob" },
    ]);
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
    mockGetCharacters.mockResolvedValue([{ id: 1, name: "Alice" }]);
    mockConfirm.mockReturnValue(true);
    mockDeleteCharacter.mockResolvedValue(undefined);

    render(<CharacterList novelId={1} />);
    expect(await screen.findByText("Alice")).toBeInTheDocument();

    const deleteBtn = screen.getByTitle("character.delete");
    fireEvent.click(deleteBtn);

    expect(mockConfirm).toHaveBeenCalledWith(
      "character.confirmDeleteWithRelation",
    );
    await vi.waitFor(() => {
      expect(mockDeleteCharacter).toHaveBeenCalledWith(1, 1);
    });
  });

  it("shows toastError when delete fails", async () => {
    mockGetCharacters.mockResolvedValue([{ id: 1, name: "Alice" }]);
    mockConfirm.mockReturnValue(true);
    mockDeleteCharacter.mockRejectedValue(new Error("network error"));

    render(<CharacterList novelId={1} />);
    expect(await screen.findByText("Alice")).toBeInTheDocument();

    const deleteBtn = screen.getByTitle("character.delete");
    fireEvent.click(deleteBtn);

    await vi.waitFor(() => {
      expect(toastError).toHaveBeenCalledWith(
        "character.deleteFailed: network error",
      );
    });
  });
});

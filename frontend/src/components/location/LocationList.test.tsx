import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import LocationList from "./LocationList";
import { toastError } from "@/lib/utils";

// Mock toastError
vi.mock("@/lib/utils", async (importOriginal) => {
  const mod = await importOriginal<typeof import("@/lib/utils")>();
  return {
    ...mod,
    toastError: vi.fn(),
  };
});

// Mock useApp
const mockGetLocations = vi.fn();
const mockDeleteLocation = vi.fn();

vi.mock("@/hooks/useApp", () => ({
  useApp: () => ({
    GetLocations: mockGetLocations,
    DeleteLocation: mockDeleteLocation,
  }),
}));

// Mock confirm
const mockConfirm = vi.fn();
vi.stubGlobal("confirm", mockConfirm);

describe("LocationList", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetLocations.mockResolvedValue([]);
    mockConfirm.mockReturnValue(false);
  });

  it("renders empty state when no locations", async () => {
    render(<LocationList novelId={1} />);
    expect(await screen.findByText("location.noLocations")).toBeInTheDocument();
  });

  it("displays location tree", async () => {
    mockGetLocations.mockResolvedValue([
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
    ]);
    render(<LocationList novelId={1} />);
    expect(await screen.findByText("Castle")).toBeInTheDocument();
    expect(await screen.findByText("Throne Room")).toBeInTheDocument();
  });

  it("shows toastError when delete fails", async () => {
    mockGetLocations.mockResolvedValue([
      { id: 1, name: "Castle", parent_location_id: null, location_type: "" },
    ]);
    mockConfirm.mockReturnValue(true);
    mockDeleteLocation.mockRejectedValue(new Error("has children"));

    render(<LocationList novelId={1} />);
    expect(await screen.findByText("Castle")).toBeInTheDocument();

    const deleteBtn = screen.getByTitle("location.delete");
    fireEvent.click(deleteBtn);

    expect(mockConfirm).toHaveBeenCalledWith(
      "location.confirmDeleteWithChildren",
    );
    await vi.waitFor(() => {
      expect(toastError).toHaveBeenCalledWith(
        "location.deleteFailed: has children",
      );
    });
  });

  it("does not call DeleteLocation when confirm is cancelled", async () => {
    mockGetLocations.mockResolvedValue([
      { id: 1, name: "Castle", parent_location_id: null, location_type: "" },
    ]);
    mockConfirm.mockReturnValue(false);

    render(<LocationList novelId={1} />);
    expect(await screen.findByText("Castle")).toBeInTheDocument();

    const deleteBtn = screen.getByTitle("location.delete");
    fireEvent.click(deleteBtn);

    expect(mockConfirm).toHaveBeenCalled();
    expect(mockDeleteLocation).not.toHaveBeenCalled();
  });
});

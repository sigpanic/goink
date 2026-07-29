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

describe("LocationList", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetLocations.mockResolvedValue([]);
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
    mockGetLocations.mockResolvedValue([
      { id: 1, name: "Castle", parent_location_id: null, location_type: "" },
    ]);

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

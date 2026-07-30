import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import StyleView from "./StyleView";
import { toastError } from "@/utils/toast";

// Mock toastError
vi.mock("@/utils/toast", async (importOriginal) => {
  const mod = await importOriginal<typeof import("@/utils/toast")>();
  return {
    ...mod,
    toastError: vi.fn(),
  };
});

// Mock child components
vi.mock("./StyleSampleCard", () => ({
  default: ({ sample, selected, onToggle, onDelete }: any) => (
    <div data-testid={`card-${sample.id}`}>
      <span>{sample.name}</span>
      <button onClick={onToggle}>{selected ? "deselect" : "select"}</button>
      <button onClick={onDelete}>delete</button>
    </div>
  ),
}));

vi.mock("@/components/Markdown", () => ({
  default: ({ content }: any) => <div>{content}</div>,
}));

vi.mock("@/components/chat/PopSelect", () => ({
  default: ({ value, options, onChange }: any) => (
    <select
      value={value}
      onChange={(e) => onChange(e.target.value)}
      data-testid="pop-select"
    >
      {options?.map((o: any) => (
        <option key={o.value} value={o.value}>
          {o.label}
        </option>
      ))}
    </select>
  ),
}));

vi.mock("@/components/shared/TagInput", () => ({
  default: () => null,
}));

// Mock useApp
const mockListStyleSamples = vi.fn();
const mockGetStyleSample = vi.fn();
const mockDeleteStyleSample = vi.fn();
const mockCreateStyleSample = vi.fn();
const mockGetNovels = vi.fn();
const mockGetModels = vi.fn();
const mockGetSettings = vi.fn();

vi.mock("@/hooks/useApp", () => ({
  useApp: () => ({
    ListStyleSamples: mockListStyleSamples,
    GetStyleSample: mockGetStyleSample,
    DeleteStyleSample: mockDeleteStyleSample,
    CreateStyleSample: mockCreateStyleSample,
    GetNovels: mockGetNovels,
    GetModels: mockGetModels,
    GetSettings: mockGetSettings,
  }),
}));

describe("StyleView", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockListStyleSamples.mockResolvedValue({
      items: [],
      total: 0,
      total_pages: 0,
    });
    mockGetNovels.mockResolvedValue([]);
    mockGetModels.mockResolvedValue([]);
    mockGetSettings.mockResolvedValue({ selected_model_key: "" });
  });

  it("renders empty state when no samples", async () => {
    render(<StyleView />);
    expect(
      await screen.findByText("styleSample.noStyleSamples"),
    ).toBeInTheDocument();
  });

  it("displays sample cards", async () => {
    mockListStyleSamples.mockResolvedValue({
      items: [
        {
          id: 1,
          name: "Suspense",
          content: "...",
          tags: [],
          is_global: true,
          novel_id: 0,
        },
        {
          id: 2,
          name: "Dialogue",
          content: "...",
          tags: [],
          is_global: true,
          novel_id: 0,
        },
      ],
      total: 2,
      total_pages: 1,
    });
    render(<StyleView />);
    expect(await screen.findByText("Suspense")).toBeInTheDocument();
    expect(screen.getByText("Dialogue")).toBeInTheDocument();
  });

  it("shows toastError when load fails", async () => {
    mockListStyleSamples.mockRejectedValue(new Error("network timeout"));
    render(<StyleView />);
    await vi.waitFor(() => {
      expect(toastError).toHaveBeenCalledWith(
        "styleSample.loadFailed: network timeout",
      );
    });
  });

  it("shows toastError when delete fails", async () => {
    mockListStyleSamples.mockResolvedValue({
      items: [
        {
          id: 1,
          name: "Suspense",
          content: "...",
          tags: [],
          is_global: true,
          novel_id: 0,
        },
      ],
      total: 1,
      total_pages: 1,
    });
    mockDeleteStyleSample.mockRejectedValue(new Error("db error"));

    render(<StyleView />);
    expect(await screen.findByText("Suspense")).toBeInTheDocument();

    const deleteBtn = screen.getByText("delete");
    fireEvent.click(deleteBtn);

    // 删除按钮现在弹出 ConfirmDialog，需点确认才执行删除
    const confirmBtn = await screen.findByText("common.delete");
    fireEvent.click(confirmBtn);

    await vi.waitFor(() => {
      expect(toastError).toHaveBeenCalledWith(
        "styleSample.deleteFailed: db error",
      );
    });
  });

  it("switches to adding phase when add button clicked", async () => {
    const user = userEvent.setup();
    render(<StyleView />);
    expect(
      await screen.findByText("styleSample.noStyleSamples"),
    ).toBeInTheDocument();

    const addBtn = screen.getByText("styleSample.addSample");
    await user.click(addBtn);

    // Should show the add form with name input
    expect(
      screen.getByPlaceholderText("styleSample.sampleNamePlaceholder"),
    ).toBeInTheDocument();
  });

  it("shows toastError when openDetail fails", async () => {
    mockListStyleSamples.mockResolvedValue({
      items: [
        {
          id: 1,
          name: "Suspense",
          content: "...",
          tags: [],
          is_global: true,
          novel_id: 0,
        },
      ],
      total: 1,
      total_pages: 1,
    });
    mockGetStyleSample.mockRejectedValue(new Error("not found"));

    render(<StyleView />);
    expect(await screen.findByText("Suspense")).toBeInTheDocument();

    // StyleSampleCard's onClick triggers openDetail
    // Since our mock card doesn't have onClick prop wired to openDetail,
    // test openDetail via focusId prop
    render(<StyleView focusId={1} onFocusHandled={vi.fn()} />);
    await vi.waitFor(() => {
      expect(toastError).toHaveBeenCalledWith(
        "styleSample.loadFailed: not found",
      );
    });
  });
});

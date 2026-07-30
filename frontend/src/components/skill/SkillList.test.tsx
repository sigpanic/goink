import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import SkillList from "./SkillList";
import { toastError } from "@/utils/toast";

// Mock toastError
vi.mock("@/utils/toast", async (importOriginal) => {
  const mod = await importOriginal<typeof import("@/utils/toast")>();
  return {
    ...mod,
    toastError: vi.fn(),
  };
});

// Mock SkillContributeDialog
vi.mock("./SkillContributeDialog", () => ({
  default: () => null,
}));

// Mock useApp
const mockListSkills = vi.fn();
const mockDeleteSkill = vi.fn();

vi.mock("@/hooks/useApp", () => ({
  useApp: () => ({
    ListSkills: mockListSkills,
    DeleteSkill: mockDeleteSkill,
  }),
}));

describe("SkillList", () => {
  const defaultProps = {
    novelId: 1,
    activeSkillName: null as string | null,
    onSelectSkill: vi.fn(),
    onEditSkill: vi.fn(),
    onNewSkill: vi.fn(),
  };

  beforeEach(() => {
    vi.clearAllMocks();
    mockListSkills.mockResolvedValue([]);
  });

  it("renders empty state when no skills", async () => {
    render(<SkillList {...defaultProps} />);
    expect(await screen.findByText("skill.noSkills")).toBeInTheDocument();
  });

  it("displays skills grouped by source", async () => {
    mockListSkills.mockResolvedValue([
      { name: "Writer", source: "novel", description: "Write chapters" },
      { name: "Editor", source: "user", description: "Edit content" },
      { name: "Helper", source: "builtin", description: "Built-in help" },
    ]);
    render(<SkillList {...defaultProps} />);
    expect(await screen.findByText("Writer")).toBeInTheDocument();
    expect(screen.getByText("Editor")).toBeInTheDocument();
    expect(screen.getByText("Helper")).toBeInTheDocument();
    // Group headers
    expect(screen.getByText("skill.currentNovel")).toBeInTheDocument();
    expect(screen.getByText("skill.userLevel")).toBeInTheDocument();
    expect(screen.getByText("skill.builtin")).toBeInTheDocument();
  });

  it("calls DeleteSkill on confirm and reloads", async () => {
    mockListSkills.mockResolvedValue([
      { name: "Writer", source: "novel", description: "" },
    ]);
    mockDeleteSkill.mockResolvedValue(undefined);

    render(<SkillList {...defaultProps} />);
    expect(await screen.findByText("Writer")).toBeInTheDocument();

    const deleteBtn = screen.getByTitle("skill.deleteSkill");
    fireEvent.click(deleteBtn);

    // 删除按钮现在弹出 ConfirmDialog，需点确认才执行删除
    const confirmBtn = await screen.findByText("common.delete");
    fireEvent.click(confirmBtn);

    await vi.waitFor(() => {
      expect(mockDeleteSkill).toHaveBeenCalledWith({
        novel_id: 1,
        name: "Writer",
        source: "novel",
      });
    });
  });

  it("shows toastError when delete fails", async () => {
    mockListSkills.mockResolvedValue([
      { name: "Writer", source: "novel", description: "" },
    ]);
    mockDeleteSkill.mockRejectedValue(new Error("permission denied"));

    render(<SkillList {...defaultProps} />);
    expect(await screen.findByText("Writer")).toBeInTheDocument();

    const deleteBtn = screen.getByTitle("skill.deleteSkill");
    fireEvent.click(deleteBtn);

    const confirmBtn = await screen.findByText("common.delete");
    fireEvent.click(confirmBtn);

    await vi.waitFor(() => {
      expect(toastError).toHaveBeenCalledWith(
        "skill.deleteFailed: permission denied",
      );
    });
  });

  it("filters skills by search", async () => {
    const user = userEvent.setup();
    mockListSkills.mockResolvedValue([
      { name: "Writer", source: "novel", description: "Write chapters" },
      { name: "Editor", source: "user", description: "Edit content" },
    ]);
    render(<SkillList {...defaultProps} />);
    expect(await screen.findByText("Writer")).toBeInTheDocument();

    const searchInput = screen.getByPlaceholderText("skill.search");
    await user.type(searchInput, "edit");

    expect(screen.queryByText("Writer")).not.toBeInTheDocument();
    expect(screen.getByText("Editor")).toBeInTheDocument();
  });
});

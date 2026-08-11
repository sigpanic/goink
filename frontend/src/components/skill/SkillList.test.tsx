import { describe, it, expect, vi, beforeEach } from "vitest";
import {
  render as originalRender,
  screen,
  fireEvent,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactElement } from "react";
import SkillList from "./SkillList";
import { toastError } from "@/utils/toast";

// 5.4 commit 1: useSkills 引入 useQuery，render 需包 QueryClientProvider。
// 每个测试用独立 QueryClient（retry:false 避免重试），无状态残留。
function render(ui: ReactElement) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return originalRender(ui, {
    wrapper: ({ children }) => (
      <QueryClientProvider client={qc}>{children}</QueryClientProvider>
    ),
  });
}

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

// 5.4 commit 1: skills 数据走 useSkills query（不再走 useApp.ListSkills）。
// mockUseSkills 用 vi.hoisted 提升，让 vi.mock 工厂能引用（vi.mock 自身被提升到文件顶部）。
const { mockUseSkills } = vi.hoisted(() => ({
  mockUseSkills: vi.fn(),
}));

vi.mock("./useSkills", () => ({
  useSkills: mockUseSkills,
}));

// DeleteSkill 仍走 useApp（commit 2 迁 useDeleteSkill mutation），mock 保留。
const mockDeleteSkill = vi.fn();

vi.mock("@/hooks/useApp", () => ({
  useApp: () => ({
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
    // 默认返回空数组（避免 undefined 报错）
    mockUseSkills.mockReturnValue({
      data: [],
      isLoading: false,
      isError: false,
    });
  });

  it("renders empty state when no skills", async () => {
    render(<SkillList {...defaultProps} />);
    expect(await screen.findByText("skill.noSkills")).toBeInTheDocument();
  });

  it("shows loadFailed when isError", async () => {
    mockUseSkills.mockReturnValue({
      data: [],
      isLoading: false,
      isError: true,
    });
    render(<SkillList {...defaultProps} />);
    expect(await screen.findByText("skill.loadFailed")).toBeInTheDocument();
  });

  it("displays skills grouped by source", async () => {
    mockUseSkills.mockReturnValue({
      data: [
        { name: "Writer", source: "novel", description: "Write chapters" },
        { name: "Editor", source: "user", description: "Edit content" },
        { name: "Helper", source: "builtin", description: "Built-in help" },
      ],
      isLoading: false,
      isError: false,
    });
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
    mockUseSkills.mockReturnValue({
      data: [{ name: "Writer", source: "novel", description: "" }],
      isLoading: false,
      isError: false,
    });
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
    mockUseSkills.mockReturnValue({
      data: [{ name: "Writer", source: "novel", description: "" }],
      isLoading: false,
      isError: false,
    });
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
    mockUseSkills.mockReturnValue({
      data: [
        { name: "Writer", source: "novel", description: "Write chapters" },
        { name: "Editor", source: "user", description: "Edit content" },
      ],
      isLoading: false,
      isError: false,
    });
    render(<SkillList {...defaultProps} />);
    expect(await screen.findByText("Writer")).toBeInTheDocument();

    const searchInput = screen.getByPlaceholderText("skill.search");
    await user.type(searchInput, "edit");

    expect(screen.queryByText("Writer")).not.toBeInTheDocument();
    expect(screen.getByText("Editor")).toBeInTheDocument();
  });
});

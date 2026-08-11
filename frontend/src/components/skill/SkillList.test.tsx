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

// 5.4 commit 2: mock SkillMarketplace 隔离测试（其内部仍用 useApp，commit 3 才迁）。
vi.mock("./SkillMarketplace", () => ({
  default: () => null,
}));

// 5.4 commit 1: skills 数据走 useSkills query（不再走 useApp.ListSkills）。
// mockUseSkills 用 vi.hoisted 提升，让 vi.mock 工厂能引用（vi.mock 自身被提升到文件顶部）。
// 5.4 commit 2: DeleteSkill 走 useDeleteSkill mutation（不再走 useApp）。
const { mockUseSkills, mockUseDeleteSkill } = vi.hoisted(() => ({
  mockUseSkills: vi.fn(),
  mockUseDeleteSkill: vi.fn(),
}));

vi.mock("./useSkills", () => ({
  useSkills: mockUseSkills,
}));

vi.mock("./useDeleteSkill", () => ({
  useDeleteSkill: mockUseDeleteSkill,
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
    // 默认 delete mutation 返回 resolved mutateAsync + isPending false
    mockUseDeleteSkill.mockReturnValue({
      mutateAsync: vi.fn().mockResolvedValue(undefined),
      isPending: false,
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
    const mockMutateAsync = vi.fn().mockResolvedValue(undefined);
    mockUseDeleteSkill.mockReturnValue({
      mutateAsync: mockMutateAsync,
      isPending: false,
    });
    mockUseSkills.mockReturnValue({
      data: [{ name: "Writer", source: "novel", description: "" }],
      isLoading: false,
      isError: false,
    });

    render(<SkillList {...defaultProps} />);
    expect(await screen.findByText("Writer")).toBeInTheDocument();

    const deleteBtn = screen.getByTitle("skill.deleteSkill");
    fireEvent.click(deleteBtn);

    // 删除按钮现在弹出 ConfirmDialog，需点确认才执行删除
    const confirmBtn = await screen.findByText("common.delete");
    fireEvent.click(confirmBtn);

    await vi.waitFor(() => {
      // 5.4 commit 2: mutateAsync 入参 {name, source}，novel_id 在 hook 内部拼
      expect(mockMutateAsync).toHaveBeenCalledWith({
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
    mockUseDeleteSkill.mockReturnValue({
      mutateAsync: vi.fn().mockRejectedValue(new Error("permission denied")),
      isPending: false,
    });

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

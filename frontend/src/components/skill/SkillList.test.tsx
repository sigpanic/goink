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
import { contentKeys } from "@/lib/queryKeys";
import { getEditorSaveQueue } from "@/components/content/editorSaveQueue";
import { useEditorTabsStore } from "@/components/content/useEditorTabsStore";

// 5.4 commit 1: useSkills 引入 useQuery，render 需包 QueryClientProvider。
// 每个测试用独立 QueryClient（retry:false 避免重试），无状态残留。
function render(ui: ReactElement) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return {
    ...originalRender(ui, {
      wrapper: ({ children }) => (
        <QueryClientProvider client={qc}>{children}</QueryClientProvider>
      ),
    }),
    qc,
  };
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
    useEditorTabsStore.setState({ byNovel: {}, positions: {} });
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
      // useDeleteSkill 模式 B：调用方传完整后端结构体 app.DeleteSkillInput（含 novel_id）
      expect(mockMutateAsync).toHaveBeenCalledWith({
        novel_id: 1,
        name: "Writer",
        source: "novel",
      });
    });
  });

  it("closes only the deleted novel skill tab and removes its content cache", async () => {
    const path = "skills/Writer.md";
    const currentId = useEditorTabsStore.getState().openTab(1, {
      type: "file",
      path,
      title: "Writer",
    });
    const otherId = useEditorTabsStore.getState().openTab(2, {
      type: "file",
      path,
      title: "Writer",
    });
    mockUseSkills.mockReturnValue({
      data: [{ name: "Writer", source: "novel", description: "" }],
      isLoading: false,
      isError: false,
    });
    const { qc } = render(<SkillList {...defaultProps} />);
    qc.setQueryData(contentKeys.detail(1, path), "current");
    qc.setQueryData(contentKeys.detail(2, path), "other");

    fireEvent.click(screen.getByTitle("skill.deleteSkill"));
    fireEvent.click(await screen.findByText("common.delete"));

    await vi.waitFor(() => {
      expect(
        useEditorTabsStore
          .getState()
          .byNovel["1"]?.tabs.some((tab) => tab.id === currentId),
      ).toBe(false);
    });
    expect(
      useEditorTabsStore
        .getState()
        .byNovel["2"]?.tabs.some((tab) => tab.id === otherId),
    ).toBe(true);
    expect(qc.getQueryData(contentKeys.detail(1, path))).toBeUndefined();
    expect(qc.getQueryData(contentKeys.detail(2, path))).toBe("other");
  });

  it("closes a deleted user skill tab in every novel", async () => {
    const path = "~/.goink/skills/Writer.md";
    useEditorTabsStore.getState().openTab(1, {
      type: "file",
      path,
      title: "Writer",
    });
    useEditorTabsStore.getState().openTab(2, {
      type: "file",
      path,
      title: "Writer",
    });
    useEditorTabsStore.getState().openTab(1, {
      type: "file",
      path: "skills/Writer.md",
      title: "Novel Writer",
    });
    mockUseSkills.mockReturnValue({
      data: [{ name: "Writer", source: "user", description: "" }],
      isLoading: false,
      isError: false,
    });
    const { qc } = render(<SkillList {...defaultProps} />);
    qc.setQueryData(contentKeys.detail(1, path), "current");
    qc.setQueryData(contentKeys.detail(2, path), "other");

    fireEvent.click(screen.getByTitle("skill.deleteSkill"));
    fireEvent.click(await screen.findByText("common.delete"));

    await vi.waitFor(() => {
      expect(
        useEditorTabsStore
          .getState()
          .byNovel["1"]?.tabs.some((tab) => tab.path === path),
      ).toBe(false);
    });
    expect(
      useEditorTabsStore
        .getState()
        .byNovel["2"]?.tabs.some((tab) => tab.path === path),
    ).toBe(false);
    expect(
      useEditorTabsStore
        .getState()
        .byNovel["1"]?.tabs.some((tab) => tab.path === "skills/Writer.md"),
    ).toBe(true);
    expect(qc.getQueryData(contentKeys.detail(1, path))).toBeUndefined();
    expect(qc.getQueryData(contentKeys.detail(2, path))).toBeUndefined();
  });

  it("shows toastError when delete fails", async () => {
    const path = "skills/Writer.md";
    const tabId = useEditorTabsStore.getState().openTab(1, {
      type: "file",
      path,
      title: "Writer",
    });
    mockUseSkills.mockReturnValue({
      data: [{ name: "Writer", source: "novel", description: "" }],
      isLoading: false,
      isError: false,
    });
    mockUseDeleteSkill.mockReturnValue({
      mutateAsync: vi.fn().mockRejectedValue(new Error("permission denied")),
      isPending: false,
    });

    const { qc } = render(<SkillList {...defaultProps} />);
    qc.setQueryData(contentKeys.detail(1, path), "cached");
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
    expect(
      useEditorTabsStore
        .getState()
        .byNovel["1"]?.tabs.some((tab) => tab.id === tabId),
    ).toBe(true);
    expect(qc.getQueryData(contentKeys.detail(1, path))).toBe("cached");
  });

  it("keeps the skill and tab when edits are unsaved", async () => {
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
    useEditorTabsStore.getState().openTab(1, {
      type: "file",
      path: "skills/Writer.md",
      title: "Writer",
      isDirty: true,
    });
    render(<SkillList {...defaultProps} />);

    fireEvent.click(screen.getByTitle("skill.deleteSkill"));
    fireEvent.click(await screen.findByText("common.delete"));

    expect(mockMutateAsync).not.toHaveBeenCalled();
    expect(toastError).toHaveBeenCalledWith("content.saveSkillBeforeClose");
    expect(useEditorTabsStore.getState().byNovel["1"]?.tabs).toHaveLength(1);
  });

  it("keeps the skill and tab while a save is pending", async () => {
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
    const path = "skills/Writer.md";
    const tabId = useEditorTabsStore.getState().openTab(1, {
      type: "file",
      path,
      title: "Writer",
    });
    const { qc } = render(<SkillList {...defaultProps} />);
    const queue = getEditorSaveQueue(qc);
    queue.schedule(
      {
        novelId: 1,
        tabId,
        path,
        content: "new draft",
        dirtyKey: "isDirty",
        expectedContent: "old draft",
      },
      vi.fn().mockResolvedValue(undefined),
      vi.fn(),
      vi.fn(),
      vi.fn(),
    );
    queue.pause(1, path);

    fireEvent.click(screen.getByTitle("skill.deleteSkill"));
    fireEvent.click(await screen.findByText("common.delete"));

    expect(mockMutateAsync).not.toHaveBeenCalled();
    expect(toastError).toHaveBeenCalledWith("skill.saving");
    expect(useEditorTabsStore.getState().byNovel["1"]?.tabs).toHaveLength(1);
    queue.discard(1, path);
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

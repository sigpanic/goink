import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { adapter } from "@/lib/wailsjs/go/models";
import PatternSessionView from "./PatternSessionView";

const { mockExtractPattern, mockGetContent, mockSaveContent } = vi.hoisted(
  () => ({
    mockExtractPattern: vi.fn(),
    mockGetContent: vi.fn(),
    mockSaveContent: vi.fn(),
  }),
);

vi.mock("@/lib/wailsjs/go/app/App", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/wailsjs/go/app/App")>()),
  ExtractPattern: mockExtractPattern,
  GetContent: mockGetContent,
  SaveContent: mockSaveContent,
}));

vi.mock("@/hooks/usePatternProgress", () => ({
  usePatternProgress: () => ({ progress: null, events: [], reset: vi.fn() }),
}));

vi.mock("./PatternProgressView", () => ({ default: () => null }));
vi.mock("@/components/Markdown", () => ({ default: () => null }));

const modelSelection: adapter.ModelSelection = {
  adapter_name: "anthropic_messages",
  provider_name: "Relay",
  model_id: "model",
  reasoning_effort: "high",
};

function renderSession(
  onExit = vi.fn(),
  chapterIds: number[] = [],
  selection = modelSelection,
) {
  const queryClient = new QueryClient();
  render(
    <QueryClientProvider client={queryClient}>
      <PatternSessionView
        taskId="task-1"
        novelId={3}
        chapterIds={chapterIds}
        modelSelection={selection}
        title="Novel"
        chapterCount={2}
        onExit={onExit}
      />
    </QueryClientProvider>,
  );
  return onExit;
}

describe("PatternSessionView generated skill save", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockExtractPattern.mockResolvedValue({
      task_id: "task-1",
      name: "Generated",
      description: "desc",
      file_path: "~/.goink/skills/generated.md",
      raw_content: "new skill",
    });
    mockSaveContent.mockResolvedValue(undefined);
  });

  it("saves immediately with an empty expected content when the target is empty", async () => {
    mockGetContent.mockResolvedValue("");
    const onExit = renderSession();
    fireEvent.click(await screen.findByText("extract.saveToUserSkill"));

    expect(mockExtractPattern).toHaveBeenCalledExactlyOnceWith({
      task_id: "task-1",
      novel_id: 3,
      ...modelSelection,
      chapter_ids: undefined,
    });

    await vi.waitFor(() => {
      expect(mockSaveContent).toHaveBeenCalledWith({
        novel_id: 3,
        path: "~/.goink/skills/generated.md",
        content: "new skill",
        expected_content: "",
      });
      expect(onExit).toHaveBeenCalledOnce();
    });
    expect(
      screen.queryByText("skill.generatedOverwriteTitle"),
    ).not.toBeInTheDocument();
  });

  it("keeps the generated result after a confirmed overwrite conflicts", async () => {
    mockGetContent.mockResolvedValue("original skill");
    mockSaveContent.mockRejectedValueOnce(
      new Error("CONTENT_CONFLICT: changed"),
    );
    const onExit = renderSession();
    fireEvent.click(await screen.findByText("extract.saveToUserSkill"));

    expect(
      await screen.findByText("skill.generatedOverwriteTitle"),
    ).toBeInTheDocument();
    expect(mockSaveContent).not.toHaveBeenCalled();
    fireEvent.click(screen.getByText("skill.generatedConfirmOverwrite"));

    expect(
      await screen.findByText("skill.generatedTargetChanged"),
    ).toBeInTheDocument();
    expect(mockSaveContent).toHaveBeenCalledWith({
      novel_id: 3,
      path: "~/.goink/skills/generated.md",
      content: "new skill",
      expected_content: "original skill",
    });
    expect(screen.getByText("extract.saveToUserSkill")).toBeInTheDocument();
    expect(onExit).not.toHaveBeenCalled();

    mockGetContent.mockResolvedValue("newer skill");
    fireEvent.click(screen.getByText("extract.saveToUserSkill"));
    fireEvent.click(await screen.findByText("skill.generatedConfirmOverwrite"));
    await vi.waitFor(() => {
      expect(mockSaveContent).toHaveBeenLastCalledWith({
        novel_id: 3,
        path: "~/.goink/skills/generated.md",
        content: "new skill",
        expected_content: "newer skill",
      });
      expect(onExit).toHaveBeenCalledOnce();
    });
  });

  it("does not write when reading the target fails", async () => {
    mockGetContent.mockRejectedValue(new Error("read failed"));
    const onExit = renderSession();
    fireEvent.click(await screen.findByText("extract.saveToUserSkill"));

    expect(await screen.findByText("read failed")).toBeInTheDocument();
    expect(mockSaveContent).not.toHaveBeenCalled();
    expect(screen.getByText("extract.saveToUserSkill")).toBeInTheDocument();
    expect(onExit).not.toHaveBeenCalled();
  });

  it("preserves selected chapters and an empty effort in the protocol-aware request", async () => {
    const selection = {
      ...modelSelection,
      adapter_name: "gemini_interactions",
      reasoning_effort: "",
    };
    renderSession(vi.fn(), [2, 5], selection);
    await screen.findByText("extract.saveToUserSkill");
    expect(mockExtractPattern).toHaveBeenCalledExactlyOnceWith({
      task_id: "task-1",
      novel_id: 3,
      ...selection,
      chapter_ids: [2, 5],
    });
  });

  it("rejects a selection without a protocol before starting extraction", async () => {
    renderSession(vi.fn(), [], { ...modelSelection, adapter_name: "" });
    await screen.findAllByText("extract.extractFailed");
    expect(mockExtractPattern).not.toHaveBeenCalled();
  });
});

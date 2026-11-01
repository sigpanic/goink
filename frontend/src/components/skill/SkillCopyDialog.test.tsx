import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { skill } from "@/lib/wailsjs/go/models";
import SkillCopyDialog from "./SkillCopyDialog";
import { useEditorTabsStore } from "@/components/content/useEditorTabsStore";

const { mockUseNovels, mockUseSkills, mockFetchContent } = vi.hoisted(() => ({
  mockUseNovels: vi.fn(),
  mockUseSkills: vi.fn(),
  mockFetchContent: vi.fn(),
}));
vi.mock("@/components/novel/useNovels", () => ({ useNovels: mockUseNovels }));
vi.mock("./useSkills", () => ({ useSkills: mockUseSkills }));
vi.mock("@/components/content/useFileContent", () => ({
  useFileContent: () => ({ fetchContent: mockFetchContent }),
}));

const novelSkill = (name: string, source = "novel") =>
  ({ name, source, description: `${name} description` }) as skill.SkillMeta;

function setup(currentSkills: skill.SkillMeta[] = []) {
  const onNewSkill = vi.fn();
  const onClose = vi.fn();
  render(
    <SkillCopyDialog
      novelId={1}
      currentSkills={currentSkills}
      onNewSkill={onNewSkill}
      onClose={onClose}
    />,
  );
  return { onNewSkill, onClose };
}

function selectSource(id = "2") {
  if (!screen.queryByRole("listbox", { name: "skill.copy.source" }))
    fireEvent.click(screen.getByRole("button", { name: "skill.copy.source" }));
  fireEvent.click(
    screen.getByRole("option", { name: id === "2" ? "源小说" : "另一部小说" }),
  );
}

describe("SkillCopyDialog", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useEditorTabsStore.setState({ byNovel: {}, positions: {} });
    mockUseNovels.mockReturnValue({
      data: [
        { id: 1, title: "当前小说" },
        { id: 2, title: "源小说" },
        { id: 3, title: "另一部小说" },
      ],
      isLoading: false,
      isError: false,
    });
    mockUseSkills.mockImplementation((id: number) => ({
      data: id
        ? [
            novelSkill("plot"),
            novelSkill("style"),
            novelSkill("global", "user"),
            novelSkill("builtin", "builtin"),
          ]
        : [],
      isLoading: false,
      isError: false,
    }));
    mockFetchContent.mockImplementation((_id: number, path: string) =>
      Promise.resolve(`raw ${path}`),
    );
  });

  it("excludes the target novel and global skills, and clears selection when switching source", () => {
    setup();
    expect(
      screen.getByRole("button", { name: "skill.copy.source" }),
    ).toHaveTextContent("skill.copy.selectSource");
    fireEvent.click(screen.getByRole("button", { name: "skill.copy.source" }));
    expect(screen.getAllByRole("option")).toHaveLength(2);
    expect(
      screen.queryByRole("option", { name: "当前小说" }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "skill.copy.openDrafts" }),
    ).toBeDisabled();
    selectSource();
    expect(
      screen.getByRole("button", { name: "skill.copy.source" }),
    ).toHaveTextContent("源小说");
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    expect(screen.getAllByRole("checkbox")).toHaveLength(2);
    expect(screen.queryByText("global")).not.toBeInTheDocument();
    expect(screen.queryByText("builtin")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("checkbox", { name: /plot/ }));
    selectSource("3");
    expect(screen.getByRole("checkbox", { name: /plot/ })).not.toBeChecked();
    expect(
      screen.getByRole("button", { name: "skill.copy.openDrafts" }),
    ).toBeDisabled();
  });

  it("opens selected skills as independent drafts through the existing new-skill callback", async () => {
    const { onNewSkill, onClose } = setup([novelSkill("plot", "user")]);
    selectSource();
    fireEvent.click(screen.getByRole("checkbox", { name: /plot/ }));
    fireEvent.click(screen.getByRole("checkbox", { name: /style/ }));
    fireEvent.click(
      screen.getByRole("button", { name: "skill.copy.openDrafts" }),
    );
    await waitFor(() => expect(onClose).toHaveBeenCalledOnce());
    expect(mockFetchContent).toHaveBeenCalledWith(2, "skills/plot.md");
    expect(mockFetchContent).toHaveBeenCalledWith(2, "skills/style.md");
    expect(onNewSkill.mock.calls).toEqual([
      ["plot", "raw skills/plot.md"],
      ["style", "raw skills/style.md"],
    ]);
    expect(
      screen.queryByText("skill.copy.overwriteWarning"),
    ).not.toBeInTheDocument();
  });

  it("skips same-name skills by default and requires confirmation when selected", async () => {
    const { onNewSkill } = setup([novelSkill("plot")]);
    selectSource();
    expect(screen.getByRole("checkbox", { name: /plot/ })).not.toBeChecked();
    fireEvent.click(screen.getByRole("checkbox", { name: /style/ }));
    fireEvent.click(
      screen.getByRole("button", { name: "skill.copy.openDrafts" }),
    );
    await waitFor(() => expect(onNewSkill).toHaveBeenCalledOnce());
    expect(onNewSkill).toHaveBeenCalledWith("style", "raw skills/style.md");
    expect(mockFetchContent).not.toHaveBeenCalledWith(2, "skills/plot.md");
  });

  it("does not read or replace an existing skill until overwrite is confirmed", async () => {
    const { onNewSkill, onClose } = setup([novelSkill("plot")]);
    selectSource();
    fireEvent.click(screen.getByRole("checkbox", { name: /plot/ }));
    fireEvent.click(
      screen.getByRole("button", { name: "skill.copy.openDrafts" }),
    );
    expect(screen.getByText("skill.copy.overwriteWarning")).toBeInTheDocument();
    expect(mockFetchContent).not.toHaveBeenCalled();
    expect(onNewSkill).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "common.back" }));
    expect(screen.getByRole("checkbox", { name: /plot/ })).toBeChecked();
    fireEvent.click(
      screen.getByRole("button", { name: "skill.copy.openDrafts" }),
    );
    fireEvent.click(
      screen.getByRole("button", { name: "skill.copy.confirmOverwrite" }),
    );
    await waitFor(() => expect(onClose).toHaveBeenCalledOnce());
    expect(onNewSkill).toHaveBeenCalledWith("plot", "raw skills/plot.md");
  });

  it("also asks for confirmation before replacing a same-name unsaved new skill", () => {
    useEditorTabsStore.getState().openTab(1, {
      type: "file",
      path: "skills/plot.md",
      title: "plot",
      content: "unsaved",
      isDirty: true,
    });
    setup();
    selectSource();
    fireEvent.click(screen.getByRole("checkbox", { name: /plot/ }));
    fireEvent.click(
      screen.getByRole("button", { name: "skill.copy.openDrafts" }),
    );
    expect(screen.getByText("skill.copy.overwriteWarning")).toBeInTheDocument();
    expect(mockFetchContent).not.toHaveBeenCalled();
  });

  it("preserves selection and opens no drafts if reading fails, allowing a retry", async () => {
    mockFetchContent.mockRejectedValueOnce(new Error("read failed"));
    const { onNewSkill, onClose } = setup();
    selectSource();
    fireEvent.click(screen.getByRole("checkbox", { name: /plot/ }));
    fireEvent.click(
      screen.getByRole("button", { name: "skill.copy.openDrafts" }),
    );
    expect(await screen.findByRole("alert")).toHaveTextContent("read failed");
    expect(onNewSkill).not.toHaveBeenCalled();
    expect(onClose).not.toHaveBeenCalled();
    expect(screen.getByRole("checkbox", { name: /plot/ })).toBeChecked();
    fireEvent.click(
      screen.getByRole("button", { name: "skill.copy.openDrafts" }),
    );
    await waitFor(() => expect(onClose).toHaveBeenCalledOnce());
  });

  it("shows empty and error states", () => {
    mockUseSkills.mockReturnValue({
      data: [],
      isLoading: false,
      isError: false,
    });
    const { onClose } = setup();
    selectSource();
    expect(screen.getByText("skill.copy.noNovelSkills")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "common.cancel" }));
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("blocks copying when source skills cannot load", () => {
    mockUseSkills.mockReturnValue({
      data: [],
      isLoading: false,
      isError: true,
    });
    setup();
    selectSource();
    expect(screen.getByRole("alert")).toHaveTextContent("skill.loadFailed");
    expect(
      screen.getByRole("button", { name: "skill.copy.openDrafts" }),
    ).toBeDisabled();
  });
});

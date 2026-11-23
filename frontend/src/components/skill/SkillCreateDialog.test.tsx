import { describe, expect, it, vi, beforeEach } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { skill } from "@/lib/wailsjs/go/models";
import SkillCreateDialog from "./SkillCreateDialog";

const { mockSaveContent } = vi.hoisted(() => ({ mockSaveContent: vi.fn() }));

vi.mock("@/lib/wailsjs/go/app/App", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/wailsjs/go/app/App")>()),
  SaveContent: mockSaveContent,
}));

function meta(name: string, source: string): skill.SkillMeta {
  return { name, source, description: "" } as skill.SkillMeta;
}

function setup(skills: skill.SkillMeta[] = []) {
  const qc = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  });
  const onCreated = vi.fn();
  const onClose = vi.fn();
  render(
    <QueryClientProvider client={qc}>
      <SkillCreateDialog
        novelId={1}
        skills={skills}
        onClose={onClose}
        onCreated={onCreated}
      />
    </QueryClientProvider>,
  );
  return { onCreated, onClose };
}

function fill(name: string, description = "技能简介") {
  fireEvent.change(screen.getByPlaceholderText("skill.namePlaceholder"), {
    target: { value: name },
  });
  fireEvent.change(screen.getByPlaceholderText("skill.summaryPlaceholder"), {
    target: { value: description },
  });
}

function clickCreate() {
  fireEvent.click(screen.getByRole("button", { name: "skill.create" }));
}

describe("SkillCreateDialog", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockSaveContent.mockResolvedValue(undefined);
  });

  it("creates a novel-level skill with the trimmed name and no-overwrite guard", async () => {
    const { onCreated, onClose } = setup();
    fill("  demo  ");
    clickCreate();

    await waitFor(() =>
      expect(mockSaveContent).toHaveBeenCalledWith(
        expect.objectContaining({
          novel_id: 1,
          path: "skills/demo.md",
          expected_content: "",
          content: expect.stringContaining("name: demo"),
        }),
      ),
    );
    expect(onCreated).toHaveBeenCalledWith("demo", "novel");
    expect(onClose).toHaveBeenCalled();
  });

  it("creates a user-level skill when the user level is selected", async () => {
    const { onCreated } = setup();
    fireEvent.click(screen.getByRole("button", { name: /skill\.scopeUser/ }));
    fill("shared");
    clickCreate();

    await waitFor(() =>
      expect(mockSaveContent).toHaveBeenCalledWith(
        expect.objectContaining({ path: "~/.goink/skills/shared.md" }),
      ),
    );
    expect(onCreated).toHaveBeenCalledWith("shared", "user");
  });

  it("blocks a duplicate name in the same level before saving", () => {
    setup([meta("demo", "novel")]);
    fill("demo");
    clickCreate();

    expect(screen.getByText("skill.nameExistsAtScope")).toBeInTheDocument();
    expect(mockSaveContent).not.toHaveBeenCalled();
  });

  it("allows the same name in another level", async () => {
    setup([meta("demo", "user")]);
    fill("demo");
    clickCreate();

    await waitFor(() =>
      expect(mockSaveContent).toHaveBeenCalledWith(
        expect.objectContaining({ path: "skills/demo.md" }),
      ),
    );
  });

  it("maps a backend content conflict to the same-level duplicate message", async () => {
    mockSaveContent.mockRejectedValue(new Error("CONTENT_CONFLICT: changed"));
    const { onCreated } = setup();
    fill("demo");
    clickCreate();

    expect(
      await screen.findByText("skill.nameExistsAtScope"),
    ).toBeInTheDocument();
    expect(onCreated).not.toHaveBeenCalled();
  });

  it("rejects names that cannot be used as a file name", () => {
    setup();
    fill("a/b");
    clickCreate();

    expect(screen.getByText("skill.invalidName")).toBeInTheDocument();
    expect(mockSaveContent).not.toHaveBeenCalled();
  });

  it("requires a summary", () => {
    setup();
    fireEvent.change(screen.getByPlaceholderText("skill.namePlaceholder"), {
      target: { value: "demo" },
    });
    clickCreate();

    expect(screen.getByText("skill.summaryRequired")).toBeInTheDocument();
    expect(mockSaveContent).not.toHaveBeenCalled();
  });
});

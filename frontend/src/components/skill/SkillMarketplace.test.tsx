import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import SkillMarketplace from "./SkillMarketplace";
import { contentKeys, skillKeys } from "@/lib/queryKeys";
import { toastError } from "@/utils/toast";

const {
  mockListSkills,
  mockListRemoteSkills,
  mockGetRemoteSkillContent,
  mockGetContent,
  mockInstallRemoteSkill,
} = vi.hoisted(() => ({
  mockListSkills: vi.fn(),
  mockListRemoteSkills: vi.fn(),
  mockGetRemoteSkillContent: vi.fn(),
  mockGetContent: vi.fn(),
  mockInstallRemoteSkill: vi.fn(),
}));

vi.mock("@/lib/wailsjs/go/app/App", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/wailsjs/go/app/App")>()),
  ListSkills: mockListSkills,
  ListRemoteSkills: mockListRemoteSkills,
  GetRemoteSkillContent: mockGetRemoteSkillContent,
  GetContent: mockGetContent,
  InstallRemoteSkill: mockInstallRemoteSkill,
}));

vi.mock("@/utils/toast", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/utils/toast")>()),
  toastError: vi.fn(),
}));

vi.mock("@/components/Markdown", () => ({
  default: ({ content }: { content: string }) => <div>{content}</div>,
}));

function renderMarketplace() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: 30_000 } },
  });
  render(
    <QueryClientProvider client={qc}>
      <SkillMarketplace open onOpenChange={vi.fn()} novelId={1} />
    </QueryClientProvider>,
  );
  return qc;
}

async function openSkill() {
  fireEvent.click(await screen.findByText("my-skill"));
  await screen.findByText("skill.marketplace.installToUser");
}

describe("SkillMarketplace installation", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockListSkills.mockResolvedValue([]);
    mockListRemoteSkills.mockResolvedValue({
      err_code: "",
      data: {
        items: [{ name: "my-skill", description: "Remote skill", version: 1 }],
        total: 1,
        total_pages: 1,
      },
    });
    mockGetRemoteSkillContent.mockResolvedValue({
      err_code: "",
      data: "remote body",
    });
    mockInstallRemoteSkill.mockResolvedValue({ err_code: "", data: {} });
  });

  it("reads the target afresh before confirming and passes that content to installation", async () => {
    mockGetContent.mockResolvedValue("current local body");
    const qc = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: 30_000 } },
    });
    const path = "~/.goink/skills/my-skill.md";
    qc.setQueryData(contentKeys.detail(1, path), "stale cached body");
    qc.setQueryData(contentKeys.detail(2, path), "other novel cached body");
    qc.setQueryData(skillKeys.list(2), []);
    render(
      <QueryClientProvider client={qc}>
        <SkillMarketplace open onOpenChange={vi.fn()} novelId={1} />
      </QueryClientProvider>,
    );

    await openSkill();
    fireEvent.click(screen.getByText("skill.marketplace.installToUser"));
    expect(await screen.findByText("current local body")).toBeInTheDocument();
    expect(mockGetContent).toHaveBeenCalledWith(1, path);
    expect(mockInstallRemoteSkill).not.toHaveBeenCalled();

    fireEvent.click(screen.getByText("skill.marketplace.confirmOverwrite"));
    await vi.waitFor(() => {
      expect(mockInstallRemoteSkill).toHaveBeenCalledWith({
        name: "my-skill",
        target: "user",
        novel_id: 1,
        expected_content: "current local body",
      });
      expect(qc.getQueryState(contentKeys.detail(2, path))?.isInvalidated).toBe(
        true,
      );
      expect(qc.getQueryState(skillKeys.list(2))?.isInvalidated).toBe(true);
    });
  });

  it("reopens confirmation with the latest target after a conflict", async () => {
    mockGetContent
      .mockResolvedValueOnce("")
      .mockResolvedValueOnce("newly appeared body");
    mockInstallRemoteSkill
      .mockResolvedValueOnce({
        err_code: "conflict",
        err_msg: "target changed",
      })
      .mockResolvedValueOnce({ err_code: "", data: {} });
    renderMarketplace();

    await openSkill();
    fireEvent.click(screen.getByText("skill.marketplace.installToUser"));
    expect(await screen.findByText("newly appeared body")).toBeInTheDocument();
    expect(mockInstallRemoteSkill).toHaveBeenCalledWith({
      name: "my-skill",
      target: "user",
      novel_id: 1,
      expected_content: "",
    });
    expect(toastError).toHaveBeenCalledWith("skill.marketplace.targetChanged");

    fireEvent.click(screen.getByText("skill.marketplace.confirmOverwrite"));
    await vi.waitFor(() => {
      expect(mockInstallRemoteSkill).toHaveBeenLastCalledWith({
        name: "my-skill",
        target: "user",
        novel_id: 1,
        expected_content: "newly appeared body",
      });
    });
  });

  it("stops before installation if the target cannot be read", async () => {
    mockGetContent.mockRejectedValue(new Error("read failed"));
    renderMarketplace();

    await openSkill();
    fireEvent.click(screen.getByText("skill.marketplace.installToUser"));
    await vi.waitFor(() => {
      expect(toastError).toHaveBeenCalledWith(
        "skill.marketplace.installFailed: read failed",
      );
    });
    expect(mockInstallRemoteSkill).not.toHaveBeenCalled();
    expect(
      screen.getByText("skill.marketplace.installToUser"),
    ).toBeInTheDocument();
  });
});

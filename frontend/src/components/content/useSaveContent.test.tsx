import { describe, expect, it, vi } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { contentKeys, skillKeys } from "@/lib/queryKeys";
import { useSaveContent } from "./useSaveContent";

const { mockSaveContent } = vi.hoisted(() => ({ mockSaveContent: vi.fn() }));

vi.mock("@/lib/wailsjs/go/app/App", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/wailsjs/go/app/App")>()),
  SaveContent: mockSaveContent,
}));

describe("useSaveContent", () => {
  it("invalidates user skill content across novels and every skill list", async () => {
    mockSaveContent.mockResolvedValue(undefined);
    const qc = new QueryClient();
    const path = "~/.goink/skills/shared.md";
    for (const novelId of [1, 2]) {
      qc.setQueryData(contentKeys.detail(novelId, path), "old");
      qc.setQueryData(skillKeys.list(novelId), ["old"]);
    }
    qc.setQueryData(contentKeys.detail(2, "goink.md"), "other");
    const { result } = renderHook(() => useSaveContent(), {
      wrapper: ({ children }: { children: ReactNode }) => (
        <QueryClientProvider client={qc}>{children}</QueryClientProvider>
      ),
    });

    await act(async () => {
      await result.current.mutateAsync({ novel_id: 1, path, content: "new" });
    });

    for (const novelId of [1, 2]) {
      expect(qc.getQueryState(contentKeys.detail(novelId, path))?.isInvalidated).toBe(true);
      expect(qc.getQueryState(skillKeys.list(novelId))?.isInvalidated).toBe(true);
    }
    expect(qc.getQueryState(contentKeys.detail(2, "goink.md"))?.isInvalidated).toBe(false);
    qc.clear();
  });
});

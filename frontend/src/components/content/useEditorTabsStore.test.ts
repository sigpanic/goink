import { describe, expect, it, vi } from "vitest";

describe("editor tab persistence", () => {
  it("drops legacy numeric paths and persists backend-provided outline paths", async () => {
    localStorage.setItem(
      "goink_tabs_all",
      JSON.stringify({
        version: 1,
        byNovel: {
          "1": {
            tabs: [{ type: "file", path: "chapters/001.md", title: "旧章节" }],
            activePath: "chapters/001.md",
          },
        },
        positions: { "1:chapters/001.md:content": { updatedAt: 1 } },
      }),
    );
    vi.resetModules();
    const { useEditorTabsStore } = await import("./useEditorTabsStore");

    expect(useEditorTabsStore.getState().byNovel).toEqual({});
    expect(useEditorTabsStore.getState().positions).toEqual({});

    useEditorTabsStore.getState().openTab(1, {
      type: "file",
      path: "chapters/id_42.md",
      outlinePath: "outlines/id_42.md",
      title: "第七章",
    });
    useEditorTabsStore.getState().openTab(1, {
      type: "file",
      path: "volumes/id_10.md",
      title: "第一卷 · 卷纲",
    });
    window.dispatchEvent(new Event("beforeunload"));
    const stored = JSON.parse(localStorage.getItem("goink_tabs_all") ?? "{}");
    expect(stored.version).toBe(2);
    expect(stored.byNovel["1"].tabs[0]).toMatchObject({
      path: "chapters/id_42.md",
      outlinePath: "outlines/id_42.md",
    });
    expect(stored.byNovel["1"].tabs[1]).toMatchObject({
      path: "volumes/id_10.md",
      title: "第一卷 · 卷纲",
    });
  });
});

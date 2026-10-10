import { beforeEach, describe, expect, it, vi } from "vitest";

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

describe("editor tab reading positions", () => {
  beforeEach(async () => {
    const { useEditorTabsStore } = await import("./useEditorTabsStore");
    useEditorTabsStore.setState({ byNovel: {}, positions: {} });
  });

  it.each(["chapters/id_42.md", "outlines/id_42.md", "volumes/id_10.md"])(
    "closing the diff for %s preserves file reading positions",
    async (path) => {
      const { useEditorTabsStore } = await import("./useEditorTabsStore");
      const store = useEditorTabsStore.getState();
      const filePath = path.startsWith("volumes/") ? path : "chapters/id_42.md";
      const fileId = store.openTab(1, {
        type: "file",
        path: filePath,
        outlinePath: "outlines/id_42.md",
        title: "正文",
        content: "保留的正文",
      });
      const position = {
        viewState: {
          cursorState: [{ position: { lineNumber: 100, column: 8 } }],
          viewState: { firstPosition: { lineNumber: 96, column: 1 } },
        },
        updatedAt: 1,
      };
      store.setPosition(`1:${filePath}:content`, position);
      store.setPosition("1:outlines/id_42.md:outline", {
        scrollTop: 800,
        updatedAt: 1,
      });
      store.setPosition("1:outlines/id_42.md:outline-edit", position);
      store.setPosition("2:chapters/id_42.md:content", position);
      const positions = useEditorTabsStore.getState().positions;
      const diffId = store.openDiffTab(1, {
        path,
        title: "Diff",
        diff: "",
        original: "原稿",
        modified: "提案",
        changeType: "modify",
        reason: "",
        toolId: "tool-42",
      });

      store.closeTab(1, diffId);

      const state = useEditorTabsStore.getState();
      expect(state.byNovel["1"].activeTabId).toBe(fileId);
      expect(state.byNovel["1"].tabs).toEqual([
        expect.objectContaining({ id: fileId, content: "保留的正文" }),
      ]);
      expect(state.positions).toEqual(positions);
    },
  );

  it("closing a file still clears its body and outline positions only", async () => {
    const { useEditorTabsStore } = await import("./useEditorTabsStore");
    const store = useEditorTabsStore.getState();
    const fileId = store.openTab(1, {
      type: "file",
      path: "chapters/id_42.md",
      outlinePath: "outlines/id_42.md",
      title: "正文",
    });
    const position = { scrollTop: 800, updatedAt: 1 };
    const retainedPositions = {
      "1:volumes/id_10.md:content": position,
      "2:chapters/id_42.md:content": position,
    };
    useEditorTabsStore.setState({
      positions: {
        "1:chapters/id_42.md:content": position,
        "1:outlines/id_42.md:outline": position,
        "1:outlines/id_42.md:outline-edit": position,
        ...retainedPositions,
      },
    });

    store.closeTab(1, fileId);

    expect(useEditorTabsStore.getState().byNovel["1"].tabs).toEqual([]);
    expect(useEditorTabsStore.getState().positions).toEqual(retainedPositions);
  });
});

import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { chapterKeys, volumeKeys } from "@/lib/queryKeys";
import { useEditorTabsStore } from "./useEditorTabsStore";
import { useEditorTabTitles } from "./useEditorTabTitles";

const { getChapters, getVolumes } = vi.hoisted(() => ({
  getChapters: vi.fn(),
  getVolumes: vi.fn(),
}));
vi.mock("@/lib/wailsjs/go/app/App", () => ({
  GetChapters: getChapters,
  GetVolumes: getVolumes,
}));
vi.mock("react-i18next", async () => {
  const { createInstance } = await import("i18next");
  const { default: zh } = await import("@/i18n/locales/zh-CN.json");
  const i18n = createInstance();
  await i18n.init({
    lng: "zh-CN",
    resources: { "zh-CN": { translation: zh } },
  });
  return { useTranslation: () => ({ t: i18n.t }) };
});
const chapter = {
  file_path: "chapters/id_42.md",
  outline_file_path: "outlines/id_42.md",
  reading_number: 7,
  title: "重逢",
};
const volume = { outline_file_path: "volumes/id_1.md", name: "第一卷 风起" };
function setup(novelId = 1) {
  const qc = new QueryClient({
    defaultOptions: { queries: { staleTime: 30_000, retry: false } },
  });
  const hook = renderHook(({ novelId }) => useEditorTabTitles(novelId), {
    initialProps: { novelId },
    wrapper: ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={qc}>{children}</QueryClientProvider>
    ),
  });
  return { qc, ...hook };
}
beforeEach(() => {
  vi.clearAllMocks();
  useEditorTabsStore.setState({ byNovel: {}, positions: {} });
  getChapters.mockResolvedValue([chapter]);
  getVolumes.mockResolvedValue([volume]);
});

describe("useEditorTabTitles", () => {
  it("恢复路径标题、同步重命名和重排，保留草稿与阅读位置", async () => {
    const store = useEditorTabsStore.getState();
    const chapterId = store.openTab(1, {
      type: "file",
      path: chapter.file_path,
      title: chapter.file_path,
      content: "本地草稿",
      isDirty: true,
      contentConflict: true,
      viewMode: "outline-edit",
    });
    const volumeId = store.openTab(1, {
      type: "file",
      path: volume.outline_file_path,
      title: volume.outline_file_path,
    });
    const diffId = store.openDiffTab(1, {
      path: chapter.outline_file_path,
      title: `diff: ${chapter.outline_file_path}`,
      diff: "",
      original: "旧",
      modified: "新",
      changeType: "full_replace",
      reason: "",
      toolId: "tool-42",
    });
    const positionKey = "1:chapters/id_42.md:content";
    const position = { scrollTop: 120, updatedAt: 1 };
    store.setPosition(positionKey, position);
    const { qc } = setup();
    const tab = (id: string) =>
      useEditorTabsStore
        .getState()
        .byNovel["1"].tabs.find((item) => item.id === id);
    await waitFor(() => {
      expect(tab(chapterId)?.title).toBe("第7章 重逢");
      expect(tab(volumeId)?.title).toBe("第一卷 风起 · 卷纲");
      expect(tab(diffId)?.title).toBe("diff: 第7章大纲");
    });
    act(() => {
      qc.setQueryData(chapterKeys.list(1), [
        { ...chapter, reading_number: 3, title: "再相见" },
      ]);
      qc.setQueryData(volumeKeys.list(1), [
        { ...volume, name: "第一卷 初入江湖" },
      ]);
    });
    await waitFor(() => {
      expect(tab(chapterId)?.title).toBe("第3章 再相见");
      expect(tab(volumeId)?.title).toBe("第一卷 初入江湖 · 卷纲");
      expect(tab(diffId)?.title).toBe("diff: 第3章大纲");
    });
    expect(tab(chapterId)).toMatchObject({
      content: "本地草稿",
      isDirty: true,
      contentConflict: true,
      viewMode: "outline-edit",
    });
    expect(tab(diffId)).toMatchObject({ original: "旧", modified: "新" });
    expect(useEditorTabsStore.getState().positions[positionKey]).toEqual(
      position,
    );
    expect(useEditorTabsStore.getState().byNovel["1"].activeTabId).toBe(diffId);
  });

  it("审批立即打开，缺失缓存加载后更新卷纲名称", async () => {
    let finish!: (value: (typeof volume)[]) => void;
    getVolumes.mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
    );
    const store = useEditorTabsStore.getState();
    const id = store.openDiffTab(1, {
      path: volume.outline_file_path,
      title: `diff: ${volume.outline_file_path}`,
      diff: "",
      original: "旧卷纲",
      modified: "新卷纲",
      changeType: "full_replace",
      reason: "",
      toolId: "tool-1",
    });
    setup();
    expect(useEditorTabsStore.getState().byNovel["1"].activeTabId).toBe(id);
    await act(async () => finish([volume]));
    await waitFor(() =>
      expect(useEditorTabsStore.getState().byNovel["1"].tabs[0].title).toBe(
        "diff: 第一卷 风起 · 卷纲",
      ),
    );
  });

  it("切换小说后，旧查询不能修改当前小说标签或重新打开已关闭审批", async () => {
    let finish!: (value: (typeof volume)[]) => void;
    getVolumes.mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
    );
    const store = useEditorTabsStore.getState();
    const id = store.openTab(1, {
      type: "file",
      path: volume.outline_file_path,
      title: "旧小说",
    });
    store.openTab(2, { type: "file", path: "goink.md", title: "故事状态" });
    const { rerender } = setup();
    act(() => {
      store.closeTab(1, id);
      rerender({ novelId: 2 });
    });
    await act(async () => finish([volume]));
    expect(useEditorTabsStore.getState().byNovel["1"].tabs).toEqual([]);
    expect(useEditorTabsStore.getState().byNovel["2"].tabs[0].title).toBe(
      "故事状态",
    );
  });
});

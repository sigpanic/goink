import { act, renderHook, waitFor } from "@testing-library/react";
import {
  QueryClient,
  QueryClientProvider,
  useQuery,
} from "@tanstack/react-query";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { EventsOn } from "@/lib/wailsjs/runtime/runtime";
import {
  chapterKeys,
  contentKeys,
  maxChapterKeys,
  skillKeys,
} from "@/lib/queryKeys";
import { useAIFileCacheInvalidation } from "./useAIFileCacheInvalidation";
import { useEditorTabsStore } from "@/components/content/useEditorTabsStore";

type FileChangedEvent = { novel_id?: number; path?: string };
let onFileChanged: ((event: FileChangedEvent) => void) | undefined;
const unsubscribe = vi.fn();

function setup() {
  const qc = new QueryClient({
    defaultOptions: { queries: { staleTime: 30_000, retry: false } },
  });
  const result = renderHook(() => useAIFileCacheInvalidation(), {
    wrapper: ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={qc}>{children}</QueryClientProvider>
    ),
  });
  return { qc, ...result };
}

function emit(novelId: number, path: string) {
  act(() => onFileChanged?.({ novel_id: novelId, path }));
}

beforeEach(() => {
  onFileChanged = undefined;
  unsubscribe.mockClear();
  useEditorTabsStore.setState({ byNovel: {}, positions: {} });
  vi.mocked(EventsOn).mockImplementation((name, callback) => {
    expect(name).toBe("file:changed");
    onFileChanged = callback;
    return unsubscribe;
  });
});

describe("useAIFileCacheInvalidation", () => {
  it("编辑器未挂载时仍标记已打开文件供恢复后刷新", () => {
    useEditorTabsStore.setState({
      byNovel: {
        "1": {
          activeTabId: "chapter",
          tabs: [{
            id: "chapter",
            type: "file",
            path: "chapters/id_3.md",
            outlinePath: "outlines/id_3.md",
            title: "第三章",
            content: "旧正文",
            outlineContent: "旧大纲",
          }],
        },
      },
    });
    setup();

    emit(1, "outlines/id_3.md");
    expect(useEditorTabsStore.getState().byNovel["1"].tabs[0]).toMatchObject({
      content: "旧正文",
      outlineContent: "旧大纲",
      outlineNeedsRefresh: true,
    });
  });

  it("章节列表正在显示时，AI 修改章节后重新读取列表", async () => {
    const qc = new QueryClient({
      defaultOptions: { queries: { staleTime: 30_000, retry: false } },
    });
    const getChapters = vi
      .fn()
      .mockResolvedValueOnce(["旧章节"])
      .mockResolvedValueOnce(["新章节"]);
    const { result } = renderHook(
      () => {
        useAIFileCacheInvalidation();
        return useQuery({
          queryKey: chapterKeys.list(1),
          queryFn: getChapters,
        });
      },
      {
        wrapper: ({ children }: { children: ReactNode }) => (
          <QueryClientProvider client={qc}>{children}</QueryClientProvider>
        ),
      },
    );

    await waitFor(() => expect(result.current.data).toEqual(["旧章节"]));
    emit(1, "chapters/id_3.md");
    await waitFor(() => expect(result.current.data).toEqual(["新章节"]));
    expect(getChapters).toHaveBeenCalledTimes(2);
  });

  it("失效对应文件后，下次读取绕过 30 秒缓存，其他小说和路径保持缓存", async () => {
    const { qc } = setup();
    const changed = contentKeys.detail(1, "volumes/id_2.md");
    const otherPath = contentKeys.detail(1, "goink.md");
    const otherNovel = contentKeys.detail(2, "volumes/id_2.md");
    qc.setQueryData(changed, "旧卷纲");
    qc.setQueryData(otherPath, "故事状态");
    qc.setQueryData(otherNovel, "别的小说");

    emit(1, "volumes/id_2.md");

    const readFresh = vi.fn().mockResolvedValue("新卷纲");
    expect(await qc.fetchQuery({ queryKey: changed, queryFn: readFresh })).toBe(
      "新卷纲",
    );
    expect(readFresh).toHaveBeenCalledOnce();
    expect(qc.getQueryData(otherPath)).toBe("故事状态");
    expect(qc.getQueryState(otherPath)?.isInvalidated).toBe(false);
    expect(qc.getQueryState(otherNovel)?.isInvalidated).toBe(false);
  });

  it("AI 事件取消在途的旧内容读取，旧结果不能重新填入缓存", async () => {
    const { qc } = setup();
    const path = "chapters/id_3.md";
    let resolveOld!: (content: string) => void;
    const pending = qc.fetchQuery({
      queryKey: contentKeys.detail(1, path),
      queryFn: () => new Promise<string>((resolve) => { resolveOld = resolve; }),
    }).catch(() => undefined);

    emit(1, path);
    resolveOld("过期正文");
    await pending;
    const readFresh = vi.fn().mockResolvedValue("AI 正文");
    expect(await qc.fetchQuery({
      queryKey: contentKeys.detail(1, path),
      queryFn: readFresh,
    })).toBe("AI 正文");
    expect(readFresh).toHaveBeenCalledOnce();
  });

  it.each([
    ["chapters/id_3.md", true],
    ["outlines/id_3.md", false],
    ["goink.md", false],
  ])("%s 变更刷新章节列表，并按需刷新最大章节号", (path, refreshMax) => {
    const { qc } = setup();
    qc.setQueryData(chapterKeys.list(1), []);
    qc.setQueryData(maxChapterKeys.detail(1), 3);
    qc.setQueryData(chapterKeys.list(2), []);

    emit(1, path);

    expect(qc.getQueryState(chapterKeys.list(1))?.isInvalidated).toBe(true);
    expect(qc.getQueryState(maxChapterKeys.detail(1))?.isInvalidated).toBe(
      refreshMax,
    );
    expect(qc.getQueryState(chapterKeys.list(2))?.isInvalidated).toBe(false);
  });

  it("小说级技能只刷新该小说，用户级技能刷新所有小说的技能列表", () => {
    const { qc } = setup();
    qc.setQueryData(skillKeys.list(1), []);
    qc.setQueryData(skillKeys.list(2), []);
    qc.setQueryData(
      contentKeys.detail(1, "~/.goink/skills/style.md"),
      "旧内容",
    );
    qc.setQueryData(
      contentKeys.detail(2, "~/.goink/skills/style.md"),
      "旧内容",
    );
    qc.setQueryData(contentKeys.detail(2, "skills/style.md"), "小说专属");

    emit(1, "skills/plot.md");
    expect(qc.getQueryState(skillKeys.list(1))?.isInvalidated).toBe(true);
    expect(qc.getQueryState(skillKeys.list(2))?.isInvalidated).toBe(false);

    qc.setQueryData(skillKeys.list(1), []);
    emit(1, "~/.goink/skills/style.md");
    expect(qc.getQueryState(skillKeys.list(1))?.isInvalidated).toBe(true);
    expect(qc.getQueryState(skillKeys.list(2))?.isInvalidated).toBe(true);
    expect(
      qc.getQueryState(contentKeys.detail(1, "~/.goink/skills/style.md"))
        ?.isInvalidated,
    ).toBe(true);
    expect(
      qc.getQueryState(contentKeys.detail(2, "~/.goink/skills/style.md"))
        ?.isInvalidated,
    ).toBe(true);
    expect(
      qc.getQueryState(contentKeys.detail(2, "skills/style.md"))?.isInvalidated,
    ).toBe(false);
  });

  it("忽略无效事件，并在工作区卸载时取消订阅", () => {
    const { qc, unmount } = setup();
    qc.setQueryData(contentKeys.detail(1, "goink.md"), "旧内容");

    act(() => onFileChanged?.({ path: "goink.md" }));
    act(() => onFileChanged?.({ novel_id: 1 }));
    expect(
      qc.getQueryState(contentKeys.detail(1, "goink.md"))?.isInvalidated,
    ).toBe(false);

    unmount();
    expect(unsubscribe).toHaveBeenCalledOnce();
  });
});

import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { createInstance } from "i18next";
import { QueryClient } from "@tanstack/react-query";
import zh from "@/i18n/locales/zh-CN.json";
import { chapterKeys, volumeKeys } from "@/lib/queryKeys";
import { describeFile, resolveFileDescription } from "./fileDescription";

const { getChapters, getVolumes } = vi.hoisted(() => ({
  getChapters: vi.fn(),
  getVolumes: vi.fn(),
}));
vi.mock("@/lib/wailsjs/go/app/App", () => ({
  GetChapters: getChapters,
  GetVolumes: getVolumes,
}));
const i18n = createInstance();
const chapter = {
  file_path: "chapters/id_42.md",
  outline_file_path: "outlines/id_42.md",
  reading_number: 7,
  title: "重逢",
};
const volume = { outline_file_path: "volumes/id_1.md", name: "第一卷 风起" };
beforeAll(async () => {
  await i18n.init({
    lng: "zh-CN",
    resources: { "zh-CN": { translation: zh } },
  });
});
beforeEach(() => {
  vi.clearAllMocks();
  getChapters.mockResolvedValue([chapter]);
  getVolumes.mockResolvedValue([volume]);
});

describe("fileDescription", () => {
  it.each([
    [
      "chapters/id_42.md",
      "第7章 重逢",
      "diff: 第7章",
      "chapters/id_42.md",
      "content",
    ],
    [
      "outlines/id_42.md",
      "第7章 重逢",
      "diff: 第7章大纲",
      "chapters/id_42.md",
      "outline",
    ],
    [
      "volumes/id_1.md",
      "第一卷 风起 · 卷纲",
      "diff: 第一卷 风起 · 卷纲",
      "volumes/id_1.md",
      "content",
    ],
    ["goink.md", "故事状态", "diff: 故事状态", "goink.md", "content"],
    [
      "skills/style.md",
      "技能: style",
      "diff: 技能: style",
      "skills/style.md",
      "preview",
    ],
    [
      "chapters/3/new.md",
      "新建章节",
      "diff: 新建章节",
      "chapters/3/new.md",
      "content",
    ],
    [
      "outlines/new.md",
      "新建大纲",
      "diff: 新建大纲",
      "outlines/new.md",
      "content",
    ],
  ])(
    "统一解析 %s 的标签、审批标题和打开目标",
    (path, title, diffTitle, tabPath, viewMode) => {
      expect(describeFile(path, i18n.t, [chapter], [volume])).toMatchObject({
        title,
        diffTitle,
        tabPath,
        viewMode,
      });
    },
  );

  it("卷纲缓存未加载时查询卷信息，按小说隔离", async () => {
    const qc = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: 30_000 } },
    });
    qc.setQueryData(volumeKeys.list(2), [{ ...volume, name: "另一部小说" }]);
    expect(
      await resolveFileDescription(qc, 1, volume.outline_file_path, i18n.t),
    ).toMatchObject({ title: "第一卷 风起 · 卷纲", resolved: true });
    expect(getVolumes).toHaveBeenCalledExactlyOnceWith(1);
    expect(getChapters).not.toHaveBeenCalled();
  });

  it("新建成功后的实际路径不在旧缓存中时刷新元数据", async () => {
    const qc = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: 30_000 } },
    });
    qc.setQueryData(chapterKeys.list(1), []);
    expect(
      await resolveFileDescription(qc, 1, chapter.outline_file_path, i18n.t),
    ).toMatchObject({
      title: "第7章 重逢",
      tabPath: chapter.file_path,
      viewMode: "outline",
    });
    expect(getChapters).toHaveBeenCalledExactlyOnceWith(1);
  });

  it("查询失败或资源不存在时保留路径，不推断其他文件", async () => {
    const qc = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    getVolumes.mockRejectedValue(new Error("unavailable"));
    expect(
      await resolveFileDescription(qc, 1, volume.outline_file_path, i18n.t),
    ).toMatchObject({
      title: volume.outline_file_path,
      tabPath: volume.outline_file_path,
      resolved: false,
    });
    getChapters.mockResolvedValue([]);
    expect(
      await resolveFileDescription(qc, 1, chapter.outline_file_path, i18n.t),
    ).toMatchObject({ tabPath: chapter.outline_file_path, resolved: false });
  });

  it("技能层级和新建通道不触发元数据查询", async () => {
    const qc = new QueryClient();
    expect(
      await resolveFileDescription(qc, 1, "/builtin/skills/style.md", i18n.t),
    ).toMatchObject({ readOnly: true, viewMode: "preview" });
    await resolveFileDescription(qc, 1, "outlines/3/new.md", i18n.t);
    expect(getChapters).not.toHaveBeenCalled();
    expect(getVolumes).not.toHaveBeenCalled();
  });
});

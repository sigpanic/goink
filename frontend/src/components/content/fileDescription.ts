import type { TFunction } from "i18next";
import type { QueryClient } from "@tanstack/react-query";
import { GetChapters, GetVolumes } from "@/lib/wailsjs/go/app/App";
import type { chapter, volume } from "@/lib/wailsjs/go/models";
import { chapterKeys, volumeKeys } from "@/lib/queryKeys";
import {
  isOutlinePath,
  isSkillPath,
  isVolumeOutlinePath,
  skillNameFromPath,
} from "./types";

export type ChapterMetadata = Pick<
  chapter.Chapter,
  "file_path" | "outline_file_path" | "reading_number" | "title"
>;
type VolumeMetadata = Pick<volume.Volume, "outline_file_path" | "name">;

export function describeFile(
  path: string,
  t: TFunction,
  chapters: readonly ChapterMetadata[] = [],
  volumes: readonly VolumeMetadata[] = [],
) {
  const chapter = chapters.find(
    (item) => item.file_path === path || item.outline_file_path === path,
  );
  const volume = volumes.find((item) => item.outline_file_path === path);
  const outline = chapter?.outline_file_path === path;
  let title = path;
  let diffLabel = path;
  let resolved = false;
  if (chapter) {
    title = `${t("sidebar.chapterN", { n: chapter.reading_number })} ${chapter.title}`;
    diffLabel = t(outline ? "chat.diffChapterOutline" : "chat.diffChapter", {
      n: chapter.reading_number,
    });
    resolved = true;
  } else if (volume) {
    title = t("sidebar.volumeOutlineTitle", { name: volume.name });
    diffLabel = title;
    resolved = true;
  } else if (/^(chapters|outlines)\/(?:\d+\/)?new\.md$/.test(path)) {
    title = t(
      isOutlinePath(path) ? "chat.diffNewOutline" : "chat.diffNewChapter",
    );
    diffLabel = title;
    resolved = true;
  } else if (path === "goink.md") {
    title = t("content.storyStatus");
    diffLabel = t("chat.diffStoryStatus");
    resolved = true;
  } else if (isSkillPath(path)) {
    title = `${t("content.skillLabel")}${skillNameFromPath(path)}`;
    diffLabel = title;
    resolved = true;
  }
  return {
    title,
    diffTitle: `diff: ${diffLabel}`,
    resolved,
    chapter,
    tabPath: outline ? chapter.file_path : path,
    outlinePath: chapter?.outline_file_path,
    viewMode: outline
      ? ("outline" as const)
      : isSkillPath(path)
        ? ("preview" as const)
        : ("content" as const),
    readOnly: path.startsWith("/builtin/skills/"),
  };
}

export function cachedFileDescription(
  qc: QueryClient,
  novelId: number,
  path: string,
  t: TFunction,
) {
  return describeFile(
    path,
    t,
    qc.getQueryData<chapter.Chapter[]>(chapterKeys.list(novelId)),
    qc.getQueryData<volume.Volume[]>(volumeKeys.list(novelId)),
  );
}

export async function resolveFileDescription(
  qc: QueryClient,
  novelId: number,
  path: string,
  t: TFunction,
) {
  const cached = cachedFileDescription(qc, novelId, path, t);
  if (!novelId || /\/new\.md$/.test(path)) return cached;
  const chapterPath = path.startsWith("chapters/") || isOutlinePath(path);
  const volumePath = isVolumeOutlinePath(path);
  if (!chapterPath && !volumePath) return cached;
  const queryKey = chapterPath
    ? chapterKeys.list(novelId)
    : volumeKeys.list(novelId);
  const queryFn = async (): Promise<chapter.Chapter[] | volume.Volume[]> =>
    chapterPath
      ? ((await GetChapters(novelId)) ?? [])
      : ((await GetVolumes(novelId)) ?? []);
  const hadCache = qc.getQueryData(queryKey) !== undefined;
  try {
    await qc.fetchQuery({ queryKey, queryFn });
    if (!cachedFileDescription(qc, novelId, path, t).resolved && hadCache) {
      await qc.invalidateQueries({ queryKey, exact: true });
      await qc.fetchQuery({ queryKey, queryFn });
    }
  } catch {
    return cachedFileDescription(qc, novelId, path, t);
  }
  return cachedFileDescription(qc, novelId, path, t);
}

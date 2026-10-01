import { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { ChevronRight } from "lucide-react";
import { EventsOn } from "@/lib/wailsjs/runtime/runtime";
import type { chapter } from "@/lib/wailsjs/go/models";
import { chapterKeys } from "@/lib/queryKeys";
import { useChapters } from "@/components/chapter/useChapters";
import { useVolumes } from "@/components/volume/useVolumes";

interface Props {
  novelId: number;
}

const EMPTY_CHAPTERS: chapter.Chapter[] = [];
const AUTO_EXPAND_LIMIT = 30;
const BLOCK_SIZE = 100;

export default function ChapterManagementView({ novelId }: Props) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const chapterQuery = useChapters(novelId);
  const volumeQuery = useVolumes(novelId);
  const chapters = chapterQuery.data ?? EMPTY_CHAPTERS;
  const [groupExpanded, setGroupExpanded] = useState<Record<string, boolean>>(
    {},
  );
  const [openRanges, setOpenRanges] = useState<Set<string>>(new Set());
  const sectionRefs = useRef(new Map<string, HTMLElement>());
  const volumes = useMemo(
    () =>
      [...(volumeQuery.data ?? [])].sort(
        (a, b) => a.sort_order - b.sort_order || a.id - b.id,
      ),
    [volumeQuery.data],
  );

  useEffect(() => {
    if (!novelId) return;
    const unsubscribe = EventsOn(
      "file:changed",
      (data: { novel_id?: number; path?: string }) => {
        if (data?.novel_id !== novelId) return;
        if (
          data.path?.startsWith("chapters/") ||
          data.path?.startsWith("outlines/")
        ) {
          void qc.invalidateQueries({ queryKey: chapterKeys.list(novelId) });
        }
      },
    );
    return () => unsubscribe();
  }, [novelId, qc]);

  const groups = useMemo(() => {
    const byVolume = new Map<number, chapter.Chapter[]>();
    for (const volume of volumes) byVolume.set(volume.id, []);
    const unassigned: chapter.Chapter[] = [];
    for (const item of chapters) {
      const group =
        item.volume_id == null ? null : byVolume.get(item.volume_id);
      if (group) group.push(item);
      else unassigned.push(item);
    }
    const byReadingNumber = (a: chapter.Chapter, b: chapter.Chapter) =>
      a.reading_number - b.reading_number;
    for (const group of byVolume.values()) group.sort(byReadingNumber);
    unassigned.sort(byReadingNumber);
    return { byVolume, unassigned };
  }, [chapters, volumes]);

  function groupKey(volumeId: number | null) {
    return `${novelId}:${volumeId ?? "unassigned"}`;
  }

  function isGroupExpanded(key: string, count: number) {
    return groupExpanded[key] ?? count <= AUTO_EXPAND_LIMIT;
  }

  function jumpToGroup(key: string) {
    setGroupExpanded((previous) => ({ ...previous, [key]: true }));
    sectionRefs.current.get(key)?.scrollIntoView?.({
      behavior: "smooth",
      block: "start",
    });
  }

  function toggleGroup(key: string, count: number) {
    setGroupExpanded((previous) => ({
      ...previous,
      [key]: !(previous[key] ?? count <= AUTO_EXPAND_LIMIT),
    }));
  }

  function toggleRange(key: string) {
    setOpenRanges((previous) => {
      const next = new Set(previous);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  }

  function renderChapters(items: chapter.Chapter[]) {
    if (items.length === 0) {
      return (
        <p className="px-4 py-5 text-sm text-muted-foreground">
          {t("chapterManagement.emptyGroup")}
        </p>
      );
    }
    return (
      <ol className="divide-y divide-border">
        {items.map((item) => (
          <li key={item.id} className="flex items-center gap-4 px-4 py-3">
            <span className="w-16 shrink-0 text-sm tabular-nums text-muted-foreground">
              {t("sidebar.chapterN", { n: item.reading_number })}
            </span>
            <span
              className="min-w-0 flex-1 truncate text-sm"
              title={item.title}
            >
              {item.title}
            </span>
            <span className="shrink-0 text-xs tabular-nums text-muted-foreground">
              {t("sidebar.wordCount", { count: item.word_count })}
            </span>
          </li>
        ))}
      </ol>
    );
  }

  function renderGroup(key: string, name: string, items: chapter.Chapter[]) {
    const expanded = isGroupExpanded(key, items.length);
    const blocks: chapter.Chapter[][] = [];
    if (expanded && items.length > BLOCK_SIZE) {
      for (let start = 0; start < items.length; start += BLOCK_SIZE) {
        blocks.push(items.slice(start, start + BLOCK_SIZE));
      }
    }
    return (
      <section
        key={key}
        ref={(element) => {
          if (element) sectionRefs.current.set(key, element);
          else sectionRefs.current.delete(key);
        }}
        className="scroll-mt-28 overflow-hidden rounded-xl border"
      >
        <h2>
          <button
            type="button"
            aria-expanded={expanded}
            onClick={() => toggleGroup(key, items.length)}
            className="flex w-full items-center gap-2 border-b bg-muted/30 px-4 py-3 text-left text-sm font-medium hover:bg-muted/60 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring"
          >
            <ChevronRight
              aria-hidden="true"
              className={`h-4 w-4 shrink-0 transition-transform ${expanded ? "rotate-90" : ""}`}
            />
            <span className="min-w-0 flex-1 truncate">{name}</span>
            <span className="text-xs font-normal text-muted-foreground">
              {t("sidebar.chapterCountShort", { count: items.length })}
            </span>
          </button>
        </h2>
        {expanded &&
          (blocks.length === 0 ? (
            renderChapters(items)
          ) : (
            <div className="divide-y divide-border">
              {blocks.map((block) => {
                const rangeKey = `${key}:${block[0].id}`;
                const rangeOpen = openRanges.has(rangeKey);
                return (
                  <div key={rangeKey}>
                    <button
                      type="button"
                      aria-expanded={rangeOpen}
                      onClick={() => toggleRange(rangeKey)}
                      className="flex w-full items-center gap-2 px-5 py-3 text-left text-sm hover:bg-muted/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring"
                    >
                      <ChevronRight
                        aria-hidden="true"
                        className={`h-4 w-4 shrink-0 transition-transform ${rangeOpen ? "rotate-90" : ""}`}
                      />
                      {t("sidebar.chapterRange", {
                        start: block[0].reading_number,
                        end: block[block.length - 1].reading_number,
                      })}
                    </button>
                    {rangeOpen && renderChapters(block)}
                  </div>
                );
              })}
            </div>
          ))}
      </section>
    );
  }

  const isLoading = chapterQuery.isPending || volumeQuery.isPending;
  const isError = chapterQuery.isError || volumeQuery.isError;

  return (
    <main className="flex min-h-0 min-w-0 flex-1 flex-col bg-background">
      <header className="shrink-0 border-b px-6 py-4">
        <h1 className="text-xl font-semibold">
          {t("chapterManagement.title")}
        </h1>
      </header>
      <div className="min-h-0 flex-1 overflow-y-auto p-6">
        <div className="mx-auto flex w-full max-w-6xl flex-col gap-6">
          {!novelId ? (
            <p className="text-sm text-muted-foreground">
              {t("chapterManagement.selectNovel")}
            </p>
          ) : isLoading ? (
            <p role="status" className="text-sm text-muted-foreground">
              {t("chapterManagement.loading")}
            </p>
          ) : isError ? (
            <div role="alert" className="flex items-center gap-3">
              <p className="text-sm text-destructive">
                {t("chapterManagement.loadFailed")}
              </p>
              <button
                type="button"
                onClick={() => {
                  void Promise.all([
                    chapterQuery.refetch(),
                    volumeQuery.refetch(),
                  ]);
                }}
                className="rounded-md border px-3 py-1.5 text-sm hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                {t("chapterManagement.retry")}
              </button>
            </div>
          ) : (
            <>
              <section
                aria-label={t("chapterManagement.volumes")}
                className="sticky top-0 z-10 rounded-lg bg-background py-2"
              >
                <h2 className="mb-3 text-sm font-medium text-muted-foreground">
                  {t("chapterManagement.volumes")}
                </h2>
                {volumes.length === 0 && (
                  <p className="mb-3 text-sm text-muted-foreground">
                    {t("chapterManagement.noVolumes")}
                  </p>
                )}
                <div className="flex gap-2 overflow-x-auto pb-1">
                  {volumes.map((volume) => (
                    <button
                      key={volume.id}
                      type="button"
                      onClick={() => jumpToGroup(groupKey(volume.id))}
                      className="shrink-0 rounded-lg border bg-card px-3 py-2 text-sm hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                    >
                      {volume.name}
                    </button>
                  ))}
                  <button
                    type="button"
                    onClick={() => jumpToGroup(groupKey(null))}
                    className="shrink-0 rounded-lg border border-dashed px-3 py-2 text-sm text-muted-foreground hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  >
                    {t("chapterManagement.unassigned")}
                  </button>
                </div>
              </section>

              {chapters.length === 0 && (
                <p className="text-sm text-muted-foreground">
                  {t("chapterManagement.noChapters")}
                </p>
              )}

              <div className="flex flex-col gap-5">
                {volumes.map((volume) =>
                  renderGroup(
                    groupKey(volume.id),
                    volume.name,
                    groups.byVolume.get(volume.id) ?? EMPTY_CHAPTERS,
                  ),
                )}
                {renderGroup(
                  groupKey(null),
                  t("chapterManagement.unassigned"),
                  groups.unassigned,
                )}
              </div>
            </>
          )}
        </div>
      </div>
    </main>
  );
}

import { useEffect, useMemo, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { Download, FileText, Plus } from "lucide-react";
import type { chapter, volume } from "@/lib/wailsjs/go/models";
import { EventsOn } from "@/lib/wailsjs/runtime/runtime";
import { chapterKeys, maxChapterKeys } from "@/lib/queryKeys";
import { toastError } from "@/utils/toast";
import { toErrorMessage } from "@/utils/error";
import { useEditorStore } from "@/stores/useEditorStore";
import { useEditorTabsStore } from "@/components/content/useEditorTabsStore";
import type { ChapterEditor } from "@/components/chapter-management/ChapterEditorForm";
import ChapterCreateDialog from "@/components/chapter-management/ChapterCreateDialog";
import { useChapterStructureMutations } from "@/components/chapter-management/useChapterStructureMutations";
import { useVolumes } from "@/components/volume/useVolumes";
import { useChapters } from "./useChapters";
import { useUpdateChapterTitle } from "./useUpdateChapterTitle";
import SidebarChapterGroup, { SIDEBAR_BLOCK_SIZE } from "./SidebarChapterGroup";

interface Props {
  novelId: number;
  onSelectChapter: (ch: chapter.Chapter) => void;
  onSelectGoink: () => void;
  onExportNovel: () => void;
}

interface ChapterGroup {
  key: string;
  volumeId: number | null;
  name: string;
  items: chapter.Chapter[];
}

const EMPTY_CHAPTERS: chapter.Chapter[] = [];
const EMPTY_VOLUMES: volume.Volume[] = [];
type GroupExpansion = { expanded: boolean; anchorId?: number };
type RangeSelection = { index: number; anchorId?: number };

function groupKey(volumeId: number | null): string {
  return volumeId === null ? "unassigned" : `volume:${volumeId}`;
}

export default function ChapterList({
  novelId,
  onSelectChapter,
  onSelectGoink,
  onExportNovel,
}: Props) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const target = useEditorStore((state) => state.tabTarget);
  const activeTabPath = useEditorTabsStore((state) => {
    const entry = state.byNovel[String(novelId)];
    return entry?.tabs.find((tab) => tab.id === entry.activeTabId)?.path;
  });
  const chapterQuery = useChapters(novelId);
  const volumeQuery = useVolumes(novelId);
  const { place } = useChapterStructureMutations(novelId);
  const updateTitle = useUpdateChapterTitle(novelId);
  const chapters = chapterQuery.data ?? EMPTY_CHAPTERS;
  const volumes = useMemo(
    () =>
      [...(volumeQuery.data ?? EMPTY_VOLUMES)].sort(
        (a, b) => a.sort_order - b.sort_order || a.id - b.id,
      ),
    [volumeQuery.data],
  );
  const [editor, setEditor] = useState<ChapterEditor | null>(null);
  const [expandedGroups, setExpandedGroups] = useState<
    Record<string, GroupExpansion>
  >({});
  const [selectedRanges, setSelectedRanges] = useState<
    Record<string, RangeSelection>
  >({});

  useEffect(() => {
    const unsubscribe = EventsOn(
      "file:changed",
      (data: { novel_id?: number; path?: string }) => {
        if (data.novel_id !== novelId || !data.path) return;
        if (
          data.path.startsWith("chapters/") ||
          data.path.startsWith("outlines/") ||
          data.path === "goink.md"
        ) {
          void qc.invalidateQueries({ queryKey: chapterKeys.list(novelId) });
          if (data.path.startsWith("chapters/")) {
            void qc.invalidateQueries({
              queryKey: maxChapterKeys.detail(novelId),
            });
          }
        }
      },
    );
    return () => unsubscribe();
  }, [novelId, qc]);

  const groups = useMemo(() => {
    const byVolume = new Map<number, ChapterGroup>();
    for (const item of volumes) {
      byVolume.set(item.id, {
        key: groupKey(item.id),
        volumeId: item.id,
        name: item.name,
        items: [],
      });
    }
    const unassigned: ChapterGroup = {
      key: groupKey(null),
      volumeId: null,
      name: t("chapterManagement.unassigned"),
      items: [],
    };
    for (const item of chapters) {
      const group =
        item.volume_id == null ? unassigned : byVolume.get(item.volume_id);
      (group ?? unassigned).items.push(item);
    }
    for (const group of byVolume.values()) {
      group.items.sort((a, b) => a.reading_number - b.reading_number);
    }
    unassigned.items.sort((a, b) => a.reading_number - b.reading_number);
    return [...byVolume.values(), unassigned];
  }, [chapters, volumes, t]);

  const selectedPath = activeTabPath ?? target?.path;
  const activeChapter = chapters.find(
    (item) =>
      item.file_path === selectedPath ||
      item.outline_file_path === selectedPath,
  );
  const activeChapterId = activeChapter?.id;
  const activeGroupKey = activeChapter
    ? groupKey(activeChapter.volume_id ?? null)
    : null;
  const activeGroup = groups.find((group) => group.key === activeGroupKey);
  const activeRangeIndex = activeChapter
    ? Math.floor(
        Math.max(
          0,
          activeGroup?.items.findIndex(
            (item) => item.id === activeChapter.id,
          ) ?? 0,
        ) / SIDEBAR_BLOCK_SIZE,
      )
    : 0;
  const defaultGroupKey =
    activeGroupKey ?? groupKey(volumes[volumes.length - 1]?.id ?? null);

  function openCreate(volumeId: number | null) {
    setEditor({ kind: "create", volumeId, beforeId: null, title: "" });
  }

  async function submitEditor(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!editor || editor.kind !== "create" || place.isPending) return;
    const title = editor.title.trim();
    if (!title) return;
    try {
      const created = await place.mutateAsync({
        novel_id: novelId,
        title,
        ...(editor.volumeId !== null
          ? { target_volume_id: editor.volumeId }
          : {}),
      });
      const key = groupKey(editor.volumeId);
      const itemCount =
        groups.find((group) => group.key === key)?.items.length ?? 0;
      setExpandedGroups((previous) => ({
        ...previous,
        [key]: { expanded: true, anchorId: created.id },
      }));
      setSelectedRanges((previous) => ({
        ...previous,
        [key]: {
          index: Math.floor(itemCount / SIDEBAR_BLOCK_SIZE),
          anchorId: created.id,
        },
      }));
      setEditor(null);
      onSelectChapter(created);
    } catch (error) {
      toastError(toErrorMessage(error));
    }
  }

  const loadError = chapterQuery.isError || volumeQuery.isError;
  const loading =
    novelId > 0 && (chapterQuery.isPending || volumeQuery.isPending);

  return (
    <>
      <div className="flex items-center justify-between border-b px-3 py-2.5">
        <span className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
          {t("sidebar.chaptersCount", { count: chapters.length })}
        </span>
        <div className="flex items-center gap-0.5">
          <button
            type="button"
            onClick={onExportNovel}
            aria-label={t("sidebar.export")}
            title={t("sidebar.export")}
            className="flex h-6 w-6 items-center justify-center rounded text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
          >
            <Download aria-hidden="true" className="h-3.5 w-3.5" />
          </button>
          <button
            type="button"
            onClick={() =>
              openCreate(
                activeChapter
                  ? (activeChapter.volume_id ?? null)
                  : (volumes[volumes.length - 1]?.id ?? null),
              )
            }
            disabled={novelId <= 0 || loadError || loading || place.isPending}
            aria-label={t("chapterManagement.createChapter")}
            title={t("chapterManagement.createChapter")}
            className="flex h-6 w-6 items-center justify-center rounded text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:opacity-50"
          >
            <Plus aria-hidden="true" className="h-4 w-4" />
          </button>
        </div>
      </div>

      {editor && (
        <ChapterCreateDialog
          editor={editor}
          volumes={volumes}
          targetItems={EMPTY_CHAPTERS}
          busy={place.isPending}
          showPosition={false}
          onChange={setEditor}
          onSubmit={(event) => void submitEditor(event)}
          onClose={() => setEditor(null)}
        />
      )}

      <button
        type="button"
        onClick={onSelectGoink}
        className={`relative flex w-full items-center gap-2.5 border-b border-border/50 px-3 py-1.5 text-left transition-colors hover:bg-muted/50 ${selectedPath === "goink.md" ? "bg-primary/10 font-medium" : ""}`}
      >
        {selectedPath === "goink.md" && (
          <span className="absolute left-0 top-1/2 h-5 w-0.5 -translate-y-1/2 rounded-r-full bg-primary" />
        )}
        <FileText
          aria-hidden="true"
          className="h-3.5 w-3.5 shrink-0 text-muted-foreground"
        />
        <span className="flex-1 truncate text-sm">
          {t("sidebar.storyStatus")}
        </span>
      </button>

      <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain">
        {novelId <= 0 ? (
          <p className="p-4 text-center text-xs text-muted-foreground">
            {t("chapterManagement.selectNovel")}
          </p>
        ) : loadError ? (
          <div className="flex h-full items-center justify-center">
            <div className="text-center">
              <FileText className="mx-auto mb-2 h-8 w-8 text-muted-foreground/30" />
              <p className="text-xs text-destructive">
                {t(
                  chapterQuery.isError
                    ? "chapter.loadFailed"
                    : "chapterManagement.loadFailed",
                )}
              </p>
              <button
                type="button"
                onClick={() => {
                  void chapterQuery.refetch();
                  void volumeQuery.refetch();
                }}
                className="mt-1 text-xs text-primary underline"
              >
                {t("common.retry")}
              </button>
            </div>
          </div>
        ) : loading ? (
          <p className="p-4 text-center text-xs text-muted-foreground">
            {t("chapterManagement.loading")}
          </p>
        ) : (
          groups.map((group) => {
            const anchorId =
              group.key === activeGroupKey ? activeChapterId : undefined;
            const defaultRange =
              group.key === activeGroupKey
                ? activeRangeIndex
                : Math.max(
                    0,
                    Math.ceil(group.items.length / SIDEBAR_BLOCK_SIZE) - 1,
                  );
            const expansion = expandedGroups[group.key];
            const expanded =
              expansion && expansion.anchorId === anchorId
                ? expansion.expanded
                : group.key === defaultGroupKey;
            const range = selectedRanges[group.key];
            return (
              <SidebarChapterGroup
                key={group.key}
                name={group.name}
                chapters={group.items}
                expanded={expanded}
                selectedPath={selectedPath}
                rangeIndex={
                  range && range.anchorId === anchorId
                    ? range.index
                    : defaultRange
                }
                busy={place.isPending}
                onToggle={() =>
                  setExpandedGroups((previous) => ({
                    ...previous,
                    [group.key]: { expanded: !expanded, anchorId },
                  }))
                }
                onRangeSelect={(index) =>
                  setSelectedRanges((previous) => ({
                    ...previous,
                    [group.key]: { index, anchorId },
                  }))
                }
                onCreate={() => openCreate(group.volumeId)}
                onSelectChapter={onSelectChapter}
                onRenameChapter={async (item, title) => {
                  await updateTitle.mutateAsync({ chapterID: item.id, title });
                }}
              />
            );
          })
        )}
      </div>
    </>
  );
}

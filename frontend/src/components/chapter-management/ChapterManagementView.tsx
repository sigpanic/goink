import { useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { EventsOn } from "@/lib/wailsjs/runtime/runtime";
import type { chapter } from "@/lib/wailsjs/go/models";
import { chapterKeys, contentKeys } from "@/lib/queryKeys";
import { useChapters } from "@/components/chapter/useChapters";
import { useVolumes } from "@/components/volume/useVolumes";
import VolumeRail from "@/components/volume/VolumeRail";
import { useEditorTabsStore } from "@/components/content/useEditorTabsStore";
import { toErrorMessage } from "@/utils/error";
import { toastError } from "@/utils/toast";
import ChapterGroup from "./ChapterGroup";
import ChapterEditorForm, { type ChapterEditor } from "./ChapterEditorForm";
import ChapterCreateDialog from "./ChapterCreateDialog";
import ChapterDeletionFeedback, {
  type BlockedChapterDeletion,
} from "./ChapterDeletionFeedback";
import { useChapterStructureMutations } from "./useChapterStructureMutations";
import { useChapterMoveFeedback } from "./useChapterMoveFeedback";

interface Props {
  novelId: number;
}

const EMPTY_CHAPTERS: chapter.Chapter[] = [];
const AUTO_EXPAND_LIMIT = 30;

export default function ChapterManagementView({ novelId }: Props) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const chapterQuery = useChapters(novelId);
  const volumeQuery = useVolumes(novelId);
  const { place, remove } = useChapterStructureMutations(novelId);
  const chapters = chapterQuery.data ?? EMPTY_CHAPTERS;
  const {
    containerRef,
    highlightedId,
    revealedId,
    capturePositions,
    cancelMove,
    showMovedChapter,
    dismissReveal,
  } = useChapterMoveFeedback(chapters);
  const [editor, setEditor] = useState<ChapterEditor | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<chapter.Chapter | null>(
    null,
  );
  const [blocked, setBlocked] = useState<BlockedChapterDeletion | null>(null);
  const [dragChapterId, setDragChapterId] = useState<number | null>(null);
  const [menuId, setMenuId] = useState<number | null>(null);
  const [groupExpanded, setGroupExpanded] = useState<Record<string, boolean>>(
    {},
  );
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

  const busy = place.isPending || remove.isPending;

  function groupKey(volumeId: number | null) {
    return `${novelId}:${volumeId ?? "unassigned"}`;
  }

  function groupItems(volumeId: number | null) {
    return volumeId === null
      ? groups.unassigned
      : (groups.byVolume.get(volumeId) ?? EMPTY_CHAPTERS);
  }

  function jumpToGroup(key: string) {
    setGroupExpanded((previous) => ({ ...previous, [key]: true }));
    sectionRefs.current.get(key)?.scrollIntoView?.({
      behavior: "smooth",
      block: "start",
    });
  }

  function openCreate(volumeId: number | null, beforeId: number | null = null) {
    setEditor({ kind: "create", volumeId, beforeId, title: "" });
  }

  function openMove(item: chapter.Chapter) {
    setEditor({
      kind: "move",
      sourceId: item.id,
      volumeId: item.volume_id ?? null,
      beforeId: null,
      title: "",
    });
  }

  async function placeChapter(
    sourceId: number,
    volumeId: number | null,
    beforeId: number | null,
  ): Promise<boolean> {
    setDragChapterId(null);
    if (busy || sourceId === beforeId) return false;
    const source = chapters.find((item) => item.id === sourceId);
    const targetItems = groupItems(volumeId);
    if (!source) return false;
    if (source.volume_id === volumeId) {
      const sourceIndex = targetItems.findIndex((item) => item.id === sourceId);
      const beforeIndex = targetItems.findIndex((item) => item.id === beforeId);
      if (
        beforeId === null
          ? sourceIndex === targetItems.length - 1
          : sourceIndex + 1 === beforeIndex
      )
        return true;
    }
    try {
      capturePositions();
      await place.mutateAsync({
        novel_id: novelId,
        source_chapter_id: sourceId,
        ...(volumeId !== null ? { target_volume_id: volumeId } : {}),
        ...(beforeId !== null ? { before_chapter_id: beforeId } : {}),
      });
      setGroupExpanded((previous) => ({
        ...previous,
        [groupKey(volumeId)]: true,
      }));
      showMovedChapter(sourceId);
      return true;
    } catch (error) {
      cancelMove();
      toastError(toErrorMessage(error));
      return false;
    }
  }

  async function submitEditor(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!editor || busy) return;
    if (editor.kind === "move") {
      if (editor.sourceId !== undefined) {
        if (
          await placeChapter(editor.sourceId, editor.volumeId, editor.beforeId)
        ) {
          setEditor(null);
        }
      }
      return;
    }
    const title = editor.title.trim();
    if (!title) return;
    try {
      await place.mutateAsync({
        novel_id: novelId,
        title,
        ...(editor.volumeId !== null
          ? { target_volume_id: editor.volumeId }
          : {}),
        ...(editor.beforeId !== null
          ? { before_chapter_id: editor.beforeId }
          : {}),
      });
      setGroupExpanded((previous) => ({
        ...previous,
        [groupKey(editor.volumeId)]: true,
      }));
      setEditor(null);
    } catch (error) {
      toastError(toErrorMessage(error));
    }
  }

  async function confirmDelete() {
    const item = deleteTarget;
    if (!item) return;
    try {
      const result = await remove.mutateAsync(item.id);
      setDeleteTarget(null);
      if (!result.deleted) {
        setBlocked({ item, references: result.references ?? [] });
        return;
      }
      const bodyPath = item.file_path;
      const draftPath = item.outline_file_path;
      const tabStore = useEditorTabsStore.getState();
      for (const tab of tabStore.byNovel[String(novelId)]?.tabs ?? []) {
        if (tab.path === bodyPath || tab.path === draftPath) {
          tabStore.closeTab(novelId, tab.id);
        }
      }
      qc.removeQueries({
        queryKey: contentKeys.detail(novelId, bodyPath),
        exact: true,
      });
      qc.removeQueries({
        queryKey: contentKeys.detail(novelId, draftPath),
        exact: true,
      });
    } catch (error) {
      toastError(toErrorMessage(error));
    }
  }

  function renderGroup(
    volumeId: number | null,
    name: string,
    items: chapter.Chapter[],
  ) {
    const key = groupKey(volumeId);
    return (
      <ChapterGroup
        key={key}
        groupKey={key}
        name={name}
        items={items}
        expanded={groupExpanded[key] ?? items.length <= AUTO_EXPAND_LIMIT}
        busy={busy}
        menuId={menuId}
        dragChapterId={dragChapterId}
        highlightedChapterId={highlightedId}
        revealedChapterId={revealedId}
        onDismissReveal={dismissReveal}
        sectionRef={(element) => {
          if (element) sectionRefs.current.set(key, element);
          else sectionRefs.current.delete(key);
        }}
        onToggle={() =>
          setGroupExpanded((previous) => ({
            ...previous,
            [key]: !(previous[key] ?? items.length <= AUTO_EXPAND_LIMIT),
          }))
        }
        onCreate={(beforeId) => openCreate(volumeId, beforeId)}
        onMove={openMove}
        onMenuChange={setMenuId}
        onDelete={(item) => {
          setBlocked(null);
          setDeleteTarget(item);
        }}
        onDragStart={setDragChapterId}
        onDragEnd={() => setDragChapterId(null)}
        onDrop={(beforeId) => {
          if (dragChapterId !== null)
            void placeChapter(dragChapterId, volumeId, beforeId);
        }}
      />
    );
  }

  const isLoading = chapterQuery.isPending || volumeQuery.isPending;
  const isError = chapterQuery.isError || volumeQuery.isError;

  return (
    <main
      ref={containerRef}
      className="flex min-h-0 min-w-0 flex-1 flex-col bg-background"
    >
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
              <VolumeRail
                novelId={novelId}
                volumes={volumes}
                chapterCounts={
                  new Map(
                    volumes.map((volume) => [
                      volume.id,
                      groupItems(volume.id).length,
                    ]),
                  )
                }
                onNavigate={(volumeId) => jumpToGroup(groupKey(volumeId))}
                dragChapterId={dragChapterId}
                onChapterDrop={(volumeId) => {
                  if (dragChapterId !== null)
                    void placeChapter(dragChapterId, volumeId, null);
                }}
              />
              {editor?.kind === "create" && (
                <ChapterCreateDialog
                  editor={editor}
                  volumes={volumes}
                  targetItems={groupItems(editor.volumeId)}
                  busy={busy}
                  showPosition
                  onChange={setEditor}
                  onSubmit={(event) => {
                    void submitEditor(event);
                  }}
                  onClose={() => setEditor(null)}
                />
              )}
              {editor?.kind === "move" && (
                <ChapterEditorForm
                  editor={editor}
                  volumes={volumes}
                  targetItems={groupItems(editor.volumeId)}
                  busy={busy}
                  onChange={setEditor}
                  onSubmit={(event) => {
                    void submitEditor(event);
                  }}
                  onClose={() => setEditor(null)}
                />
              )}
              <ChapterDeletionFeedback
                novelId={novelId}
                deleteTarget={deleteTarget}
                blocked={blocked}
                loading={remove.isPending}
                onConfirm={confirmDelete}
                onClose={() => {
                  if (!remove.isPending) setDeleteTarget(null);
                }}
                onDismissBlocked={() => setBlocked(null)}
              />
              {chapters.length === 0 && (
                <p className="text-sm text-muted-foreground">
                  {t("chapterManagement.noChapters")}
                </p>
              )}
              <div className="flex flex-col gap-5">
                {volumes.map((volume) =>
                  renderGroup(volume.id, volume.name, groupItems(volume.id)),
                )}
                {renderGroup(
                  null,
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

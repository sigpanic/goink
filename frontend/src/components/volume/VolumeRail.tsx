import { useState, type DragEvent, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { Check, GripVertical, Pencil, Plus, Trash2 } from "lucide-react";
import type { volume } from "@/lib/wailsjs/go/models";
import ConfirmDialog from "@/components/ui/ConfirmDialog";
import PopSelect from "@/components/shared/PopSelect";
import { useEditorTabsStore } from "@/components/content/useEditorTabsStore";
import { contentKeys } from "@/lib/queryKeys";
import { toErrorMessage } from "@/utils/error";
import { toastError } from "@/utils/toast";
import { useVolumeMutations } from "./useVolumeMutations";
import { useVolumeMoveFeedback } from "./useVolumeMoveFeedback";
import "./VolumeRailDrag.css";

interface Props {
  novelId: number;
  volumes: volume.Volume[];
  chapterCounts: Map<number, number>;
  onNavigate: (volumeId: number | null) => void;
  dragChapterId?: number | null;
  onChapterDrop?: (volumeId: number | null) => void;
}

type Editor =
  { kind: "create"; beforeId: number | null } | { kind: "rename"; id: number };
type DropEdge = "before" | "after";

export default function VolumeRail({
  novelId,
  volumes,
  chapterCounts,
  onNavigate,
  dragChapterId = null,
  onChapterDrop,
}: Props) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { place, rename, remove } = useVolumeMutations(novelId);
  const {
    railRef,
    highlightedId,
    capturePositions,
    cancelMove,
    showMovedVolume,
  } = useVolumeMoveFeedback(volumes);
  const [editor, setEditor] = useState<Editor | null>(null);
  const [name, setName] = useState("");
  const [deleteId, setDeleteId] = useState<number | null>(null);
  const [dragId, setDragId] = useState<number | null>(null);
  const [dropTarget, setDropTarget] = useState<
    { id: number; edge: DropEdge } | "end" | null
  >(null);
  const [chapterDropId, setChapterDropId] = useState<
    number | "unassigned" | null
  >(null);
  const busy = place.isPending || rename.isPending || remove.isPending;
  const deleteVolume = volumes.find((item) => item.id === deleteId);
  const deleteHasUnsaved =
    deleteVolume !== undefined &&
    (useEditorTabsStore.getState().byNovel[String(novelId)]?.tabs ?? []).some(
      (tab) =>
        tab.path === deleteVolume.outline_file_path &&
        (tab.isDirty || tab.outlineIsDirty),
    );

  function openCreate() {
    setName("");
    setEditor({ kind: "create", beforeId: null });
  }

  function openRename(item: volume.Volume) {
    setName(item.name);
    setEditor({ kind: "rename", id: item.id });
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const trimmed = name.trim();
    if (!trimmed || !editor || busy) return;
    try {
      if (editor.kind === "create") {
        await place.mutateAsync({
          novel_id: novelId,
          name: trimmed,
          ...(editor.beforeId !== null
            ? { before_volume_id: editor.beforeId }
            : {}),
        });
      } else {
        await rename.mutateAsync({ id: editor.id, name: trimmed });
        const outlinePath = volumes.find(
          (item) => item.id === editor.id,
        )?.outline_file_path;
        if (outlinePath) {
          const tabStore = useEditorTabsStore.getState();
          for (const tab of tabStore.byNovel[String(novelId)]?.tabs ?? []) {
            if (tab.type === "file" && tab.path === outlinePath) {
              tabStore.updateTab(novelId, tab.id, {
                title: t("sidebar.volumeOutlineTitle", { name: trimmed }),
              });
            }
          }
        }
      }
      setEditor(null);
    } catch (error) {
      toastError(toErrorMessage(error));
    }
  }

  function startDrag(event: DragEvent<HTMLButtonElement>, id: number) {
    if (busy) {
      event.preventDefault();
      return;
    }
    event.dataTransfer.effectAllowed = "move";
    event.dataTransfer.setData("text/plain", String(id));
    setDragId(id);
  }

  function dropEdgeFor(event: DragEvent<HTMLElement>): DropEdge {
    const rect = event.currentTarget.getBoundingClientRect();
    if (rect.width <= 0) return "before";
    return event.clientX < rect.left + rect.width / 2 ? "before" : "after";
  }

  function anchorFor(target: number | "end", edge: DropEdge): number | null {
    if (target === "end") return null;
    if (edge === "before") return target;
    const targetIndex = volumes.findIndex((item) => item.id === target);
    return volumes[targetIndex + 1]?.id ?? null;
  }

  function gapDropTarget(
    clientX: number,
  ): { id: number; edge: DropEdge } | "end" {
    for (const chip of railRef.current?.querySelectorAll<HTMLElement>(
      "[data-volume-id]",
    ) ?? []) {
      const rect = chip.getBoundingClientRect();
      if (clientX < rect.left + rect.width / 2) {
        return { id: Number(chip.dataset.volumeId), edge: "before" };
      }
    }
    return "end";
  }

  function dragStillInside(event: DragEvent<HTMLElement>) {
    if (
      event.relatedTarget instanceof Node &&
      event.currentTarget.contains(event.relatedTarget)
    )
      return true;
    const rect = event.currentTarget.getBoundingClientRect();
    return (
      rect.width > 0 &&
      rect.height > 0 &&
      event.clientX >= rect.left &&
      event.clientX <= rect.right &&
      event.clientY >= rect.top &&
      event.clientY <= rect.bottom
    );
  }

  function allowDrop(event: DragEvent<HTMLElement>, target: number | "end") {
    if (dragChapterId !== null && target !== "end") {
      event.preventDefault();
      event.dataTransfer.dropEffect = "move";
      setChapterDropId(target);
      return;
    }
    if (dragId === null || busy) return;
    event.preventDefault();
    event.dataTransfer.dropEffect = "move";
    if (target === "end") setDropTarget("end");
    else {
      const edge = dropEdgeFor(event);
      setDropTarget((previous) =>
        previous !== null &&
        previous !== "end" &&
        previous.id === target &&
        previous.edge === edge
          ? previous
          : { id: target, edge },
      );
    }
  }

  async function moveVolume(beforeId: number | null) {
    const source = dragId;
    setDragId(null);
    setDropTarget(null);
    if (source === null || busy) return;
    const sourceIndex = volumes.findIndex((item) => item.id === source);
    if (
      sourceIndex < 0 ||
      (beforeId !== null && !volumes.some((item) => item.id === beforeId))
    )
      return;
    const beforeIndex = volumes.findIndex((item) => item.id === beforeId);
    if (
      beforeId === null
        ? sourceIndex === volumes.length - 1
        : beforeId === source || sourceIndex + 1 === beforeIndex
    )
      return;
    try {
      capturePositions();
      await place.mutateAsync({
        novel_id: novelId,
        source_volume_id: source,
        ...(beforeId !== null ? { before_volume_id: beforeId } : {}),
      });
      showMovedVolume(source);
    } catch (error) {
      cancelMove();
      toastError(toErrorMessage(error));
    }
  }

  function move(event: DragEvent<HTMLElement>, target: number | "end") {
    event.preventDefault();
    if (dragChapterId !== null && target !== "end") {
      setChapterDropId(null);
      onChapterDrop?.(target);
      return;
    }
    const beforeId = anchorFor(
      target,
      target === "end" ? "after" : dropEdgeFor(event),
    );
    void moveVolume(beforeId);
  }

  function dragOverRailGap(event: DragEvent<HTMLDivElement>) {
    if (event.target !== event.currentTarget || dragId === null || busy) return;
    event.preventDefault();
    event.dataTransfer.dropEffect = "move";
    setDropTarget(gapDropTarget(event.clientX));
  }

  function dropOnRailGap(event: DragEvent<HTMLDivElement>) {
    if (event.target !== event.currentTarget || dragId === null) return;
    event.preventDefault();
    const target = gapDropTarget(event.clientX);
    void moveVolume(target === "end" ? null : target.id);
  }

  function askDelete(id: number) {
    if ((chapterCounts.get(id) ?? 0) > 0) {
      toastError(t("chapterManagement.volumeNotEmpty"));
      return;
    }
    setDeleteId(id);
  }

  async function confirmDelete() {
    if (deleteId === null) return;
    try {
      await remove.mutateAsync(deleteId);
      const outlinePath = deleteVolume?.outline_file_path;
      if (outlinePath) {
        const tabStore = useEditorTabsStore.getState();
        for (const tab of tabStore.byNovel[String(novelId)]?.tabs ?? []) {
          if (tab.path === outlinePath) tabStore.closeTab(novelId, tab.id);
        }
        qc.removeQueries({
          queryKey: contentKeys.detail(novelId, outlinePath),
          exact: true,
        });
      }
      setDeleteId(null);
    } catch (error) {
      toastError(toErrorMessage(error));
    }
  }

  return (
    <section
      aria-label={t("chapterManagement.volumes")}
      className="sticky top-0 z-10 bg-background py-2"
    >
      <div className="mb-3 flex items-center justify-between gap-3">
        <h2 className="text-sm font-medium text-muted-foreground">
          {t("chapterManagement.volumes")}
        </h2>
        <button
          type="button"
          onClick={openCreate}
          disabled={busy}
          className="flex items-center gap-1 rounded-md border px-2 py-1 text-sm hover:bg-muted disabled:opacity-50"
        >
          <Plus aria-hidden="true" className="h-4 w-4" />
          {t("chapterManagement.createVolume")}
        </button>
      </div>
      {volumes.length === 0 && (
        <p className="mb-3 text-sm text-muted-foreground">
          {t("chapterManagement.noVolumes")}
        </p>
      )}
      <div
        ref={railRef}
        onDragOver={dragOverRailGap}
        onDrop={dropOnRailGap}
        className="flex gap-3 overflow-x-auto px-1 py-2"
      >
        {volumes.map((item) => (
          <div
            key={item.id}
            data-volume-id={item.id}
            onDragOver={(event) => allowDrop(event, item.id)}
            onDrop={(event) => move(event, item.id)}
            onDragLeave={(event) => {
              if (dragStillInside(event)) return;
              setDropTarget(null);
              setChapterDropId(null);
            }}
            className={`volume-chip flex shrink-0 items-center rounded-lg border bg-card ${dragId === item.id ? "volume-chip-dragging" : ""} ${dropTarget !== null && dropTarget !== "end" && dropTarget.id === item.id ? `volume-chip-drop-${dropTarget.edge}` : ""} ${chapterDropId === item.id ? "border-primary ring-2 ring-primary/40" : ""} ${highlightedId === item.id ? "volume-chip-moved" : ""}`}
          >
            <button
              type="button"
              draggable={!busy}
              onDragStart={(event) => startDrag(event, item.id)}
              onDragEnd={() => {
                setDragId(null);
                setDropTarget(null);
              }}
              aria-label={t("chapterManagement.dragVolume", {
                name: item.name,
              })}
              title={t("chapterManagement.dragVolume", { name: item.name })}
              className="cursor-grab p-2 text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              {highlightedId === item.id ? (
                <Check aria-hidden="true" className="h-4 w-4 text-primary" />
              ) : (
                <GripVertical aria-hidden="true" className="h-4 w-4" />
              )}
            </button>
            {highlightedId === item.id && (
              <span role="status" className="sr-only">
                {t("chapterManagement.volumeMoveSuccess", { name: item.name })}
              </span>
            )}
            <button
              type="button"
              onClick={() => onNavigate(item.id)}
              className="max-w-48 truncate py-2 text-sm hover:text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              title={item.name}
            >
              {item.name}
            </button>
            <button
              type="button"
              onClick={() => openRename(item)}
              disabled={busy}
              aria-label={t("chapterManagement.renameVolume", {
                name: item.name,
              })}
              className="p-2 text-muted-foreground hover:text-foreground disabled:opacity-50"
            >
              <Pencil aria-hidden="true" className="h-3.5 w-3.5" />
            </button>
            <button
              type="button"
              onClick={() => askDelete(item.id)}
              disabled={busy}
              aria-label={t("chapterManagement.deleteVolume", {
                name: item.name,
              })}
              className="p-2 text-muted-foreground hover:text-destructive disabled:opacity-50"
            >
              <Trash2 aria-hidden="true" className="h-3.5 w-3.5" />
            </button>
          </div>
        ))}
        {volumes.length > 0 && (
          <div
            onDragOver={(event) => allowDrop(event, "end")}
            onDrop={(event) => move(event, "end")}
            onDragLeave={() => setDropTarget(null)}
            className={`min-w-28 shrink-0 rounded-lg border border-dashed px-3 py-2 text-center text-sm text-muted-foreground ${dropTarget === "end" ? "border-primary bg-primary/10 ring-2 ring-primary/30" : ""}`}
          >
            {t("chapterManagement.moveToEnd")}
          </div>
        )}
        <button
          type="button"
          onClick={() => onNavigate(null)}
          onDragOver={(event) => {
            if (dragChapterId === null) return;
            event.preventDefault();
            event.dataTransfer.dropEffect = "move";
            setChapterDropId("unassigned");
          }}
          onDrop={(event) => {
            if (dragChapterId === null) return;
            event.preventDefault();
            setChapterDropId(null);
            onChapterDrop?.(null);
          }}
          onDragLeave={() => setChapterDropId(null)}
          className={`shrink-0 rounded-lg border border-dashed px-3 py-2 text-sm text-muted-foreground hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring ${chapterDropId === "unassigned" ? "border-primary bg-primary/10" : ""}`}
        >
          {t("chapterManagement.unassigned")}
        </button>
      </div>
      {editor && (
        <form
          onSubmit={(event) => void submit(event)}
          className="mt-3 flex flex-wrap items-center gap-2 rounded-lg border bg-card p-3"
        >
          <label htmlFor="volume-name" className="text-sm">
            {t("chapterManagement.volumeName")}
          </label>
          <input
            id="volume-name"
            autoFocus
            value={name}
            onChange={(event) => setName(event.target.value)}
            maxLength={100}
            disabled={busy}
            className="min-w-40 flex-1 rounded-md border bg-background px-2 py-1.5 text-sm"
          />
          {editor.kind === "create" && (
            <div className="flex min-w-48 flex-1 items-center gap-2 text-sm">
              <span>{t("chapterManagement.volumePosition")}</span>
              <PopSelect
                ariaLabel={t("chapterManagement.volumePosition")}
                size="form"
                dropUp={false}
                className="min-w-40 flex-1"
                minWidth="0"
                value={String(editor.beforeId ?? "end")}
                options={[
                  { value: "end", label: t("chapterManagement.atVolumeEnd") },
                  ...volumes.map((item) => ({
                    value: String(item.id),
                    label: t("chapterManagement.beforeVolume", {
                      name: item.name,
                    }),
                  })),
                ]}
                onChange={(value) =>
                  setEditor({
                    kind: "create",
                    beforeId: value === "end" ? null : Number(value),
                  })
                }
                disabled={busy}
              />
            </div>
          )}
          <button
            type="submit"
            disabled={!name.trim() || busy}
            className="rounded-md bg-primary px-3 py-1.5 text-sm text-primary-foreground disabled:opacity-50"
          >
            {t("common.save")}
          </button>
          <button
            type="button"
            onClick={() => setEditor(null)}
            disabled={busy}
            className="rounded-md border px-3 py-1.5 text-sm disabled:opacity-50"
          >
            {t("common.cancel")}
          </button>
        </form>
      )}
      <ConfirmDialog
        open={deleteId !== null}
        title={t("chapterManagement.deleteVolume", {
          name: deleteVolume?.name ?? "",
        })}
        message={t(
          deleteHasUnsaved
            ? "chapterManagement.deleteVolumeConfirmUnsaved"
            : "chapterManagement.deleteVolumeConfirm",
          {
            name: deleteVolume?.name ?? "",
          },
        )}
        danger
        loading={remove.isPending}
        onConfirm={confirmDelete}
        onClose={() => {
          if (!remove.isPending) setDeleteId(null);
        }}
      />
    </section>
  );
}

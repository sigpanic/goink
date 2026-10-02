import { useState, type DragEvent, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { GripVertical, Pencil, Plus, Trash2 } from "lucide-react";
import type { volume } from "@/lib/wailsjs/go/models";
import ConfirmDialog from "@/components/ui/ConfirmDialog";
import { toErrorMessage } from "@/utils/error";
import { toastError } from "@/utils/toast";
import { useVolumeMutations } from "./useVolumeMutations";

interface Props {
  novelId: number;
  volumes: volume.Volume[];
  chapterCounts: Map<number, number>;
  onNavigate: (volumeId: number | null) => void;
}

type Editor =
  { kind: "create"; beforeId: number | null } | { kind: "rename"; id: number };

export default function VolumeRail({
  novelId,
  volumes,
  chapterCounts,
  onNavigate,
}: Props) {
  const { t } = useTranslation();
  const { place, rename, remove } = useVolumeMutations(novelId);
  const [editor, setEditor] = useState<Editor | null>(null);
  const [name, setName] = useState("");
  const [deleteId, setDeleteId] = useState<number | null>(null);
  const [dragId, setDragId] = useState<number | null>(null);
  const [dropId, setDropId] = useState<number | "end" | null>(null);
  const busy = place.isPending || rename.isPending || remove.isPending;
  const deleteVolume = volumes.find((item) => item.id === deleteId);

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

  function allowDrop(event: DragEvent<HTMLElement>, target: number | "end") {
    if (dragId === null || dragId === target || busy) return;
    event.preventDefault();
    event.dataTransfer.dropEffect = "move";
    setDropId(target);
  }

  async function move(event: DragEvent<HTMLElement>, target: number | "end") {
    event.preventDefault();
    const source = dragId;
    setDragId(null);
    setDropId(null);
    if (source === null || source === target || busy) return;
    const sourceIndex = volumes.findIndex((item) => item.id === source);
    const targetIndex = volumes.findIndex((item) => item.id === target);
    if (sourceIndex < 0 || (target !== "end" && targetIndex < 0)) return;
    if (
      target === "end"
        ? sourceIndex === volumes.length - 1
        : sourceIndex + 1 === targetIndex
    )
      return;
    try {
      await place.mutateAsync({
        novel_id: novelId,
        source_volume_id: source,
        ...(target !== "end" ? { before_volume_id: target } : {}),
      });
    } catch (error) {
      toastError(toErrorMessage(error));
    }
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
      setDeleteId(null);
    } catch (error) {
      toastError(toErrorMessage(error));
    }
  }

  return (
    <section
      aria-label={t("chapterManagement.volumes")}
      className="sticky top-0 z-10 rounded-lg bg-background py-2"
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
      <div className="flex gap-2 overflow-x-auto pb-1">
        {volumes.map((item) => (
          <div
            key={item.id}
            onDragOver={(event) => allowDrop(event, item.id)}
            onDrop={(event) => void move(event, item.id)}
            onDragLeave={() => setDropId(null)}
            className={`flex shrink-0 items-center rounded-lg border bg-card ${dropId === item.id ? "border-primary ring-2 ring-primary/40" : ""}`}
          >
            <button
              type="button"
              draggable={!busy}
              onDragStart={(event) => startDrag(event, item.id)}
              onDragEnd={() => {
                setDragId(null);
                setDropId(null);
              }}
              aria-label={t("chapterManagement.dragVolume", {
                name: item.name,
              })}
              title={t("chapterManagement.dragVolume", { name: item.name })}
              className="cursor-grab p-2 text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              <GripVertical aria-hidden="true" className="h-4 w-4" />
            </button>
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
            onDrop={(event) => void move(event, "end")}
            onDragLeave={() => setDropId(null)}
            className={`shrink-0 rounded-lg border border-dashed px-3 py-2 text-sm text-muted-foreground ${dropId === "end" ? "border-primary bg-primary/10" : ""}`}
          >
            {t("chapterManagement.moveToEnd")}
          </div>
        )}
        <button
          type="button"
          onClick={() => onNavigate(null)}
          className="shrink-0 rounded-lg border border-dashed px-3 py-2 text-sm text-muted-foreground hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
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
            <label className="flex items-center gap-2 text-sm">
              {t("chapterManagement.insertBefore")}
              <select
                value={editor.beforeId ?? "end"}
                onChange={(event) =>
                  setEditor({
                    kind: "create",
                    beforeId:
                      event.target.value === "end"
                        ? null
                        : Number(event.target.value),
                  })
                }
                disabled={busy}
                className="rounded-md border bg-background px-2 py-1.5"
              >
                <option value="end">{t("chapterManagement.atEnd")}</option>
                {volumes.map((item) => (
                  <option key={item.id} value={item.id}>
                    {item.name}
                  </option>
                ))}
              </select>
            </label>
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
        message={t("chapterManagement.deleteVolumeConfirm", {
          name: deleteVolume?.name ?? "",
        })}
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

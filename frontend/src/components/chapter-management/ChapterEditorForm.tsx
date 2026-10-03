import type { FormEvent } from "react";
import { useTranslation } from "react-i18next";
import type { chapter, volume } from "@/lib/wailsjs/go/models";
import PopSelect from "@/components/shared/PopSelect";

export type ChapterEditor = {
  kind: "create" | "move";
  sourceId?: number;
  volumeId: number | null;
  beforeId: number | null;
  title: string;
};

interface Props {
  editor: ChapterEditor;
  volumes: volume.Volume[];
  targetItems: chapter.Chapter[];
  busy: boolean;
  dialog?: boolean;
  showPosition?: boolean;
  onChange: (editor: ChapterEditor) => void;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
  onClose: () => void;
}

export default function ChapterEditorForm({
  editor,
  volumes,
  targetItems,
  busy,
  dialog = false,
  showPosition = true,
  onChange,
  onSubmit,
  onClose,
}: Props) {
  const { t } = useTranslation();

  return (
    <form
      onSubmit={onSubmit}
      className={
        dialog
          ? "flex flex-col gap-4"
          : "flex flex-wrap items-end gap-3 rounded-lg border bg-card p-4"
      }
    >
      <h2
        id={dialog ? "chapter-create-dialog-title" : undefined}
        className={
          dialog ? "text-base font-semibold" : "w-full text-sm font-medium"
        }
      >
        {t(
          editor.kind === "create"
            ? "chapterManagement.createChapter"
            : "chapterManagement.moveChapter",
        )}
      </h2>
      {editor.kind === "create" && (
        <label
          className={`flex flex-col gap-1 text-sm ${dialog ? "w-full" : "min-w-40 flex-1"}`}
        >
          {t("chapterManagement.chapterTitle")}
          <input
            autoFocus
            value={editor.title}
            onChange={(event) =>
              onChange({ ...editor, title: event.target.value })
            }
            disabled={busy}
            className="rounded-md border bg-background px-2 py-1.5"
          />
        </label>
      )}
      <div
        className={`flex flex-col gap-1 text-sm ${dialog ? "w-full" : "min-w-40 flex-1"}`}
      >
        <span>{t("chapterManagement.targetVolume")}</span>
        <PopSelect
          ariaLabel={t("chapterManagement.targetVolume")}
          size="form"
          dropUp={false}
          className="w-full"
          minWidth="0"
          value={String(editor.volumeId ?? "unassigned")}
          options={[
            ...volumes.map((item) => ({
              value: String(item.id),
              label: item.name,
            })),
            { value: "unassigned", label: t("chapterManagement.unassigned") },
          ]}
          onChange={(value) =>
            onChange({
              ...editor,
              volumeId: value === "unassigned" ? null : Number(value),
              beforeId: null,
            })
          }
          disabled={busy}
        />
      </div>
      {showPosition && (
        <div
          className={`flex flex-col gap-1 text-sm ${dialog ? "w-full" : "min-w-48 flex-1"}`}
        >
          <span>{t("chapterManagement.chapterPosition")}</span>
          <PopSelect
            ariaLabel={t("chapterManagement.chapterPosition")}
            size="form"
            dropUp={false}
            className="w-full"
            minWidth="0"
            value={String(editor.beforeId ?? "end")}
            options={[
              { value: "end", label: t("chapterManagement.atGroupEnd") },
              ...targetItems
                .filter((item) => item.id !== editor.sourceId)
                .map((item) => ({
                  value: String(item.id),
                  label: t("chapterManagement.beforeChapter", {
                    n: item.reading_number,
                    title: item.title,
                  }),
                })),
            ]}
            onChange={(value) =>
              onChange({
                ...editor,
                beforeId: value === "end" ? null : Number(value),
              })
            }
            disabled={busy}
          />
        </div>
      )}
      {!showPosition && (
        <p className="text-xs text-muted-foreground">
          {t("sidebar.addAtGroupEnd")}
        </p>
      )}
      <div className={dialog ? "flex justify-end gap-2" : "contents"}>
        <button
          type="submit"
          disabled={busy || (editor.kind === "create" && !editor.title.trim())}
          className="rounded-md bg-primary px-3 py-1.5 text-sm text-primary-foreground disabled:opacity-50"
        >
          {t(showPosition ? "common.save" : "sidebar.add")}
        </button>
        <button
          type="button"
          onClick={onClose}
          disabled={busy}
          className="rounded-md border px-3 py-1.5 text-sm disabled:opacity-50"
        >
          {t("common.cancel")}
        </button>
      </div>
    </form>
  );
}

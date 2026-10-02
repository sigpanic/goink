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
  onChange: (editor: ChapterEditor) => void;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
  onClose: () => void;
}

export default function ChapterEditorForm({
  editor,
  volumes,
  targetItems,
  busy,
  onChange,
  onSubmit,
  onClose,
}: Props) {
  const { t } = useTranslation();

  return (
    <form
      onSubmit={onSubmit}
      className="flex flex-wrap items-end gap-3 rounded-lg border bg-card p-4"
    >
      <h2 className="w-full text-sm font-medium">
        {t(
          editor.kind === "create"
            ? "chapterManagement.createChapter"
            : "chapterManagement.moveChapter",
        )}
      </h2>
      {editor.kind === "create" && (
        <label className="flex min-w-40 flex-1 flex-col gap-1 text-sm">
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
      <div className="flex min-w-40 flex-1 flex-col gap-1 text-sm">
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
      <div className="flex min-w-48 flex-1 flex-col gap-1 text-sm">
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
      <button
        type="submit"
        disabled={busy || (editor.kind === "create" && !editor.title.trim())}
        className="rounded-md bg-primary px-3 py-1.5 text-sm text-primary-foreground disabled:opacity-50"
      >
        {t("common.save")}
      </button>
      <button
        type="button"
        onClick={onClose}
        disabled={busy}
        className="rounded-md border px-3 py-1.5 text-sm disabled:opacity-50"
      >
        {t("common.cancel")}
      </button>
    </form>
  );
}

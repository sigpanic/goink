import { useState, useEffect } from "react";
import { useTranslation } from "react-i18next";
import type { novel } from "@/hooks/useApp";

interface Props {
  open: boolean;
  novel?: novel.Novel | null; // 传了=编辑，不传=创建
  onClose: () => void;
  onSave: (input: {
    title: string;
    description: string;
    genre: string;
  }) => Promise<void>;
}

export default function NovelEditDialog({
  open,
  novel,
  onClose,
  onSave,
}: Props) {
  const { t } = useTranslation();
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [genre, setGenre] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  const GENRE_PRESETS = [
    t("novel.genreFantasy"),
    t("novel.genreSciFi"),
    t("novel.genreUrban"),
    t("novel.genreHistory"),
    t("novel.genreMystery"),
    t("novel.genreWuxia"),
    t("novel.genreRomance"),
    t("novel.genreOther"),
  ];

  useEffect(() => {
    if (open) {
      setTitle(novel?.title ?? "");
      setDescription(novel?.description ?? "");
      setGenre(novel?.genre ?? "");
      setSaving(false);
      setError("");
    }
  }, [open, novel]);

  if (!open) return null;

  const isEdit = !!novel;
  const canSave = isEdit ? true : title.trim().length > 0;

  async function handleSave() {
    if (!canSave || saving) return;
    setSaving(true);
    setError("");
    try {
      await onSave({
        title: title.trim(),
        description: description.trim(),
        genre: genre.trim(),
      });
    } catch (e: any) {
      setError(e?.message ?? t("novel.saveFailedRetry"));
    } finally {
      setSaving(false);
    }
  }

  function handleKeyDown(e: React.KeyboardEvent) {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      handleSave();
    }
    if (e.key === "Escape") onClose();
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-black/40" onClick={onClose} />
      <div
        className="relative bg-background rounded-xl shadow-2xl border w-[420px] max-w-[90vw] p-6"
        onKeyDown={handleKeyDown}
      >
        <button
          onClick={onClose}
          className="absolute top-3 right-3 w-7 h-7 flex items-center justify-center rounded-md text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
        >
          ✕
        </button>

        <h2 className="text-base font-semibold mb-5">
          {isEdit ? t("novel.editWork") : t("novel.newWork")}
        </h2>

        {error && (
          <p className="text-sm text-red-600 bg-danger-bg border border-danger-border rounded-md px-3 py-2 mb-4">
            {error}
          </p>
        )}

        <div className="space-y-4">
          <div>
            <label className="block text-xs font-medium text-muted-foreground mb-1.5">
              {t("novel.bookTitle")}{" "}
              {!isEdit && <span className="text-red-500">*</span>}
            </label>
            <input
              type="text"
              value={title}
              autoFocus
              onChange={(e) => setTitle(e.target.value)}
              placeholder={t("novel.enterBookTitle")}
              className="w-full h-9 rounded-md border bg-background px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
          </div>

          <div>
            <label className="block text-xs font-medium text-muted-foreground mb-1.5">
              {t("novel.genre")}
            </label>
            <input
              type="text"
              value={genre}
              onChange={(e) => setGenre(e.target.value)}
              placeholder={t("novel.genreExample")}
              list="genre-suggestions"
              className="w-full h-9 rounded-md border bg-background px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
            <datalist id="genre-suggestions">
              {GENRE_PRESETS.map((g) => (
                <option key={g} value={g} />
              ))}
            </datalist>
          </div>

          <div>
            <label className="block text-xs font-medium text-muted-foreground mb-1.5">
              {t("novel.summary")}
            </label>
            <textarea
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder={t("novel.summaryPlaceholder")}
              rows={3}
              className="w-full rounded-md border bg-background px-3 py-2 text-sm resize-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
          </div>
        </div>

        <div className="flex justify-end gap-2 mt-6">
          <button
            onClick={onClose}
            className="h-9 px-4 rounded-md text-sm border hover:bg-muted transition-colors"
          >
            {t("common.cancel")}
          </button>
          <button
            onClick={handleSave}
            disabled={!canSave || saving}
            className="h-9 px-4 rounded-md text-sm bg-primary text-primary-foreground hover:opacity-90 transition-opacity disabled:opacity-50"
          >
            {saving ? t("common.saving") : t("common.save")}
          </button>
        </div>
      </div>
    </div>
  );
}

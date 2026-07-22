import { useState, useEffect } from "react";
import { useTranslation } from "react-i18next";

interface Props {
  open: boolean;
  novelTitle: string;
  onClose: () => void;
  onConfirm: () => Promise<void>;
}

export default function NovelDeleteDialog({
  open,
  novelTitle,
  onClose,
  onConfirm,
}: Props) {
  const { t } = useTranslation();
  const [confirmText, setConfirmText] = useState("");
  const [deleting, setDeleting] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (open) {
      setConfirmText("");
      setDeleting(false);
      setError("");
    }
  }, [open]);

  if (!open) return null;

  const canDelete = confirmText === novelTitle;

  async function handleDelete() {
    if (!canDelete || deleting) return;
    setDeleting(true);
    setError("");
    try {
      await onConfirm();
    } catch (e: any) {
      setError(e?.message ?? t("novel.deleteFailedRetry"));
    } finally {
      setDeleting(false);
    }
  }

  function handleKeyDown(e: React.KeyboardEvent) {
    if (e.key === "Escape") onClose();
    if (e.key === "Enter" && canDelete) handleDelete();
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center"
      onKeyDown={handleKeyDown}
    >
      <div className="absolute inset-0 bg-black/40" onClick={onClose} />
      <div className="relative bg-background rounded-xl shadow-2xl border w-[420px] max-w-[90vw] p-6">
        <button
          onClick={onClose}
          className="absolute top-3 right-3 w-7 h-7 flex items-center justify-center rounded-md text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
        >
          ✕
        </button>

        <h2 className="text-base font-semibold text-destructive mb-3">
          {t("novel.deleteWork")}
        </h2>

        {error && (
          <p className="text-sm text-red-600 bg-danger-bg border border-danger-border rounded-md px-3 py-2 mb-3">
            {error}
          </p>
        )}

        <p className="text-sm text-muted-foreground mb-1">
          {t("novel.deleteWarning", {
            permanentLoss: t("novel.permanentLoss"),
          })}
        </p>
        <p className="text-sm text-muted-foreground mb-4">
          {t("novel.pleaseEnterTitle")}{" "}
          <b className="text-foreground">{novelTitle}</b>{" "}
          {t("novel.confirmDelete2")}：
        </p>

        <input
          type="text"
          value={confirmText}
          autoFocus
          onChange={(e) => setConfirmText(e.target.value)}
          placeholder={t("novel.enterTitleToConfirm")}
          className="w-full h-9 rounded-md border bg-background px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring mb-5"
        />

        <div className="flex justify-end gap-2">
          <button
            onClick={onClose}
            className="h-9 px-4 rounded-md text-sm border hover:bg-muted transition-colors"
          >
            {t("common.cancel")}
          </button>
          <button
            onClick={handleDelete}
            disabled={!canDelete || deleting}
            className="h-9 px-4 rounded-md text-sm bg-destructive text-destructive-foreground hover:bg-destructive/85 transition-colors disabled:opacity-50"
          >
            {deleting ? t("common.deleting") : t("novel.confirmDelete2")}
          </button>
        </div>
      </div>
    </div>
  );
}

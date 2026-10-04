import { useCallback, useEffect, useState } from "react";
import { createPortal } from "react-dom";
import { DiffEditor } from "@monaco-editor/react";
import { Loader2, X } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useThemeStore } from "@/stores/useThemeStore";
import { toErrorMessage } from "@/utils/error";
import type {
  ConflictChoiceResult,
  ConflictSnapshot,
} from "./useEditorFileSync";

interface Props {
  tabId: string;
  path: string;
  outline: boolean;
  onLoad: (
    tabId: string,
    path: string,
    outline: boolean,
  ) => Promise<ConflictSnapshot>;
  onChoose: (
    tabId: string,
    path: string,
    outline: boolean,
    choice: "disk" | "local",
    shown: ConflictSnapshot,
  ) => Promise<ConflictChoiceResult>;
}

export default function ContentConflictNotice({
  tabId,
  path,
  outline,
  onLoad,
  onChoose,
}: Props) {
  const { t } = useTranslation();
  const { theme } = useThemeStore();
  const [open, setOpen] = useState(true);
  const [snapshot, setSnapshot] = useState<ConflictSnapshot | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [changed, setChanged] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      setSnapshot(await onLoad(tabId, path, outline));
      setChanged(false);
    } catch (cause) {
      setError(toErrorMessage(cause));
    } finally {
      setLoading(false);
    }
  }, [onLoad, tabId, path, outline]);

  useEffect(() => {
    let active = true;
    void onLoad(tabId, path, outline)
      .then((value) => {
        if (active) setSnapshot(value);
      })
      .catch((cause) => {
        if (active) setError(toErrorMessage(cause));
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [onLoad, tabId, path, outline]);

  const choose = async (choice: "disk" | "local") => {
    if (!snapshot || busy) return;
    setBusy(true);
    setError(null);
    try {
      const result = await onChoose(tabId, path, outline, choice, snapshot);
      if (result.status === "changed") {
        setSnapshot(result.snapshot);
        setChanged(true);
        return;
      }
      setOpen(false);
    } catch (cause) {
      setError(toErrorMessage(cause));
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <div
        role="alert"
        className="flex flex-wrap items-center gap-2 border-b border-amber-500/30 bg-amber-500/10 px-4 py-2 text-sm"
      >
        <span className="mr-auto">{t("content.conflictNotice")}</span>
        {!open && (
          <button
            className="rounded border px-2 py-1 hover:bg-muted"
            onClick={() => {
              setOpen(true);
              void load();
            }}
          >
            {t("content.conflictReview")}
          </button>
        )}
      </div>
      {open &&
        createPortal(
          <div
            className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-3 sm:p-6"
            role="dialog"
            aria-modal="true"
            aria-label={t("content.conflictTitle")}
          >
            <div className="flex h-[min(90vh,900px)] w-[min(96vw,1500px)] min-h-0 flex-col overflow-hidden rounded-xl border bg-background shadow-2xl">
              <div className="flex items-start gap-4 border-b px-5 py-4">
                <div className="min-w-0 flex-1">
                  <h2 className="text-base font-semibold">
                    {t("content.conflictTitle")}
                  </h2>
                  <p
                    className="mt-1 truncate text-xs text-muted-foreground"
                    title={path}
                  >
                    {path}
                  </p>
                  <p className="mt-2 text-sm text-muted-foreground">
                    {t("content.conflictDiffHint")}
                  </p>
                </div>
                <button
                  type="button"
                  aria-label={t("content.conflictLater")}
                  className="rounded p-1 text-muted-foreground hover:bg-muted"
                  onClick={() => setOpen(false)}
                  disabled={busy}
                >
                  <X className="h-5 w-5" />
                </button>
              </div>
              {changed && (
                <p
                  role="status"
                  className="border-b bg-amber-500/10 px-5 py-2 text-sm text-amber-700 dark:text-amber-300"
                >
                  {t("content.conflictChangedAgain")}
                </p>
              )}
              {error && (
                <p
                  role="alert"
                  className="border-b bg-destructive/10 px-5 py-2 text-sm text-destructive"
                >
                  {error}
                </p>
              )}
              <div className="grid grid-cols-2 border-b bg-muted/30 text-xs font-medium">
                <span className="border-r px-4 py-2">
                  {t("content.conflictDiskVersion")}
                </span>
                <span className="px-4 py-2">
                  {t("content.conflictLocalVersion")}
                </span>
              </div>
              <div className="min-h-0 flex-1">
                {loading ? (
                  <div className="flex h-full items-center justify-center gap-2 text-sm text-muted-foreground">
                    <Loader2 className="h-4 w-4 animate-spin" />
                    {t("content.conflictLoading")}
                  </div>
                ) : snapshot ? (
                  <DiffEditor
                    height="100%"
                    language="markdown"
                    theme={theme === "dark" ? "vs-dark" : "light"}
                    original={snapshot.diskContent}
                    modified={snapshot.localContent}
                    options={{
                      readOnly: true,
                      originalEditable: false,
                      renderSideBySide: true,
                      useInlineViewWhenSpaceIsLimited: false,
                      automaticLayout: true,
                      wordWrap: "on",
                      minimap: { enabled: false },
                    }}
                  />
                ) : (
                  <div className="flex h-full items-center justify-center">
                    <button
                      type="button"
                      className="rounded border px-3 py-2 text-sm hover:bg-muted"
                      onClick={() => void load()}
                    >
                      {t("content.conflictRetry")}
                    </button>
                  </div>
                )}
              </div>
              <div className="flex flex-wrap items-center justify-end gap-2 border-t px-5 py-3">
                <button
                  type="button"
                  className="rounded border px-3 py-2 text-sm hover:bg-muted"
                  onClick={() => setOpen(false)}
                  disabled={busy}
                >
                  {t("content.conflictLater")}
                </button>
                <button
                  type="button"
                  className="rounded border px-3 py-2 text-sm hover:bg-muted disabled:opacity-50"
                  onClick={() => void choose("disk")}
                  disabled={!snapshot || busy || loading}
                >
                  {t("content.conflictUseDisk")}
                </button>
                <button
                  type="button"
                  className="rounded bg-primary px-3 py-2 text-sm text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
                  onClick={() => void choose("local")}
                  disabled={!snapshot || busy || loading}
                >
                  {busy && (
                    <Loader2 className="mr-1 inline h-3.5 w-3.5 animate-spin" />
                  )}
                  {t("content.conflictSaveLocal")}
                </button>
              </div>
            </div>
          </div>,
          document.body,
        )}
    </>
  );
}

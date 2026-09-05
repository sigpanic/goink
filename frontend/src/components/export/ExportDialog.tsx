import { useState } from "react";
import { BookOpen, FileText, AlignLeft } from "lucide-react";
import { useTranslation } from "react-i18next";
import { toErrorMessage } from "@/utils/error";

interface Props {
  novelTitle: string;
  onClose: () => void;
  onExport: (format: "epub" | "markdown" | "txt") => Promise<void>;
}

const FORMATS = [
  {
    id: "epub" as const,
    label: "EPUB",
    descKey: "export.epubDesc",
    icon: BookOpen,
  },
  {
    id: "markdown" as const,
    label: "Markdown",
    descKey: "export.markdownDesc",
    icon: FileText,
  },
  {
    id: "txt" as const,
    label: "TXT",
    descKey: "export.textDesc",
    icon: AlignLeft,
  },
] as const;

export default function ExportDialog({
  novelTitle,
  onClose,
  onExport,
}: Props) {
  const { t } = useTranslation();
  const [format, setFormat] = useState<"epub" | "markdown" | "txt">("epub");
  const [exporting, setExporting] = useState(false);
  const [error, setError] = useState("");
  const [success, setSuccess] = useState(false);

  async function handleExport() {
    if (exporting) return;
    setExporting(true);
    setError("");
    setSuccess(false);
    try {
      await onExport(format);
      setSuccess(true);
    } catch (e) {
      setError(toErrorMessage(e, t("export.exportFailed")));
    } finally {
      setExporting(false);
    }
  }

  function handleKeyDown(e: React.KeyboardEvent) {
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

        <h2 className="text-base font-semibold mb-1">
          {t("export.exportWork")}
        </h2>
        <p className="text-sm text-muted-foreground mb-5">{novelTitle}</p>

        {error && (
          <p className="text-sm text-red-600 bg-danger-bg border border-danger-border rounded-md px-3 py-2 mb-4">
            {error}
          </p>
        )}

        {success && (
          <p className="text-sm text-success-foreground bg-success border-success-border rounded-md px-3 py-2 mb-4">
            {t("export.exportSuccess")}
          </p>
        )}

        <div className="space-y-2">
          {FORMATS.map((f) => (
            <button
              key={f.id}
              onClick={() => setFormat(f.id)}
              className={`w-full flex items-start gap-3 p-3 rounded-lg border text-left transition-colors
                ${
                  format === f.id
                    ? "ring-2 ring-primary border-primary/30 bg-primary/5"
                    : "border-border hover:bg-muted/50"
                }`}
            >
              <f.icon
                className={`w-5 h-5 mt-0.5 shrink-0 ${format === f.id ? "text-primary" : "text-muted-foreground"}`}
              />
              <div>
                <span
                  className={`text-sm font-medium ${format === f.id ? "text-primary" : "text-foreground"}`}
                >
                  {f.label}
                </span>
                <p className="text-xs text-muted-foreground mt-0.5">
                  {t(f.descKey)}
                </p>
              </div>
            </button>
          ))}
        </div>

        <div className="flex justify-end gap-2 mt-6">
          {success ? (
            <button
              onClick={onClose}
              className="h-9 px-4 rounded-md text-sm bg-primary text-primary-foreground hover:opacity-90 transition-opacity"
            >
              {t("export.done")}
            </button>
          ) : (
            <>
              <button
                onClick={onClose}
                className="h-9 px-4 rounded-md text-sm border hover:bg-muted transition-colors"
              >
                {t("export.cancel")}
              </button>
              <button
                onClick={handleExport}
                disabled={exporting}
                className="h-9 px-4 rounded-md text-sm bg-primary text-primary-foreground hover:opacity-90 transition-opacity disabled:opacity-50"
              >
                {exporting ? t("export.exporting") : t("export.export")}
              </button>
            </>
          )}
        </div>
      </div>
    </div>
  );
}

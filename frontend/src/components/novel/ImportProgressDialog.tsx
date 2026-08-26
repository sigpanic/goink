import {
  AlertTriangle,
  Bot,
  CheckCircle2,
  FileText,
  Loader2,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import type {
  ImportProgressStage,
  ImportProgressState,
} from "@/hooks/useImportNovel";
import type { llm } from "@/lib/wailsjs/go/models";
import PopSelect from "@/components/shared/PopSelect";

interface Props {
  open: boolean;
  progress: ImportProgressState;
  error: string;
  skippedCount: number;
  skippedChapters: { title: string; reason: string }[];
  selectedModel: llm.AvailableModel | null;
  models: llm.AvailableModel[];
  onModelChange: (model: llm.AvailableModel) => void;
  onStartLLM: () => void;
  onClose: () => void;
}

export default function ImportProgressDialog({
  open,
  progress,
  error,
  skippedCount,
  skippedChapters,
  selectedModel,
  models,
  onModelChange,
  onStartLLM,
  onClose,
}: Props) {
  const { t } = useTranslation();

  const STAGE_LABEL: Record<ImportProgressStage, string> = {
    idle: t("novel.importPreparing"),
    select_file: t("novel.importSelectFile"),
    parse: t("novel.importParsing"),
    create_novel: t("novel.importCreating"),
    write_chapters: t("novel.importWriting"),
    commit: t("novel.importSaving"),
    done: t("novel.importComplete"),
    error: t("novel.importFailed"),
    needs_llm: t("novel.importNeedsLLM"),
    analyzing: t("novel.importAnalyzing"),
  };

  if (!open) return null;

  const isDone = progress.stage === "done";
  const isError = progress.stage === "error" || error !== "";
  const isNeedsLLM = progress.stage === "needs_llm";
  const isAnalyzing = progress.stage === "analyzing";
  const canClose = isDone || isError;
  const percent = Math.max(0, Math.min(100, progress.percent || 0));
  const chapterText =
    progress.total > 0
      ? t("novel.chapterCount", {
          current: progress.current,
          total: progress.total,
        })
      : "";

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-foreground/40" />
      <div className="relative w-[420px] max-w-[90vw] rounded-xl border border-border bg-background p-6 shadow-2xl">
        <div className="flex items-start gap-3">
          <div
            className={`flex h-10 w-10 shrink-0 items-center justify-center rounded-lg ${
              isError
                ? "bg-danger-bg text-destructive"
                : isDone
                  ? "bg-tag-green text-tag-green-foreground"
                  : isNeedsLLM
                    ? "bg-primary/10 text-primary"
                    : "bg-primary/10 text-primary"
            }`}
          >
            {isError ? (
              <AlertTriangle className="h-5 w-5" />
            ) : isDone ? (
              <CheckCircle2 className="h-5 w-5" />
            ) : isNeedsLLM ? (
              <Bot className="h-5 w-5" />
            ) : progress.stage === "select_file" ? (
              <FileText className="h-5 w-5" />
            ) : (
              <Loader2
                className={`h-5 w-5 ${isAnalyzing ? "animate-spin" : ""}`}
              />
            )}
          </div>
          <div className="min-w-0 flex-1">
            <h2 className="text-base font-semibold text-foreground">
              {t("novel.importBook")}
            </h2>
            <p className="mt-1 text-sm text-muted-foreground">
              {STAGE_LABEL[progress.stage]}
            </p>
          </div>
          {chapterText && (
            <span className="rounded-md bg-muted px-2 py-1 text-xs text-muted-foreground">
              {chapterText}
            </span>
          )}
        </div>

        {/* 进度条 */}
        {!isNeedsLLM && (
          <div className="mt-5">
            <div className="mb-2 flex items-center justify-between gap-3">
              <p
                className={`text-sm ${isError ? "text-destructive" : "text-foreground"}`}
              >
                {error || progress.message}
              </p>
              {!isError && (
                <span className="text-xs tabular-nums text-muted-foreground">
                  {percent}%
                </span>
              )}
            </div>
            <div className="h-2 overflow-hidden rounded-full bg-muted">
              <div
                className={`h-full rounded-full transition-all duration-300 ${
                  isError
                    ? "bg-destructive"
                    : isDone
                      ? "bg-tag-green-foreground"
                      : "bg-primary"
                }`}
                style={{ width: `${isError ? 100 : percent}%` }}
              />
            </div>
          </div>
        )}

        {/* needs_llm: 提示用户使用 AI 分析 + 模型选择 */}
        {isNeedsLLM && !isError && (
          <div className="mt-4 space-y-3">
            <p className="text-sm text-muted-foreground">
              {t("novel.importNeedsLLMDesc")}
            </p>
            <div className="flex items-center gap-2">
              <span className="text-xs text-muted-foreground">
                {t("novel.importModel")}
              </span>
              <PopSelect
                value={selectedModel?.Key ?? ""}
                options={models.map((m) => ({
                  value: m.Key,
                  label: m.ModelName,
                }))}
                onChange={(key) => {
                  const model = models.find((m) => m.Key === key);
                  if (model) onModelChange(model);
                }}
                minWidth="180px"
                dropUp={false}
              />
            </div>
            <div className="flex justify-end gap-2">
              <button
                onClick={onClose}
                className="h-9 rounded-md border border-border px-4 text-sm text-muted-foreground transition-colors hover:bg-muted"
              >
                {t("common.cancel")}
              </button>
              <button
                onClick={onStartLLM}
                disabled={!selectedModel}
                className="h-9 rounded-md bg-primary px-4 text-sm text-primary-foreground transition-opacity hover:opacity-90 disabled:opacity-40"
              >
                {t("novel.importAnalyzeBtn")}
              </button>
            </div>
          </div>
        )}

        {isDone && skippedCount > 0 && (
          <div className="mt-4 rounded-md border border-border bg-muted px-3 py-2">
            <p className="text-xs font-medium text-muted-foreground">
              {t("novel.importSkippedCount", { count: skippedCount })}
            </p>
            <ul className="mt-1.5 max-h-32 space-y-1 overflow-y-auto">
              {skippedChapters.map((ch, i) => (
                <li key={i} className="text-xs text-muted-foreground">
                  <span className="font-medium">{ch.title}</span>
                  <span className="mx-1">—</span>
                  <span>{ch.reason}</span>
                </li>
              ))}
            </ul>
          </div>
        )}

        {isError && (
          <p className="mt-4 rounded-md border border-danger-border bg-danger-bg px-3 py-2 text-xs text-destructive">
            {t("novel.importRollbackNote")}
          </p>
        )}

        {canClose && (
          <div className="mt-6 flex justify-end">
            <button
              onClick={onClose}
              className="h-9 rounded-md bg-primary px-4 text-sm text-primary-foreground transition-opacity hover:opacity-90"
            >
              {t("common.done")}
            </button>
          </div>
        )}
      </div>
    </div>
  );
}

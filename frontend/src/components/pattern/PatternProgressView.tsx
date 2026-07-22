import { CheckCircle2, Circle, Loader2, Route, Sparkle } from "lucide-react";
import { useTranslation } from "react-i18next";
import type {
  PatternLLMStatus,
  PatternProgressEvent,
  PatternProgressStage,
  PatternProgressState,
} from "@/hooks/usePatternProgress";

interface Props {
  progress: PatternProgressState | null;
  events: PatternProgressEvent[];
  novelTitle: string;
  chapterCount: number;
}

type PipelineStage = Exclude<PatternProgressStage, "done">;

const stageOrder: PipelineStage[] = [
  "loaded",
  "boundaries",
  "summaries",
  "initial_chunks",
  "compress_chunks",
  "finalizing",
];

function stageKey(stage: PatternProgressStage) {
  switch (stage) {
    case "loaded":
      return "extract.progress.stageLoaded";
    case "boundaries":
      return "extract.progress.stageBoundaries";
    case "summaries":
      return "extract.progress.stageSummaries";
    case "initial_chunks":
      return "extract.progress.stageInitialChunks";
    case "compress_chunks":
      return "extract.progress.stageCompressChunks";
    case "finalizing":
      return "extract.progress.stageFinalizing";
    case "done":
      return "extract.progress.stageDone";
  }
}

// stageIndex returns the ordinal position of a stage in the pipeline.
function stageIndex(stage: PatternProgressStage): number {
  return stageOrder.indexOf(stage as PipelineStage);
}

function isStageDone(
  stage: PipelineStage,
  progress: PatternProgressState | null,
): boolean {
  if (!progress) return false;
  if (progress.stage === "done") return true;
  return stageIndex(progress.stage) > stageIndex(stage);
}

function isStageActive(
  stage: PipelineStage,
  progress: PatternProgressState | null,
): boolean {
  if (!progress) return false;
  return progress.stage === stage;
}

function llmStatusLabel(status: PatternLLMStatus): string {
  switch (status) {
    case "thinking":
      return "extract.progress.thinking";
    case "generating":
      return "extract.progress.generating";
    default:
      return "";
  }
}

function formatCompactNumber(value: number) {
  if (!value) return "";
  return value.toLocaleString();
}

export default function PatternProgressView({
  progress,
  events,
  novelTitle,
  chapterCount,
}: Props) {
  const { t } = useTranslation();
  const isComplete = progress?.stage === "done";

  return (
    <div className="mx-auto flex w-full max-w-4xl flex-col gap-4">
      {/* Header card */}
      <section className="rounded-lg border bg-card p-4">
        <div className="flex items-start justify-between gap-4">
          <div className="min-w-0">
            <div className="flex items-center gap-2 text-sm font-semibold text-foreground">
              <Sparkle className="h-4 w-4 text-action-extract" />
              <span className="truncate">{t("extract.progress.title")}</span>
            </div>
            <p className="mt-1 truncate text-xs text-muted-foreground">
              {t("extract.progress.extractingWork", {
                title: novelTitle || t("extract.progress.unknownWork"),
              })}
              <span className="mx-1">·</span>
              {t("extract.progress.chapterCount", { count: chapterCount })}
            </p>
          </div>
          {isComplete && (
            <div className="shrink-0 text-right">
              <div className="flex items-center gap-1 text-sm font-semibold text-action-save">
                <CheckCircle2 className="h-4 w-4" />
                {t("extract.progress.done")}
              </div>
            </div>
          )}
        </div>

        {/* Status line: message + LLM status + metadata */}
        <div className="mt-3 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
          {progress?.message && !progress.llmStatus && (
            <span className="min-w-0 max-w-full truncate rounded-md border bg-muted/30 px-2 py-1 text-foreground">
              {progress.message}
            </span>
          )}
          {progress?.llmStatus && (
            <span className="flex items-center gap-1 rounded-md border bg-action-extract/10 px-2 py-1 text-action-extract">
              <Loader2 className="h-3 w-3 animate-spin" />
              {t(llmStatusLabel(progress.llmStatus))}
            </span>
          )}
          {!!progress?.round && (
            <span className="rounded-md border bg-muted/30 px-2 py-1">
              {t("extract.progress.round", { round: progress.round })}
            </span>
          )}
          {!!progress?.batchTotal && (
            <span className="rounded-md border bg-muted/30 px-2 py-1">
              {t("extract.progress.batch", {
                current: progress.batchIndex,
                total: progress.batchTotal,
              })}
            </span>
          )}
          {!!progress?.tokens && (
            <span className="rounded-md border bg-muted/30 px-2 py-1">
              {t("extract.progress.tokens", {
                count: formatCompactNumber(progress.tokens),
              })}
            </span>
          )}
        </div>
      </section>

      {/* Pipeline stages */}
      <section className="rounded-lg border bg-card p-4">
        <div className="mb-3 flex items-center gap-2 text-sm font-semibold text-foreground">
          <Route className="h-4 w-4 text-muted-foreground" />
          {t("extract.progress.pipeline")}
        </div>
        <div className="grid gap-2 md:grid-cols-3 lg:grid-cols-6">
          {stageOrder.map((stage) => {
            const done = isStageDone(stage, progress);
            const active = isStageActive(stage, progress);
            return (
              <div
                key={stage}
                className="flex min-w-0 items-center gap-2 rounded-md border bg-muted/20 px-3 py-2"
              >
                {done ? (
                  <CheckCircle2 className="h-4 w-4 shrink-0 text-action-save" />
                ) : active ? (
                  <Loader2 className="h-4 w-4 shrink-0 animate-spin text-action-extract" />
                ) : (
                  <Circle className="h-4 w-4 shrink-0 text-muted-foreground" />
                )}
                <div className="min-w-0">
                  <div
                    className={`truncate text-xs font-medium ${active || done ? "text-foreground" : "text-muted-foreground"}`}
                  >
                    {t(stageKey(stage))}
                  </div>
                  <div className="text-[11px] text-muted-foreground">
                    {done
                      ? t("extract.progress.done")
                      : active
                        ? progress?.llmStatus
                          ? t(llmStatusLabel(progress.llmStatus))
                          : t("extract.progress.running")
                        : t("extract.progress.waiting")}
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      </section>

      {/* Recent events */}
      <section className="rounded-lg border bg-card p-4">
        <div className="mb-3 text-sm font-semibold text-foreground">
          {t("extract.progress.recentEvents")}
        </div>
        {events.length === 0 ? (
          <p className="text-xs text-muted-foreground">
            {t("extract.progress.noEvents")}
          </p>
        ) : (
          <div className="space-y-2">
            {events.map((event) => (
              <div
                key={event.id}
                className="flex items-center justify-between gap-3 rounded-md border bg-muted/20 px-3 py-2"
              >
                <div className="min-w-0">
                  <div className="truncate text-xs text-foreground">
                    {event.message || t(stageKey(event.stage))}
                  </div>
                  <div className="mt-0.5 flex flex-wrap items-center gap-x-2 gap-y-1 text-[11px] text-muted-foreground">
                    {event.llmStatus && (
                      <span>{t(llmStatusLabel(event.llmStatus))}</span>
                    )}
                    {!!event.round && (
                      <span>
                        {t("extract.progress.round", { round: event.round })}
                      </span>
                    )}
                    {!!event.batchTotal && (
                      <span>
                        {t("extract.progress.batch", {
                          current: event.batchIndex,
                          total: event.batchTotal,
                        })}
                      </span>
                    )}
                    {!!event.boundaryCount && (
                      <span>
                        {t("extract.progress.boundaryCount", {
                          count: event.boundaryCount,
                        })}
                      </span>
                    )}
                    {!!event.summaryCount && (
                      <span>
                        {t("extract.progress.summaryCount", {
                          count: event.summaryCount,
                        })}
                      </span>
                    )}
                    {!!event.chunkCount && (
                      <span>
                        {t("extract.progress.chunkCount", {
                          count: event.chunkCount,
                        })}
                      </span>
                    )}
                  </div>
                </div>
                {!!event.tokens && (
                  <span className="shrink-0 text-xs tabular-nums text-muted-foreground">
                    {formatCompactNumber(event.tokens)} tok
                  </span>
                )}
              </div>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}

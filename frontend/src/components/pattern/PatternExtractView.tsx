import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  BookOpen,
  CheckSquare,
  Loader2,
  Save,
  Sparkle,
  Square,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import { useApp } from "@/hooks/useApp";
import type { chapter, novel } from "@/hooks/useApp";
import {
  createPatternTaskID,
  usePatternProgress,
} from "@/hooks/usePatternProgress";
import Markdown from "@/components/Markdown";
import { splitFrontmatter } from "@/components/content/types";
import PopSelect from "@/components/chat/PopSelect";
import PatternProgressView from "./PatternProgressView";
import ChapterRangeInput from "./ChapterRangeInput";

interface Props {
  currentNovelId: number;
}

type Phase = "idle" | "extracting" | "preview";
type Scope = "all" | "selected";

interface ModelOption {
  Key: string;
  ModelName: string;
}

function errorMessage(error: unknown, fallback: string): string {
  if (error instanceof Error) return error.message;
  if (typeof error === "string") return error;
  return fallback;
}

export default function PatternExtractView({ currentNovelId }: Props) {
  const app = useApp();
  const { t } = useTranslation();
  const runningNovelIdRef = useRef<number | null>(null);
  const runningTaskIdRef = useRef<string | null>(null);
  const userSelectedNovelRef = useRef(false);
  const [targetNovelId, setTargetNovelId] = useState(currentNovelId);
  const [runningTask, setRunningTask] = useState<{
    taskId: string;
    novelId: number;
    title: string;
    chapterCount: number;
  } | null>(null);
  const [novels, setNovels] = useState<novel.Novel[]>([]);
  const [chapters, setChapters] = useState<chapter.Chapter[]>([]);
  const [selected, setSelected] = useState<Set<number>>(new Set());
  const [scope, setScope] = useState<Scope>("all");
  const [phase, setPhase] = useState<Phase>("idle");
  const [loading, setLoading] = useState(false);
  const [modelKey, setModelKey] = useState("");
  const [models, setModels] = useState<ModelOption[]>([]);
  const [error, setError] = useState("");
  const [result, setResult] = useState<{
    taskId: string;
    novelId: number;
    name: string;
    description: string;
    filePath: string;
    rawContent: string;
  } | null>(null);
  const {
    progress,
    events,
    reset: resetProgress,
  } = usePatternProgress(runningTask?.taskId ?? null);

  useEffect(() => {
    let cancelled = false;
    app
      .GetNovels()
      .then((list) => {
        if (cancelled) return;
        setNovels(list ?? []);
      })
      .catch((e) => {
        if (!cancelled) console.error("Load novels failed", e);
      });
    return () => {
      cancelled = true;
    };
  }, [app]);

  useEffect(() => {
    if (!userSelectedNovelRef.current) {
      setTargetNovelId(currentNovelId);
    }
  }, [currentNovelId]);

  useEffect(() => {
    if (phase === "extracting") return;
    let cancelled = false;
    const chaptersPromise = targetNovelId
      ? app.GetChapters(targetNovelId)
      : Promise.resolve([]);
    chaptersPromise
      .then((list) => {
        if (cancelled) return;
        setChapters(list ?? []);
        setSelected(new Set());
      })
      .catch((e) => {
        if (!cancelled) console.error("Load chapters failed", e);
      });
    return () => {
      cancelled = true;
    };
  }, [app, targetNovelId, phase]);

  useEffect(() => {
    let cancelled = false;
    app
      .GetModels()
      .then((list) => {
        if (cancelled) return;
        if (list?.length) {
          setModels(list);
          app.GetSettings().then((s) => {
            if (cancelled) return;
            let key = s?.selected_model_key || "";
            if (!list.find((m) => m.Key === key)) key = list[0].Key;
            setModelKey(key);
          });
        }
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [app]);

  const modelOptions = useMemo(
    () => models.map((m) => ({ value: m.Key, label: m.ModelName })),
    [models],
  );
  const novelOptions = useMemo(
    () => novels.map((n) => ({ value: String(n.id), label: n.title })),
    [novels],
  );
  const activeChapterIds = useMemo(
    () =>
      scope === "all"
        ? []
        : chapters.filter((ch) => selected.has(ch.id)).map((ch) => ch.id),
    [chapters, scope, selected],
  );
  const activeChapterCount = scope === "all" ? chapters.length : selected.size;
  const canExtract =
    targetNovelId > 0 &&
    activeChapterCount >= 5 &&
    !!modelKey &&
    phase !== "preview";
  const { meta, body } = result
    ? splitFrontmatter(result.rawContent)
    : { meta: {}, body: "" };
  const allSelected =
    (scope === "all" && chapters.length > 0) ||
    (scope === "selected" &&
      selected.size === chapters.length &&
      chapters.length > 0);
  const targetNovelTitle =
    novels.find((n) => n.id === targetNovelId)?.title ?? "";

  const toggleChapter = useCallback((id: number) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  const toggleAll = useCallback(() => {
    setSelected((prev) => {
      if (prev.size === chapters.length) return new Set();
      return new Set(chapters.map((ch) => ch.id));
    });
  }, [chapters]);

  const selectWholeBook = useCallback(() => {
    setScope("all");
    setSelected(new Set());
  }, []);

  const selectCustomChapters = useCallback(() => {
    setScope("selected");
  }, []);

  const handleTargetNovelChange = useCallback((value: string) => {
    const nextNovelId = Number(value);
    if (!Number.isFinite(nextNovelId) || nextNovelId <= 0) return;
    userSelectedNovelRef.current = true;
    setTargetNovelId(nextNovelId);
    setPhase("idle");
    setError("");
    setResult(null);
  }, []);

  const handleExtract = useCallback(async () => {
    if (phase === "extracting") {
      const runningNovelId = runningNovelIdRef.current;
      if (runningNovelId !== null) app.CancelExtractPattern(runningNovelId);
      runningNovelIdRef.current = null;
      runningTaskIdRef.current = null;
      setRunningTask(null);
      setPhase("idle");
      return;
    }
    if (!canExtract) return;

    const [providerName, modelID] = modelKey.split("/");
    if (!providerName || !modelID) return;
    const extractNovelId = targetNovelId;
    const taskId = createPatternTaskID();
    const taskTitle = targetNovelTitle || t("extract.progress.unknownWork");

    setPhase("extracting");
    setError("");
    setResult(null);
    resetProgress();
    runningNovelIdRef.current = extractNovelId;
    runningTaskIdRef.current = taskId;
    setRunningTask({
      taskId,
      novelId: extractNovelId,
      title: taskTitle,
      chapterCount: activeChapterCount,
    });

    try {
      const res = await app.ExtractPattern({
        task_id: taskId,
        novel_id: extractNovelId,
        provider_name: providerName,
        model_id: modelID,
        reasoning_effort: "",
        chapter_ids: activeChapterIds.length > 0 ? activeChapterIds : undefined,
      });
      if (runningTaskIdRef.current !== taskId) return;
      setResult({
        taskId: res.task_id || taskId,
        novelId: extractNovelId,
        name: res.name,
        description: res.description,
        filePath: res.file_path,
        rawContent: res.raw_content,
      });
      setPhase("preview");
    } catch (e: unknown) {
      if (runningTaskIdRef.current !== taskId) return;
      const msg = errorMessage(e, "");
      if (!msg.includes("canceled") && !msg.includes("取消")) {
        setError(msg || t("extract.extractFailed"));
      }
      setPhase("idle");
    } finally {
      if (runningTaskIdRef.current === taskId) {
        runningNovelIdRef.current = null;
        runningTaskIdRef.current = null;
        setRunningTask(null);
      }
    }
  }, [
    activeChapterCount,
    activeChapterIds,
    app,
    canExtract,
    modelKey,
    phase,
    resetProgress,
    t,
    targetNovelId,
    targetNovelTitle,
  ]);

  const handleSave = useCallback(async () => {
    if (!result) return;
    setLoading(true);
    setError("");
    try {
      await app.SaveContent({
        novel_id: result.novelId,
        path: result.filePath,
        content: result.rawContent,
      });
      setPhase("idle");
      setResult(null);
      resetProgress();
    } catch (e: unknown) {
      setError(errorMessage(e, t("extract.saveFailed")));
    } finally {
      setLoading(false);
    }
  }, [app, resetProgress, result, t]);

  return (
    <div className="flex-1 flex flex-col min-h-0 bg-background">
      <div className="flex items-center justify-between px-6 py-4 border-b shrink-0">
        {phase === "preview" && result ? (
          <>
            <span className="text-sm text-muted-foreground flex items-center gap-2">
              <Sparkle className="w-4 h-4 text-primary" />
              {t("extract.generated")}「{result.name}」
            </span>
            <div className="flex items-center gap-2">
              <button
                onClick={() => {
                  setPhase("idle");
                  setResult(null);
                  resetProgress();
                }}
                className="h-8 px-3 rounded-lg text-sm border border-border hover:bg-muted transition-colors"
              >
                {t("extract.cancel")}
              </button>
              <button
                onClick={handleExtract}
                className="h-8 px-3 rounded-lg text-sm border border-border hover:bg-muted transition-colors"
              >
                {t("extract.reExtract")}
              </button>
              <button
                onClick={handleSave}
                disabled={loading}
                className="inline-flex items-center gap-1.5 h-8 px-4 rounded-lg text-sm font-medium bg-action-save text-action-save-foreground hover:bg-action-save/80 disabled:opacity-50 transition-colors"
              >
                <Save className="w-3.5 h-3.5" />
                {loading ? t("extract.saving") : t("extract.saveToUserSkill")}
              </button>
            </div>
          </>
        ) : (
          <>
            <span className="text-sm text-muted-foreground">
              {t("extract.totalChapters", { count: chapters.length })}
              <span className="ml-2 text-primary">
                · {t("extract.currentRange", { count: activeChapterCount })}
              </span>
            </span>
            <div className="flex items-center gap-2">
              <div
                className={
                  phase === "extracting" ? "pointer-events-none opacity-60" : ""
                }
              >
                <PopSelect
                  value={String(targetNovelId)}
                  options={novelOptions}
                  onChange={handleTargetNovelChange}
                  minWidth="160px"
                  placeholder={t("extract.noAvailableWork")}
                  dropUp={false}
                />
              </div>
              <div className="inline-flex items-center gap-1 rounded-lg bg-muted/60 p-0.5">
                <button
                  onClick={selectWholeBook}
                  className={`h-7 px-2.5 rounded-md text-xs transition-colors ${
                    scope === "all"
                      ? "bg-background text-foreground shadow-sm"
                      : "text-muted-foreground hover:text-foreground"
                  }`}
                >
                  {t("extract.wholeBook")}
                </button>
                <button
                  onClick={selectCustomChapters}
                  className={`h-7 px-2.5 rounded-md text-xs transition-colors ${
                    scope === "selected"
                      ? "bg-background text-foreground shadow-sm"
                      : "text-muted-foreground hover:text-foreground"
                  }`}
                >
                  {t("extract.selectedChapters")}
                </button>
              </div>
              <div
                className={
                  phase === "extracting" ? "pointer-events-none opacity-60" : ""
                }
              >
                <PopSelect
                  value={modelKey}
                  options={modelOptions}
                  onChange={setModelKey}
                  minWidth="140px"
                  dropUp={false}
                />
              </div>
              <button
                onClick={handleExtract}
                disabled={!canExtract && phase !== "extracting"}
                className={`inline-flex items-center gap-1.5 h-8 px-4 rounded-lg text-sm font-medium transition-colors shadow-sm disabled:opacity-40 ${
                  phase === "extracting"
                    ? "bg-destructive text-destructive-foreground hover:bg-destructive/80"
                    : "bg-action-extract text-action-extract-foreground hover:bg-action-extract/80"
                }`}
              >
                {phase === "extracting" ? (
                  <>
                    <Loader2 className="w-3.5 h-3.5 animate-spin" />
                    {t("extract.cancel")}
                  </>
                ) : (
                  <>
                    <Sparkle className="w-3.5 h-3.5" />
                    {t("extract.startExtract")}
                  </>
                )}
              </button>
            </div>
          </>
        )}
      </div>

      {error && (
        <div className="mx-6 mt-3 px-3 py-2 text-xs text-destructive bg-danger-bg border border-danger-border rounded-md shrink-0">
          {error}
        </div>
      )}

      {phase === "preview" && result ? (
        <div className="flex-1 min-h-0 overflow-y-auto px-6 py-4 space-y-3">
          {Object.keys(meta).length > 0 && (
            <table className="border bg-muted/20 w-full text-sm rounded-lg overflow-hidden">
              <tbody>
                {meta.name && (
                  <tr className="border-b">
                    <td className="px-4 py-2 text-muted-foreground whitespace-nowrap w-16">
                      {t("extract.name")}
                    </td>
                    <td className="px-4 py-2 text-foreground font-semibold">
                      {meta.name}
                    </td>
                  </tr>
                )}
                {(meta.description || result.description) && (
                  <tr className="border-b">
                    <td className="px-4 py-2 text-muted-foreground whitespace-nowrap w-16">
                      {t("extract.summary")}
                    </td>
                    <td className="px-4 py-2 text-foreground">
                      {meta.description || result.description}
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          )}
          <div className="rounded-lg border bg-muted/10 p-4">
            <Markdown content={body} />
          </div>
        </div>
      ) : phase === "extracting" && runningTask ? (
        <div className="flex-1 min-h-0 overflow-y-auto p-6">
          <PatternProgressView
            progress={progress}
            events={events}
            novelTitle={runningTask.title}
            chapterCount={runningTask.chapterCount}
          />
        </div>
      ) : (
        <div className="flex-1 min-h-0 overflow-y-auto p-6">
          {chapters.length === 0 ? (
            <div className="flex flex-col items-center justify-center h-full text-muted-foreground gap-3">
              <BookOpen className="w-12 h-12 opacity-20" />
              <p className="text-sm">{t("extract.noChaptersYet")}</p>
            </div>
          ) : (
            <div className="space-y-3">
              <div className="flex items-center justify-between rounded-lg border bg-card px-4 py-3">
                <div>
                  <h2 className="text-sm font-semibold text-foreground">
                    {t("extract.chapterRange")}
                  </h2>
                  <p className="text-xs text-muted-foreground mt-1">
                    {t("extract.chapterRangeNote")}
                  </p>
                </div>
                <div className="flex items-center gap-2">
                  {scope === "selected" && (
                    <>
                      <ChapterRangeInput
                        chapters={chapters}
                        onSelect={setSelected}
                        disabled={phase === "extracting"}
                      />
                      <div className="w-px h-6 bg-border" />
                    </>
                  )}
                  <button
                    onClick={toggleAll}
                    disabled={scope !== "selected"}
                    className="inline-flex items-center gap-1.5 h-7 px-3 rounded-md text-xs border border-border hover:bg-muted disabled:opacity-40 transition-colors"
                  >
                    {allSelected ? (
                      <CheckSquare className="w-4 h-4" />
                    ) : (
                      <Square className="w-4 h-4" />
                    )}
                    {scope === "all"
                      ? t("extract.selectAll")
                      : allSelected
                        ? t("extract.deselectAll")
                        : t("extract.selectAll")}
                  </button>
                </div>
              </div>
              <div className="grid grid-cols-[repeat(auto-fill,minmax(240px,1fr))] gap-3">
                {chapters.map((ch) => {
                  const checked = scope === "all" || selected.has(ch.id);
                  return (
                    <button
                      key={ch.id}
                      onClick={() => {
                        if (scope === "all") {
                          // 整书模式下点击：切到自选模式，全选后移除该章
                          const allIds = new Set(chapters.map((c) => c.id));
                          allIds.delete(ch.id);
                          setSelected(allIds);
                          setScope("selected");
                        } else {
                          toggleChapter(ch.id);
                        }
                      }}
                      className={`group flex items-start gap-3 rounded-lg border bg-card p-3 text-left transition-colors hover:bg-muted/40 ${
                        checked ? "ring-2 ring-primary" : ""
                      }`}
                    >
                      <span className="mt-0.5 text-muted-foreground group-hover:text-foreground">
                        {checked ? (
                          <CheckSquare className="w-4 h-4 text-primary" />
                        ) : (
                          <Square className="w-4 h-4" />
                        )}
                      </span>
                      <span className="min-w-0">
                        <span className="block text-sm text-foreground truncate">
                          {t("extract.chapterN", { n: ch.chapter_number })}{" "}
                          {ch.title}
                        </span>
                        <span className="block text-xs text-muted-foreground mt-1">
                          {ch.word_count} {t("extract.charCount")}
                        </span>
                      </span>
                    </button>
                  );
                })}
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

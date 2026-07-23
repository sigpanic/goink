import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { BookOpen, CheckSquare, Sparkle, Square } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useApp } from "@/hooks/useApp";
import type { chapter, novel } from "@/hooks/useApp";
import { createPatternTaskID } from "@/hooks/usePatternProgress";
import PopSelect from "@/components/chat/PopSelect";
import ChapterRangeInput from "./ChapterRangeInput";
import PatternSessionView from "./PatternSessionView";

interface Props {
  currentNovelId: number;
}

type View = "select" | "session";
type Scope = "all" | "selected";

interface ModelOption {
  Key: string;
  ModelName: string;
}

interface SessionParams {
  taskId: string;
  novelId: number;
  chapterIds: number[];
  modelKey: string;
  title: string;
  chapterCount: number;
}

export default function PatternExtractView({ currentNovelId }: Props) {
  const app = useApp();
  const { t } = useTranslation();
  const userSelectedNovelRef = useRef(false);
  const [view, setView] = useState<View>("select");
  const [sessionParams, setSessionParams] = useState<SessionParams | null>(null);
  const [targetNovelId, setTargetNovelId] = useState(currentNovelId);
  const [novels, setNovels] = useState<novel.Novel[]>([]);
  const [chapters, setChapters] = useState<chapter.Chapter[]>([]);
  const [selected, setSelected] = useState<Set<number>>(new Set());
  const [scope, setScope] = useState<Scope>("all");
  const [modelKey, setModelKey] = useState("");
  const [models, setModels] = useState<ModelOption[]>([]);

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
  }, [app, targetNovelId]);

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
    targetNovelId > 0 && activeChapterCount >= 5 && !!modelKey;
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
  }, []);

  const handleExtract = useCallback(() => {
    if (!canExtract) return;
    const taskId = createPatternTaskID();
    setSessionParams({
      taskId,
      novelId: targetNovelId,
      chapterIds: activeChapterIds,
      modelKey,
      title: targetNovelTitle || t("extract.progress.unknownWork"),
      chapterCount: activeChapterCount,
    });
    setView("session");
  }, [
    activeChapterCount,
    activeChapterIds,
    canExtract,
    modelKey,
    t,
    targetNovelId,
    targetNovelTitle,
  ]);

  if (view === "session" && sessionParams) {
    return (
      <PatternSessionView
        taskId={sessionParams.taskId}
        novelId={sessionParams.novelId}
        chapterIds={sessionParams.chapterIds}
        modelKey={sessionParams.modelKey}
        title={sessionParams.title}
        chapterCount={sessionParams.chapterCount}
        onExit={() => {
          setView("select");
          setSessionParams(null);
        }}
      />
    );
  }

  return (
    <div className="flex-1 flex flex-col min-h-0 bg-background">
      <div className="flex items-center justify-between px-6 py-4 border-b shrink-0">
        <span className="text-sm text-muted-foreground">
          {t("extract.totalChapters", { count: chapters.length })}
          <span className="ml-2 text-primary">
            · {t("extract.currentRange", { count: activeChapterCount })}
          </span>
        </span>
        <div className="flex items-center gap-2">
          <PopSelect
            value={String(targetNovelId)}
            options={novelOptions}
            onChange={handleTargetNovelChange}
            onOpen={() => {
              app
                .GetNovels()
                .then((list) => setNovels(list ?? []))
                .catch(() => {});
            }}
            minWidth="160px"
            placeholder={t("extract.noAvailableWork")}
            dropUp={false}
          />
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
          <PopSelect
            value={modelKey}
            options={modelOptions}
            onChange={setModelKey}
            minWidth="140px"
            dropUp={false}
          />
          <button
            onClick={handleExtract}
            disabled={!canExtract}
            className="inline-flex items-center gap-1.5 h-8 px-4 rounded-lg text-sm font-medium transition-colors shadow-sm bg-action-extract text-action-extract-foreground hover:bg-action-extract/80 disabled:opacity-40"
          >
            <Sparkle className="w-3.5 h-3.5" />
            {t("extract.startExtract")}
          </button>
        </div>
      </div>

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
    </div>
  );
}

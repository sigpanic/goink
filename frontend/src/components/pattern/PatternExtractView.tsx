import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { BookOpen, CheckSquare, Sparkle, Square } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useNovels } from "@/components/novel/useNovels";
import { useChapters } from "@/components/chapter/useChapters";
import { useModels } from "@/components/settings/useModels";
import { useSettings } from "@/components/settings/useSettings";
import type { llm } from "@/lib/wailsjs/go/models";
import { createPatternTaskID } from "@/hooks/usePatternProgress";
import PopSelect from "@/components/shared/PopSelect";
import ModelPicker from "@/components/model/ModelPicker";
import ChapterRangeInput from "./ChapterRangeInput";
import PatternSessionView from "./PatternSessionView";

interface Props {
  currentNovelId: number;
}

type View = "select" | "session";
type Scope = "all" | "selected";

interface SessionParams {
  taskId: string;
  novelId: number;
  chapterIds: number[];
  providerName: string;
  modelId: string;
  reasoningEffort: string;
  title: string;
  chapterCount: number;
}

export default function PatternExtractView({ currentNovelId }: Props) {
  const { t } = useTranslation();
  const userSelectedNovelRef = useRef(false);
  const [view, setView] = useState<View>("select");
  const [sessionParams, setSessionParams] = useState<SessionParams | null>(
    null,
  );
  const [targetNovelId, setTargetNovelId] = useState(currentNovelId);
  // 3.9: novels 走 useNovels query（共享缓存）。refetchNovels 供 PopSelect onOpen 强制刷新。
  const { data: novels = [], refetch: refetchNovels } = useNovels();
  // 5.3 pattern commit 1: chapters/models/settings 走 query（共享 5.1/5.2 缓存），废弃 useApp + load 三件套。
  //   targetNovelId 变化时 useChapters 自动 refetch（queryKey 含 novelId）；novelId=0 时 enabled 守卫不 fetch，chapters 兜底空数组。
  //   GET 错误由全局中间件接管（queryErrorToast.ts），组件不挂 console.error。
  const { data: chapters = [] } = useChapters(targetNovelId);
  const modelsQuery = useModels();
  const settingsQuery = useSettings();
  const models = useMemo(() => modelsQuery.data ?? [], [modelsQuery.data]);
  const [selected, setSelected] = useState<Set<number>>(new Set());
  const [scope, setScope] = useState<Scope>("all");
  // 结构化选中模型（含 ProviderName/ModelID），替代 modelKey 字符串 + find。
  const [selectedModel, setSelectedModel] = useState<llm.AvailableModel | null>(
    null,
  );
  // reasoningEffort：跟随选中模型。model 支持时默认第一档，不支持时空串。
  const [reasoningEffort, setReasoningEffort] = useState("");

  useEffect(() => {
    if (!userSelectedNovelRef.current) {
      setTargetNovelId(currentNovelId);
    }
  }, [currentNovelId]);

  // targetNovelId 变化时清空 selected（替代原 loadChapters useEffect 内的 setSelected(new Set())）。
  useEffect(() => {
    setSelected(new Set());
  }, [targetNovelId]);

  // 从 settings 恢复选中模型（models + settings query ready 后回填，替代手动 GetModels/GetSettings fetch）。
  useEffect(() => {
    if (!selectedModel && models.length > 0 && settingsQuery.data) {
      const key = settingsQuery.data.selected_model_key || "";
      setSelectedModel(models.find((m) => m.Key === key) ?? models[0] ?? null);
    }
  }, [models, settingsQuery.data, selectedModel]);
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
    targetNovelId > 0 && activeChapterCount >= 5 && !!selectedModel;
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
    if (!canExtract || !selectedModel) return;
    // 结构化入参：直接取 selectedModel.ProviderName/ModelID（不再 find + splitModelKey）。
    const taskId = createPatternTaskID();
    setSessionParams({
      taskId,
      novelId: targetNovelId,
      chapterIds: activeChapterIds,
      providerName: selectedModel.ProviderName,
      modelId: selectedModel.ModelID,
      reasoningEffort,
      title: targetNovelTitle || t("extract.progress.unknownWork"),
      chapterCount: activeChapterCount,
    });
    setView("session");
  }, [
    activeChapterCount,
    activeChapterIds,
    canExtract,
    reasoningEffort,
    selectedModel,
    t,
    targetNovelId,
    targetNovelTitle,
  ]);

  // 切 model：若支持 reasoning 默认第一档，不支持时清空（与 ChatControls 一致）。
  const handleSelectModel = useCallback(
    (key: string) => {
      const m = models.find((item) => item.Key === key);
      if (!m) return;
      setSelectedModel(m);
      if (m.ReasoningLevels?.length) {
        setReasoningEffort(m.ReasoningLevels[0]);
      } else {
        setReasoningEffort("");
      }
    },
    [models],
  );

  const handleSelectEffort = useCallback((effort: string) => {
    setReasoningEffort(effort);
  }, []);

  if (view === "session" && sessionParams) {
    return (
      <PatternSessionView
        taskId={sessionParams.taskId}
        novelId={sessionParams.novelId}
        chapterIds={sessionParams.chapterIds}
        providerName={sessionParams.providerName}
        modelId={sessionParams.modelId}
        reasoningEffort={sessionParams.reasoningEffort}
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
              refetchNovels();
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
          <ModelPicker
            models={models}
            selectedKey={selectedModel?.Key ?? ""}
            reasoningEffort={reasoningEffort}
            onSelectModel={handleSelectModel}
            onSelectEffort={handleSelectEffort}
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

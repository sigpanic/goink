import { useState, useEffect, useCallback, useRef, useMemo } from "react";
import {
  Plus,
  Sparkle,
  Loader2,
  BarChart3,
  ChevronLeft,
  ChevronRight,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import { toastError } from "@/utils/toast";
import { toErrorMessage } from "@/utils/error";
import { ExtractStyle, CancelExtract } from "@/lib/wailsjs/go/app/App";
import { useModels } from "@/components/settings/useModels";
import { useSettings } from "@/components/settings/useSettings";
import { useSaveContent } from "@/components/content/useSaveContent";
import type { llm } from "@/lib/wailsjs/go/models";
import { useNovels } from "@/components/novel/useNovels";
import { useStyleSamples } from "./useStyleSamples";
import { useStyleSample } from "./useStyleSample";
import { useCreateStyleSample } from "./useCreateStyleSample";
import { useUpdateStyleSample } from "./useUpdateStyleSample";
import { useDeleteStyleSample } from "./useDeleteStyleSample";
import StyleSampleCard from "./StyleSampleCard";
import Markdown from "@/components/Markdown";
import { splitFrontmatter } from "@/components/content/types";
import PopSelect from "@/components/chat/PopSelect";
import TagInput from "@/components/shared/TagInput";
import ConfirmDialog from "@/components/ui/ConfirmDialog";

const PAGE_SIZE = 15;

interface Props {
  focusId?: number | null;
  onFocusHandled?: () => void;
  embedded?: boolean;
  novelId?: number;
}

type Phase = "browse" | "adding" | "extracting" | "preview";

export default function StyleView({
  focusId,
  onFocusHandled,
  embedded = false,
  novelId = 0,
}: Props) {
  // 5.3 commit 4: model 走 useModels/useSettings query（共享 5.1 chat 缓存）+ SaveContent 走 useSaveContent mutation（5.2），废弃 splitModelKey。
  //   流式/命令调用仍直接 import wailsjs：ExtractStyle/CancelExtract（流式）
  //   已迁 query + mutation：List/Get/Create/Update/DeleteStyleSample + GetModels/GetSettings + SaveContent
  const { t } = useTranslation();
  const runningTaskIdRef = useRef<string | null>(null);

  // CRUD 走 mutation（onSuccess 内部 invalidateQueries，组件不再手动 qc.invalidate + setLoading 三件套）。
  const createMutation = useCreateStyleSample();
  const updateMutation = useUpdateStyleSample();
  const deleteMutation = useDeleteStyleSample();
  // SaveContent 走 content 领域 mutation（5.2 建，onSuccess 失效 contentKeys.detail 保持缓存一致）。
  const saveMutation = useSaveContent();

  const [page, setPage] = useState(1);
  const [selected, setSelected] = useState<Set<number>>(new Set());
  const [phase, setPhase] = useState<Phase>("browse");
  const [deleteTarget, setDeleteTarget] = useState<{
    id: number;
    name: string;
  } | null>(null);

  // add form
  const [newName, setNewName] = useState("");
  const [newContent, setNewContent] = useState("");
  const [newNovelId, setNewNovelId] = useState(0);
  const [newTags, setNewTags] = useState<string[]>([]);

  // extract: models/settings 走 query（共享 5.1 chat 缓存），废弃手动 fetch + splitModelKey。
  const modelsQuery = useModels();
  const settingsQuery = useSettings();
  const models = useMemo(() => modelsQuery.data ?? [], [modelsQuery.data]);
  // 结构化选中模型（含 ProviderName/ModelID），替代 modelKey 字符串 + useMemo find。
  const [selectedModel, setSelectedModel] = useState<llm.AvailableModel | null>(
    null,
  );
  const [error, setError] = useState("");
  const [result, setResult] = useState<{
    name: string;
    filePath: string;
    rawContent: string;
  } | null>(null);

  // 3.9: novels 走 useNovels query（与 WorkspaceView/GeneralConfigTab/PatternExtractView 共享缓存）。
  const { data: novels = [] } = useNovels();

  // samples 走 useStyleSamples query（不再 useApp.ListStyleSamples + loadRef 三件套）。
  // queryKey 含 page/size/search，page 变化触发新 query；novelId 变化自动 refetch（queryKey 含 novelId）。
  // GET 错误由全局中间件接管（queryErrorToast.ts），组件不挂 toastError。
  // CRUD 后由 mutation 的 onSuccess invalidateQueries 同步缓存。
  const samplesQuery = useStyleSamples({
    novelId,
    page,
    size: PAGE_SIZE,
    search: "",
  });
  const samples = useMemo(
    () => samplesQuery.data?.items ?? [],
    [samplesQuery.data],
  );
  const total = samplesQuery.data?.total ?? 0;
  const totalPages = samplesQuery.data?.total_pages ?? 0;

  const novelOptions = useMemo(
    () => [
      { value: "0", label: t("styleSample.global") },
      ...novels.map((n) => ({ value: String(n.id), label: n.title })),
    ],
    [novels, t],
  );

  // detail/edit dialog
  const [detailId, setDetailId] = useState<number | null>(null);
  const [editName, setEditName] = useState("");
  const [editContent, setEditContent] = useState("");
  const [editTags, setEditTags] = useState<string[]>([]);
  const [editNovelId, setEditNovelId] = useState(0);

  // detail 走 useStyleSample query（不再 useApp.GetStyleSample 手动 fetch）。
  // openDetail 只 setDetailId 触发 query，useEffect 监听 query.data ready 回填编辑字段。
  // GET 错误由中间件接管，弹窗内 inline 显示错误文案（三分支）。
  const sampleQuery = useStyleSample(detailId);

  useEffect(() => {
    if (sampleQuery.data) {
      setEditName(sampleQuery.data.name);
      setEditContent(sampleQuery.data.content);
      setEditTags(sampleQuery.data.tags || []);
      setEditNovelId(
        sampleQuery.data.is_global ? 0 : sampleQuery.data.novel_id,
      );
    }
  }, [sampleQuery.data]);

  const openDetail = useCallback((id: number) => {
    setDetailId(id);
  }, []);

  useEffect(() => {
    if (focusId) {
      openDetail(focusId);
      onFocusHandled?.();
    }
  }, [focusId, openDetail, onFocusHandled]);

  // 从 settings 恢复选中模型（models + settings query ready 后回填，替代手动 GetSettings fetch）。
  useEffect(() => {
    if (!selectedModel && models.length > 0 && settingsQuery.data) {
      const key = settingsQuery.data.selected_model_key || "";
      setSelectedModel(models.find((m) => m.Key === key) ?? models[0] ?? null);
    }
  }, [models, settingsQuery.data, selectedModel]);

  const toggleSelect = useCallback((id: number) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  const toggleSelectAll = useCallback(() => {
    if (selected.size === samples.length && samples.length > 0) {
      setSelected(new Set());
    } else {
      setSelected(new Set(samples.map((s) => s.id)));
    }
  }, [selected.size, samples]);

  const handleAdd = useCallback(async () => {
    if (!newName.trim() || !newContent.trim()) return;
    try {
      const isGlobal = newNovelId === 0;
      await createMutation.mutateAsync({
        novel_id: newNovelId,
        is_global: isGlobal,
        name: newName.trim(),
        content: newContent.trim(),
        tags: newTags,
      });
      setNewName("");
      setNewContent("");
      setNewNovelId(0);
      setNewTags([]);
      setPhase("browse");
      setPage(1);
    } catch (e) {
      setError(toErrorMessage(e, t("styleSample.addFailed")));
    }
  }, [newName, newContent, newNovelId, newTags, createMutation.mutateAsync, t]);

  const handleDelete = useCallback((id: number, name: string) => {
    setDeleteTarget({ id, name });
  }, []);

  const confirmDelete = useCallback(async () => {
    if (!deleteTarget) return;
    try {
      await deleteMutation.mutateAsync({ id: deleteTarget.id });
      setSelected((prev) => {
        const n = new Set(prev);
        n.delete(deleteTarget.id);
        return n;
      });
      setDeleteTarget(null);
    } catch (err) {
      toastError(t("styleSample.deleteFailed") + ": " + toErrorMessage(err));
      console.error(err);
    }
  }, [deleteTarget, deleteMutation.mutateAsync, t]);

  const handleExtract = useCallback(async () => {
    if (selected.size === 0 || !selectedModel) return;

    if (phase === "extracting") {
      const runningTaskId = runningTaskIdRef.current;
      if (runningTaskId !== null) CancelExtract(runningTaskId);
      runningTaskIdRef.current = null;
      setPhase("browse");
      return;
    }

    const taskId =
      typeof crypto !== "undefined" && "randomUUID" in crypto
        ? crypto.randomUUID()
        : "style-extract-" + Date.now();

    setPhase("extracting");
    setError("");
    runningTaskIdRef.current = taskId;

    try {
      const res = await ExtractStyle({
        task_id: taskId,
        sample_ids: [...selected],
        provider_name: selectedModel.ProviderName,
        model_id: selectedModel.ModelID,
        reasoning_effort: "",
      });
      if (runningTaskIdRef.current !== taskId) return;
      setResult({
        name: res.name,
        filePath: res.file_path,
        rawContent: res.raw_content,
      });
      setPhase("preview");
    } catch (e) {
      if (runningTaskIdRef.current !== taskId) return;
      const msg = toErrorMessage(e, "");
      if (!msg.toLowerCase().includes("cancel") && !msg.includes("取消")) {
        setError(msg || t("styleSample.extractFailed"));
      }
      setPhase("browse");
    } finally {
      if (runningTaskIdRef.current === taskId) {
        runningTaskIdRef.current = null;
      }
    }
  }, [selected, selectedModel, phase, t]);

  const handleSave = useCallback(async () => {
    if (!result) return;
    try {
      await saveMutation.mutateAsync({
        novel_id: novelId,
        path: result.filePath,
        content: result.rawContent,
      });
      setPhase("browse");
      setResult(null);
      setSelected(new Set());
    } catch (e) {
      setError(toErrorMessage(e, t("styleSample.saveFailed")));
    }
  }, [result, saveMutation.mutateAsync, t, novelId]);

  const handleUpdate = useCallback(async () => {
    if (!detailId) return;
    try {
      const isGlobal = editNovelId === 0;
      await updateMutation.mutateAsync({
        id: detailId,
        name: editName,
        content: editContent,
        tags: editTags,
        is_global: isGlobal,
        novel_id: editNovelId,
      });
      setDetailId(null);
    } catch (e) {
      setError(toErrorMessage(e, t("styleSample.saveFailed")));
    }
  }, [
    detailId,
    editName,
    editContent,
    editTags,
    editNovelId,
    updateMutation.mutateAsync,
    t,
  ]);

  const { meta, body } = result
    ? splitFrontmatter(result.rawContent)
    : { meta: {}, body: "" };

  return (
    <div
      className={`flex-1 flex flex-col min-h-0 ${embedded ? "" : "bg-background"}`}
    >
      {/* 顶部工具栏 */}
      <div className="flex items-center justify-between px-6 py-4 border-b shrink-0">
        {phase === "preview" && result ? (
          <>
            <span className="text-sm text-muted-foreground flex items-center gap-2">
              <Sparkle className="w-4 h-4 text-primary" />
              {t("styleSample.generated")}「{result.name}」
            </span>
            <div className="flex items-center gap-2">
              <button
                onClick={() => {
                  setPhase("browse");
                  setResult(null);
                  setSelected(new Set());
                }}
                className="h-8 px-3 rounded-lg text-sm border border-border hover:bg-muted transition-colors"
              >
                {t("styleSample.cancel")}
              </button>
              <button
                onClick={() => {
                  setPhase("browse");
                  setResult(null);
                  setSelected(new Set());
                }}
                className="h-8 px-3 rounded-lg text-sm border border-border hover:bg-muted transition-colors"
              >
                {t("styleSample.reExtract")}
              </button>
              <button
                onClick={handleSave}
                disabled={saveMutation.isPending}
                className="h-8 px-5 rounded-lg text-sm font-medium bg-action-save text-action-save-foreground hover:bg-action-save/80 disabled:opacity-50 transition-colors"
              >
                {saveMutation.isPending
                  ? t("styleSample.saving")
                  : t("styleSample.saveToUserSkill")}
              </button>
            </div>
          </>
        ) : (
          <>
            <span className="text-sm text-muted-foreground">
              {selected.size > 0 ? (
                <>
                  {t("styleSample.totalSamples", { count: total })}
                  <span className="ml-2 text-primary">
                    ·{" "}
                    {t("styleSample.selectedSamples", { count: selected.size })}
                  </span>
                </>
              ) : total > 0 ? (
                t("styleSample.selectHint")
              ) : null}
            </span>
            <div className="flex items-center gap-2">
              {phase !== "adding" && samples.length > 0 && (
                <button
                  onClick={toggleSelectAll}
                  className="inline-flex items-center gap-1 h-8 px-3 rounded-lg text-sm
                    border border-border hover:bg-muted transition-colors"
                >
                  {selected.size === samples.length
                    ? t("styleSample.deselectAll")
                    : t("styleSample.selectAll")}
                </button>
              )}
              {phase !== "adding" && selected.size > 0 && (
                <>
                  <PopSelect
                    value={selectedModel?.Key ?? ""}
                    options={models.map((m) => ({
                      value: m.Key,
                      label: m.ModelName,
                    }))}
                    onChange={(key) => {
                      const m = models.find((item) => item.Key === key);
                      if (m) setSelectedModel(m);
                    }}
                    minWidth="140px"
                    dropUp={false}
                  />
                  <button
                    onClick={handleExtract}
                    className={`inline-flex items-center gap-1.5 h-8 px-4 rounded-lg text-sm font-medium transition-colors shadow-sm
                      ${
                        phase === "extracting"
                          ? "bg-destructive text-destructive-foreground hover:bg-destructive/80"
                          : "bg-action-extract text-action-extract-foreground hover:bg-action-extract/80"
                      }`}
                  >
                    {phase === "extracting" ? (
                      <>
                        <Loader2 className="w-3.5 h-3.5 animate-spin" />
                        {t("styleSample.cancel")}
                      </>
                    ) : (
                      <>
                        <Sparkle className="w-3.5 h-3.5" />
                        {t("styleSample.startExtract")}
                      </>
                    )}
                  </button>
                </>
              )}
              {phase !== "adding" && (
                <button
                  onClick={() => {
                    setPhase("adding");
                    setError("");
                  }}
                  className="inline-flex items-center gap-1.5 h-8 px-3 rounded-lg text-sm
                    border border-border hover:bg-muted transition-colors"
                >
                  <Plus className="w-4 h-4" />
                  {t("styleSample.addSample")}
                </button>
              )}
            </div>
          </>
        )}
      </div>

      {error && (
        <div className="mx-6 mt-3 px-3 py-2 text-xs text-destructive bg-danger-bg border border-danger-border rounded-md shrink-0">
          {error}
        </div>
      )}

      {/* 添加素材面板 */}
      {phase === "adding" && (
        <div className="flex-1 min-h-0 flex flex-col mx-6 my-4 p-4 rounded-xl border bg-card/50 backdrop-blur-sm">
          <div className="flex gap-3 shrink-0">
            <input
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              placeholder={t("styleSample.sampleNamePlaceholder")}
              className="flex-1 px-3 py-2 text-sm rounded-lg border bg-background outline-none focus:ring-2 focus:ring-ring"
              autoFocus
            />
            <PopSelect
              value={String(newNovelId)}
              options={novelOptions}
              onChange={(v: string) => setNewNovelId(Number(v))}
              minWidth="120px"
              dropUp={false}
            />
            <button
              onClick={() => {
                setPhase("browse");
                setNewName("");
                setNewContent("");
                setNewNovelId(0);
                setNewTags([]);
              }}
              className="h-9 px-3 text-sm border rounded-md hover:bg-muted transition-colors"
            >
              {t("styleSample.cancel")}
            </button>
            <button
              onClick={handleAdd}
              disabled={
                !newName.trim() ||
                !newContent.trim() ||
                createMutation.isPending
              }
              className="h-9 px-4 text-sm font-medium rounded-md bg-primary text-primary-foreground hover:opacity-90 disabled:opacity-40 transition-opacity"
            >
              {createMutation.isPending
                ? t("styleSample.saving")
                : t("styleSample.addSample")}
            </button>
          </div>
          <div className="mt-2">
            <TagInput
              tags={newTags}
              onChange={setNewTags}
              placeholder={t("styleSample.tagPlaceholder")}
              size="md"
            />
          </div>
          <textarea
            value={newContent}
            onChange={(e) => setNewContent(e.target.value)}
            placeholder={t("styleSample.bodyPlaceholder")}
            className="w-full flex-1 mt-3 px-3 py-2.5 text-sm rounded-lg border bg-background resize-none outline-none focus:ring-2 focus:ring-ring"
          />
        </div>
      )}

      {/* 预览结果 */}
      {phase === "preview" && result && (
        <div className="flex-1 min-h-0 overflow-y-auto px-6 py-4 space-y-3">
          {Object.keys(meta).length > 0 && (
            <table className="border bg-muted/20 w-full text-sm rounded-lg overflow-hidden">
              <tbody>
                {meta.name && (
                  <tr className="border-b">
                    <td className="px-4 py-2 text-muted-foreground whitespace-nowrap w-16">
                      {t("styleSample.name")}
                    </td>
                    <td className="px-4 py-2 text-foreground font-semibold">
                      {meta.name}
                    </td>
                  </tr>
                )}
                {meta.description && (
                  <tr className="border-b">
                    <td className="px-4 py-2 text-muted-foreground whitespace-nowrap w-16">
                      {t("styleSample.summary")}
                    </td>
                    <td className="px-4 py-2 text-foreground">
                      {meta.description}
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
      )}

      {/* 素材卡片网格 */}
      {(phase === "browse" || phase === "extracting") && (
        <div className="flex-1 flex flex-col min-h-0">
          <div className="flex-1 overflow-y-auto overscroll-contain px-6 py-6">
            {samplesQuery.isLoading ? (
              <div className="flex items-center justify-center h-full text-muted-foreground">
                <Loader2 className="w-6 h-6 animate-spin" />
              </div>
            ) : samplesQuery.isError ? (
              <div className="flex flex-col items-center justify-center h-full text-muted-foreground gap-3">
                <BarChart3 className="w-12 h-12 opacity-20" />
                <p className="text-sm text-destructive">
                  {t("styleSample.loadFailed")}
                </p>
              </div>
            ) : samples.length === 0 ? (
              <div className="flex flex-col items-center justify-center h-full text-muted-foreground gap-3">
                <BarChart3 className="w-12 h-12 opacity-20" />
                <p className="text-sm">{t("styleSample.noStyleSamples")}</p>
              </div>
            ) : (
              <div className="grid grid-cols-[repeat(auto-fill,minmax(280px,1fr))] gap-4">
                {samples.map((s) => (
                  <StyleSampleCard
                    key={s.id}
                    sample={s}
                    selected={selected.has(s.id)}
                    onToggle={() => toggleSelect(s.id)}
                    onDelete={() => handleDelete(s.id, s.name)}
                    onClick={() => openDetail(s.id)}
                  />
                ))}
              </div>
            )}
          </div>

          {/* 分页控件 */}
          {totalPages > 1 && (
            <div className="flex items-center justify-center gap-2 px-6 py-3 border-t shrink-0">
              <button
                onClick={() => setPage(page - 1)}
                disabled={page <= 1}
                className="h-7 w-7 flex items-center justify-center rounded-md border border-border hover:bg-muted disabled:opacity-30 transition-colors"
              >
                <ChevronLeft className="w-4 h-4" />
              </button>
              <span className="text-xs text-muted-foreground">
                {page} / {totalPages}
              </span>
              <button
                onClick={() => setPage(page + 1)}
                disabled={page >= totalPages}
                className="h-7 w-7 flex items-center justify-center rounded-md border border-border hover:bg-muted disabled:opacity-30 transition-colors"
              >
                <ChevronRight className="w-4 h-4" />
              </button>
              <span className="text-xs text-muted-foreground ml-2">
                {t("styleSample.totalSamples", { count: total })}
              </span>
            </div>
          )}
        </div>
      )}

      {/* 详情编辑弹窗 */}
      {detailId && (
        <div className="fixed inset-0 z-50 flex items-center justify-center">
          <div
            className="absolute inset-0 bg-black/40"
            onClick={() => setDetailId(null)}
          />
          <div className="relative bg-background rounded-xl shadow-2xl border w-[900px] max-w-[96vw] h-[88vh] max-h-[96vh] flex flex-col">
            <div className="flex items-center justify-between px-5 py-3.5 border-b shrink-0">
              <h2 className="text-sm font-semibold">
                {t("styleSample.editSample")}
              </h2>
              <button
                onClick={() => setDetailId(null)}
                className="w-7 h-7 flex items-center justify-center rounded-md text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
              >
                ✕
              </button>
            </div>
            <div className="flex-1 min-h-0 flex flex-col px-5 py-4 gap-3">
              {sampleQuery.isLoading ? (
                <div className="flex-1 flex items-center justify-center text-muted-foreground">
                  <Loader2 className="w-6 h-6 animate-spin" />
                </div>
              ) : sampleQuery.isError ? (
                <div className="flex-1 flex items-center justify-center">
                  <p className="text-sm text-destructive">
                    {t("styleSample.loadFailed")}
                  </p>
                </div>
              ) : (
                <>
                  <div className="flex gap-3">
                    <div className="flex-1">
                      <label className="text-xs text-muted-foreground mb-1 block">
                        {t("styleSample.name")}
                      </label>
                      <input
                        value={editName}
                        onChange={(e) => setEditName(e.target.value)}
                        className="w-full px-3 py-2 text-sm rounded-lg border bg-background outline-none focus:ring-2 focus:ring-ring"
                      />
                    </div>
                    <div>
                      <label className="text-xs text-muted-foreground mb-1 block">
                        {t("styleSample.belongTo")}
                      </label>
                      <PopSelect
                        value={String(editNovelId)}
                        options={novelOptions}
                        onChange={(v: string) => setEditNovelId(Number(v))}
                        minWidth="120px"
                        dropUp={false}
                      />
                    </div>
                  </div>
                  <div>
                    <label className="text-xs text-muted-foreground mb-1 block">
                      {t("styleSample.tags")}
                    </label>
                    <TagInput
                      tags={editTags}
                      onChange={setEditTags}
                      placeholder={t("styleSample.tagPlaceholder")}
                      size="md"
                    />
                  </div>
                  <div className="flex-1 flex flex-col min-h-0">
                    <label className="text-xs text-muted-foreground mb-1 block shrink-0">
                      {t("styleSample.body")}
                    </label>
                    <textarea
                      value={editContent}
                      onChange={(e) => setEditContent(e.target.value)}
                      className="w-full flex-1 px-3 py-2.5 text-sm rounded-lg border bg-background resize-none outline-none focus:ring-2 focus:ring-ring font-mono leading-relaxed"
                    />
                  </div>
                </>
              )}
            </div>
            <div className="flex justify-end gap-2 px-5 py-3.5 border-t shrink-0">
              <button
                onClick={() => setDetailId(null)}
                className="h-9 px-4 rounded-md text-sm border hover:bg-muted transition-colors"
              >
                {t("styleSample.cancel")}
              </button>
              <button
                onClick={handleUpdate}
                disabled={updateMutation.isPending || !sampleQuery.data}
                className="h-9 px-5 rounded-md text-sm font-medium bg-primary text-primary-foreground hover:opacity-90 disabled:opacity-40 transition-opacity"
              >
                {updateMutation.isPending
                  ? t("styleSample.saving")
                  : t("common.save")}
              </button>
            </div>
          </div>
        </div>
      )}

      <ConfirmDialog
        open={deleteTarget !== null}
        title={t("common.confirmDelete")}
        message={
          deleteTarget
            ? `${t("styleSample.confirmDeleteSample")}「${deleteTarget.name}」？`
            : ""
        }
        danger
        loading={deleteMutation.isPending}
        confirmText={t("common.delete")}
        onClose={() => setDeleteTarget(null)}
        onConfirm={confirmDelete}
      />
    </div>
  );
}

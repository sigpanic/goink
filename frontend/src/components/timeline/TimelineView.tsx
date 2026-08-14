import { useEffect, useMemo, useState } from "react";
import {
  AlertTriangle,
  BookOpen,
  Flag,
  Lightbulb,
  Pencil,
  Plus,
  Target,
  Trash2,
  X,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import type { timeline } from "@/lib/wailsjs/go/models";
import { toastError } from "@/utils/toast";
import { toErrorMessage } from "@/utils/error";
import AutoGrowTextarea from "@/components/ui/AutoGrowTextarea";
import ConfirmDialog from "@/components/ui/ConfirmDialog";
import { useFocusWithNonce } from "@/hooks/useFocusWithNonce";
import { timelineKeys, chapterPlanKeys, maxChapterKeys } from "@/lib/queryKeys";
import { useTimelineEntries } from "./useTimelineEntries";
import { useChapterPlans } from "./useChapterPlans";
import { useDeleteTimelineEntry } from "./useDeleteTimelineEntry";
import { useCreateTimelineEntry } from "./useCreateTimelineEntry";
import { useUpdateTimelineEntry } from "./useUpdateTimelineEntry";
import { useSaveChapterPlan } from "./useSaveChapterPlan";
// useMaxChapterNumber 跨领域复用：storyarc 4.3 先建，timeline 共用
// （GetMaxChapterNumber 同一 API，maxChapterKeys 共享缓存）。
import { useMaxChapterNumber } from "../storyarc/useMaxChapterNumber";

interface Props {
  novelId: number;
}

type Tab = "next" | "near" | "far";
type Filter = "all" | "pending" | "resolved" | "abandoned";

const ENTRY_WINDOW = 20;

const FILTERS: { key: Filter; label: string }[] = [
  { key: "all", label: "timeline.all" },
  { key: "pending", label: "timeline.inProgress" },
  { key: "resolved", label: "timeline.recovered" },
  { key: "abandoned", label: "timeline.abandoned" },
];

const PLAN_LABELS: Record<Tab, string> = {
  next: "timeline.nextChapter",
  near: "timeline.nearTerm",
  far: "timeline.farTerm",
};
// 4b: CATEGORIES 只存数据值，label 由 t("timeline." + value) 动态拼接（与搜索 subtitle 同机制）。
const CATEGORIES = ["foreshadowing", "user_directive"];
const STATUSES = [
  { value: "pending", label: "timeline.inProgress" },
  { value: "resolved", label: "timeline.recovered" },
  { value: "abandoned", label: "timeline.abandoned" },
];
const IMPORTANCES = [1, 2, 3, 4, 5];

function importStars(v: number) {
  return "★".repeat(Math.max(0, Math.min(5, v)));
}

type EditMode =
  | { type: "create" }
  | { type: "edit"; entry: timeline.TimelineEntry }
  | { type: "plan"; scope: string; content: string }
  | null;

type EditForm = {
  title: string;
  content: string;
  target_chapter: number;
  importance: number;
  status: string;
  resolved_chapter: number;
  // create-only
  category?: string;
  source_chapter?: number;
  source?: string;
};

const EDIT_FORM_EMPTY: EditForm = {
  title: "",
  content: "",
  target_chapter: 1,
  importance: 3,
  status: "pending",
  resolved_chapter: 0,
};

export default function TimelineView({ novelId }: Props) {
  const focus = useFocusWithNonce("timeline");
  const focusEntryId = focus?.id ?? 0;
  const queryClient = useQueryClient();
  const { t } = useTranslation();

  // 4.4.1: entries/plans/maxChapter 走 query（与 TimelineList 共享缓存）。
  // 4a: query 错误 toast 由全局中间件接管（queryErrorToast.ts），此处不挂 useEffect。
  const entriesQuery = useTimelineEntries(novelId);
  const plansQuery = useChapterPlans(novelId);
  const maxChQuery = useMaxChapterNumber(novelId);
  // 4.4.2/4.4.3: CRUD 走 mutation，deleting/saving 由 mutation.isPending 推导（不再用 useState）。
  // onSuccess 失效对应 query（entry CRUD 失效 timeline；plan CRUD 失效 chapter-plans；
  // 都不失效 max-chapter：entry/plan 不影响小说最大章节号）。
  const deleteMutation = useDeleteTimelineEntry(novelId);
  const createMutation = useCreateTimelineEntry(novelId);
  const updateMutation = useUpdateTimelineEntry(novelId);
  const savePlanMutation = useSaveChapterPlan(novelId);
  const deleting = deleteMutation.isPending;
  const saving =
    createMutation.isPending ||
    updateMutation.isPending ||
    savePlanMutation.isPending;
  const entries = entriesQuery.data ?? [];
  const plans = plansQuery.data ?? [];
  const loading = entriesQuery.isLoading || plansQuery.isLoading;
  // loadFailed 只看 entries（entries 失败整列表不可用）。plans 失败时列表仍渲染（空 plan）+ toast。
  const loadFailed = entriesQuery.isError;

  const [planTab, setPlanTab] = useState<Tab>("next");
  const [filter, setFilter] = useState<Filter>("all");
  const [windowCenter, setWindowCenter] = useState(0);
  const [editMode, setEditMode] = useState<EditMode>(null);
  const [form, setForm] = useState<EditForm>(EDIT_FORM_EMPTY);
  const [createCat, setCreateCat] = useState("foreshadowing");
  const [deleteTarget, setDeleteTarget] = useState<number | null>(null);
  // 4b: 高亮声明式——focus 触发后由 state 驱动 className（参考 CharacterListView highlightedId）。
  const [highlightedId, setHighlightedId] = useState<number | null>(null);

  // 4.4.1: maxChapter 就绪后初始化 windowCenter（替代原 load() 里的 setWindowCenter）。
  useEffect(() => {
    const max = maxChQuery.data ?? 0;
    if (max > 0) setWindowCenter(Math.max(1, max));
  }, [maxChQuery.data]);

  // 4b: focus 触发后——滑窗对齐到 entry 章节 + 高亮该 entry + 滚动到卡片。
  // 高亮走 state 驱动 className（声明式），不命令式 classList.add/remove（易漏 cleanup）。
  useEffect(() => {
    if (!focusEntryId || focusEntryId <= 0 || entries.length === 0) return;
    const entry = entries.find((e) => e.id === focusEntryId);
    if (!entry) return;
    setWindowCenter(entry.target_chapter || entry.source_chapter || 1);
    setHighlightedId(focusEntryId);
    const el = document.querySelector<HTMLElement>(
      `[data-entry-id="${focusEntryId}"]`,
    );
    if (el) el.scrollIntoView({ behavior: "smooth", block: "center" });
    const timer = setTimeout(() => setHighlightedId(null), 2000);
    return () => clearTimeout(timer);
  }, [focusEntryId, entries, focus?.nonce]);

  const windowFrom = Math.max(1, windowCenter - ENTRY_WINDOW);
  const windowTo = windowCenter + ENTRY_WINDOW;

  const planMap = useMemo(() => {
    const map: Record<string, string> = { next: "", near: "", far: "" };
    for (const p of plans) {
      if (p.content) map[p.scope] = p.content;
    }
    return map;
  }, [plans]);

  const filteredEntries = useMemo(() => {
    if (filter === "all") return entries;
    return entries.filter((e) => e.status === filter);
  }, [entries, filter]);

  const grouped = useMemo(() => {
    const map = new Map<number, timeline.TimelineEntry[]>();
    for (const e of filteredEntries) {
      const ch = e.target_chapter;
      if (!map.has(ch)) map.set(ch, []);
      map.get(ch)!.push(e);
    }
    return [...map.entries()].sort(([a], [b]) => a - b);
  }, [filteredEntries]);

  const visibleChapters = grouped.filter(
    ([ch]) => ch >= windowFrom && ch <= windowTo,
  );
  const beforeChapters = grouped.filter(([ch]) => ch < windowFrom);
  const afterChapters = grouped.filter(([ch]) => ch > windowTo);
  const beforeCount = beforeChapters.reduce(
    (s, [, items]) => s + items.length,
    0,
  );
  const afterCount = afterChapters.reduce(
    (s, [, items]) => s + items.length,
    0,
  );
  const maxChapter = grouped.length > 0 ? grouped[grouped.length - 1][0] : 0;

  function shiftWindow(delta: number) {
    setWindowCenter((prev) =>
      Math.max(
        ENTRY_WINDOW + 1,
        Math.min(maxChapter - ENTRY_WINDOW, prev + delta),
      ),
    );
  }

  const statusStyle = (status: string) => {
    switch (status) {
      case "pending":
        return {
          bg: "bg-tag-blue",
          text: "text-tag-blue-foreground",
          label: t("timeline.inProgress"),
        };
      case "resolved":
        return {
          bg: "bg-tag-green",
          text: "text-tag-green-foreground",
          label: t("timeline.recovered"),
        };
      case "abandoned":
        return {
          bg: "bg-secondary",
          text: "text-muted-foreground",
          label: t("timeline.abandoned"),
        };
      default:
        return { bg: "bg-muted", text: "text-muted-foreground", label: status };
    }
  };

  const catStyle = (category: string) => {
    switch (category) {
      case "foreshadowing":
        return {
          icon: Target,
          color: "text-tag-amber-foreground",
          bg: "bg-tag-amber",
          label: t(`timeline.${category}`),
        };
      case "user_directive":
        return {
          icon: Lightbulb,
          color: "text-tag-purple-foreground",
          bg: "bg-tag-purple",
          label: t(`timeline.${category}`),
        };
      default:
        return {
          icon: Flag,
          color: "text-muted-foreground",
          bg: "bg-muted",
          label: category,
        };
    }
  };

  // ── CRUD handlers ────────────────────────────────────

  function openCreate() {
    setForm({ ...EDIT_FORM_EMPTY, target_chapter: Math.max(1, windowCenter) });
    setCreateCat("foreshadowing");
    setEditMode({ type: "create" });
  }

  function openEdit(entry: timeline.TimelineEntry) {
    setForm({
      title: entry.title,
      content: entry.content || "",
      target_chapter: entry.target_chapter,
      importance: entry.importance,
      status: entry.status,
      resolved_chapter: entry.resolved_chapter,
    });
    setEditMode({ type: "edit", entry });
  }

  function openPlanEdit(scope: string, content: string) {
    setForm({ ...EDIT_FORM_EMPTY, content });
    setEditMode({ type: "plan", scope, content });
  }

  async function handleSavePlan() {
    if (!editMode || editMode.type !== "plan") return;
    // 4.4.3: 走 mutation（onSuccess 失效 chapter-plans），删 setSaving/bumpRefresh。
    try {
      await savePlanMutation.mutateAsync({
        scope: editMode.scope,
        content: form.content,
      });
      setEditMode(null);
    } catch (err) {
      toastError(t("timeline.savePlanFailed") + ": " + toErrorMessage(err));
      console.error(err);
    }
  }

  async function handleCreate() {
    if (!form.title.trim()) {
      toastError(t("timeline.pleaseEnterTitle"));
      return;
    }
    if (!form.target_chapter) {
      toastError(t("timeline.pleaseEnterTargetChapter"));
      return;
    }
    // 4.4.3: 走 mutation（onSuccess 失效 timeline），删 setSaving/bumpRefresh。
    try {
      await createMutation.mutateAsync({
        category: createCat,
        title: form.title,
        content: form.content,
        target_chapter: form.target_chapter,
        importance: form.importance,
        source_chapter: 0,
        source: "user",
      });
      setEditMode(null);
    } catch (err) {
      toastError(t("timeline.createFailed") + ": " + toErrorMessage(err));
      console.error(err);
    }
  }

  async function handleUpdate() {
    if (!editMode || editMode.type !== "edit") return;
    if (!form.title.trim()) {
      toastError(t("timeline.pleaseEnterTitle"));
      return;
    }
    // 4.4.3: 走 mutation（onSuccess 失效 timeline），删 setSaving/bumpRefresh。
    // 全量回传 input 所有字段（§6 等价 PUT）。
    try {
      await updateMutation.mutateAsync({
        id: editMode.entry.id,
        input: {
          title: form.title,
          content: form.content,
          target_chapter: form.target_chapter,
          importance: form.importance,
          status: form.status,
          resolved_chapter:
            form.status === "resolved"
              ? form.resolved_chapter || form.target_chapter
              : 0,
        },
      });
      setEditMode(null);
    } catch (err) {
      toastError(t("timeline.updateFailed") + ": " + toErrorMessage(err));
      console.error(err);
    }
  }

  function handleDelete(entryId: number) {
    setDeleteTarget(entryId);
  }

  async function confirmDelete() {
    if (deleteTarget === null) return;
    // 4.4.2: 走 mutation（onSuccess 失效 timeline），删 setDeleting/bumpRefresh。
    try {
      await deleteMutation.mutateAsync(deleteTarget);
      setDeleteTarget(null);
    } catch (err) {
      toastError(t("timeline.deleteFailed") + ": " + toErrorMessage(err));
      console.error(err);
    }
  }

  async function handleQuickStatus(
    entry: timeline.TimelineEntry,
    newStatus: string,
  ) {
    // 4.4.3: 走 updateMutation（onSuccess 失效 timeline），删 setSaving/bumpRefresh。
    // 全量回传 input 所有字段（§6 等价 PUT）：其他字段传 entry 原值，status 传 newStatus。
    try {
      await updateMutation.mutateAsync({
        id: entry.id,
        input: {
          title: entry.title,
          content: entry.content || "",
          target_chapter: entry.target_chapter,
          importance: entry.importance,
          status: newStatus,
          resolved_chapter: newStatus === "resolved" ? entry.target_chapter : 0,
        },
      });
    } catch (err) {
      toastError(t("timeline.updateStatusFailed") + ": " + toErrorMessage(err));
      console.error(err);
    }
  }

  // ── Form fields ──────────────────────────────────────

  function renderFormFields(showCategory: boolean, showStatus: boolean) {
    return (
      <div className="space-y-3">
        {showCategory && (
          <div>
            <label className="text-xs font-medium text-muted-foreground mb-1 block">
              {t("timeline.type")}
            </label>
            <select
              value={createCat}
              onChange={(e) => setCreateCat(e.target.value)}
              className="w-full rounded-md border border-border bg-background px-2.5 py-1.5 text-xs text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              {CATEGORIES.map((c) => (
                <option key={c} value={c}>
                  {t(`timeline.${c}`)}
                </option>
              ))}
            </select>
          </div>
        )}
        <div>
          <label className="text-xs font-medium text-muted-foreground mb-1 block">
            {t("timeline.title")}
          </label>
          <input
            type="text"
            value={form.title}
            onChange={(e) => setForm((f) => ({ ...f, title: e.target.value }))}
            className="w-full rounded-md border border-border bg-background px-2.5 py-1.5 text-xs text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            placeholder={t("timeline.shortTitle")}
          />
        </div>
        <div>
          <label className="text-xs font-medium text-muted-foreground mb-1 block">
            {t("timeline.content")}
          </label>
          <AutoGrowTextarea
            value={form.content}
            onChange={(e) =>
              setForm((f) => ({ ...f, content: e.target.value }))
            }
            minHeight={60}
            maxHeight={160}
            className="w-full rounded-md border border-border bg-background px-2.5 py-1.5 text-xs text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            placeholder={t("timeline.detailedDescription")}
          />
        </div>
        <div className="flex gap-3">
          <div className="flex-1">
            <label className="text-xs font-medium text-muted-foreground mb-1 block">
              {t("timeline.targetChapter")}
            </label>
            <input
              type="number"
              value={form.target_chapter}
              onChange={(e) =>
                setForm((f) => ({
                  ...f,
                  target_chapter: parseInt(e.target.value) || 1,
                }))
              }
              min={1}
              className="w-full rounded-md border border-border bg-background px-2.5 py-1.5 text-xs text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
          </div>
          <div>
            <label className="text-xs font-medium text-muted-foreground mb-1 block">
              {t("timeline.importance")}
            </label>
            <select
              value={form.importance}
              onChange={(e) =>
                setForm((f) => ({ ...f, importance: parseInt(e.target.value) }))
              }
              className="rounded-md border border-border bg-background px-2.5 py-1.5 text-xs text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              {IMPORTANCES.map((i) => (
                <option key={i} value={i}>
                  {importStars(i)}
                </option>
              ))}
            </select>
          </div>
        </div>
        {showStatus && (
          <div>
            <label className="text-xs font-medium text-muted-foreground mb-1 block">
              {t("timeline.status")}
            </label>
            <select
              value={form.status}
              onChange={(e) =>
                setForm((f) => ({ ...f, status: e.target.value }))
              }
              className="w-full rounded-md border border-border bg-background px-2.5 py-1.5 text-xs text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              {STATUSES.map((s) => (
                <option key={s.value} value={s.value}>
                  {t(s.label)}
                </option>
              ))}
            </select>
          </div>
        )}
      </div>
    );
  }

  return (
    <main className="relative flex-1 min-w-0 overflow-y-auto overscroll-contain bg-background">
      {loading ? (
        <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
          {t("timeline.loading")}
        </div>
      ) : (
        <div className="max-w-3xl mx-auto px-5 py-6 space-y-6">
          {/* Chapter Plans */}
          <section>
            <div className="flex items-center gap-2 mb-3">
              <BookOpen className="h-4 w-4 text-tag-green-foreground" />
              <h2 className="text-sm font-semibold text-foreground">
                {t("timeline.chapterPlan")}
              </h2>
            </div>
            <div className="flex gap-1 mb-3">
              {(["next", "near", "far"] as Tab[]).map((tab) => (
                <button
                  key={tab}
                  onClick={() => setPlanTab(tab)}
                  className={`
                    px-3 py-1.5 rounded text-xs font-medium transition-colors
                    ${
                      planTab === tab
                        ? "bg-card border border-border text-foreground shadow-sm"
                        : "text-muted-foreground hover:text-foreground hover:bg-card/60"
                    }
                  `}
                >
                  {t(PLAN_LABELS[tab])}
                </button>
              ))}
            </div>
            <div className="rounded-lg border border-border bg-card p-4 min-h-[80px] relative group">
              {editMode?.type === "plan" && editMode.scope === planTab ? (
                <div className="space-y-3">
                  <AutoGrowTextarea
                    value={form.content}
                    onChange={(e) =>
                      setForm((f) => ({ ...f, content: e.target.value }))
                    }
                    minHeight={80}
                    maxHeight={200}
                    className="w-full rounded-md border border-border bg-background px-3 py-2 text-xs text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                    placeholder={t("timeline.planContent", {
                      type: t(PLAN_LABELS[planTab]),
                    })}
                  />
                  <div className="flex items-center gap-2 justify-end">
                    <button
                      onClick={() => {
                        setEditMode(null);
                        setForm(EDIT_FORM_EMPTY);
                      }}
                      className="px-3 py-1 rounded text-xs text-muted-foreground hover:text-foreground transition-colors"
                    >
                      {t("timeline.cancel")}
                    </button>
                    <button
                      onClick={handleSavePlan}
                      disabled={saving}
                      className="px-3 py-1 rounded bg-primary text-primary-foreground text-xs font-medium hover:opacity-90 transition-opacity disabled:opacity-50"
                    >
                      {saving ? t("timeline.saving") : t("timeline.save")}
                    </button>
                  </div>
                </div>
              ) : (
                <>
                  {plansQuery.isError ? (
                    <p className="text-sm text-destructive">
                      {t("timeline.chapterPlansLoadFailed")}
                    </p>
                  ) : planMap[planTab] ? (
                    <p className="text-sm text-muted-foreground leading-relaxed whitespace-pre-wrap">
                      {planMap[planTab]}
                    </p>
                  ) : (
                    <p className="text-sm text-muted-foreground">
                      {t("timeline.noPlan", { type: t(PLAN_LABELS[planTab]) })}
                    </p>
                  )}
                  <button
                    onClick={() => openPlanEdit(planTab, planMap[planTab])}
                    className="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity p-1 rounded text-muted-foreground hover:text-foreground hover:bg-secondary"
                    title={t("timeline.edit")}
                  >
                    <Pencil className="h-3.5 w-3.5" />
                  </button>
                </>
              )}
            </div>
          </section>

          {/* Timeline Entries */}
          <section>
            <div className="flex items-center justify-between mb-3">
              <div className="flex items-center gap-2">
                <AlertTriangle className="h-4 w-4 text-tag-amber-foreground" />
                <h2 className="text-sm font-semibold text-foreground">
                  {t("timeline.foreshadowingAndInstructions")}
                  <span className="ml-2 text-xs font-normal text-muted-foreground">
                    {entries.length} {t("timeline.countUnit")}
                  </span>
                </h2>
              </div>
              <div className="flex items-center gap-2">
                <span className="text-[11px] text-muted-foreground">
                  {t("sidebar.chapterRange", {
                    start: windowFrom,
                    end: windowTo,
                  })}{" "}
                  · {t("storyarc.totalChapters", { count: maxChapter })}
                </span>
                <button
                  onClick={() => {
                    // 4.4.1: refresh 按钮 invalidate 三个 query（替代原 bumpRefresh）。
                    queryClient.invalidateQueries({
                      queryKey: timelineKeys.list(novelId),
                    });
                    queryClient.invalidateQueries({
                      queryKey: chapterPlanKeys.list(novelId),
                    });
                    queryClient.invalidateQueries({
                      queryKey: maxChapterKeys.detail(novelId),
                    });
                  }}
                  className="text-xs text-muted-foreground hover:text-muted-foreground transition-colors"
                >
                  {t("timeline.refresh")}
                </button>
                <button
                  onClick={openCreate}
                  className="inline-flex items-center gap-1 px-2.5 py-1 rounded text-xs font-medium bg-primary text-primary-foreground hover:opacity-90 transition-opacity"
                >
                  <Plus className="h-3 w-3" />
                  {t("timeline.new")}
                </button>
              </div>
            </div>

            {/* Filter tabs */}
            <div className="flex gap-1 mb-4">
              {FILTERS.map((f) => (
                <button
                  key={f.key}
                  onClick={() => setFilter(f.key)}
                  className={`
                    px-3 py-1 rounded text-xs transition-colors
                    ${
                      filter === f.key
                        ? "bg-card border border-border text-foreground shadow-sm"
                        : "text-muted-foreground hover:text-foreground"
                    }
                  `}
                >
                  {t(f.label)}
                  {f.key !== "all" && (
                    <span className="ml-1 text-muted-foreground">
                      ({entries.filter((e) => e.status === f.key).length})
                    </span>
                  )}
                </button>
              ))}
            </div>

            {/* Create form */}
            {editMode?.type === "create" && (
              <div className="rounded-lg border border-border bg-card p-4 mb-4">
                <div className="flex items-center justify-between mb-3">
                  <span className="text-xs font-semibold text-foreground">
                    {t("timeline.newEntry")}
                  </span>
                  <button
                    onClick={() => {
                      setEditMode(null);
                      setForm(EDIT_FORM_EMPTY);
                    }}
                    className="p-0.5 rounded text-muted-foreground hover:text-foreground"
                  >
                    <X className="h-3.5 w-3.5" />
                  </button>
                </div>
                {renderFormFields(true, false)}
                <div className="flex items-center gap-2 justify-end mt-3">
                  <button
                    onClick={() => {
                      setEditMode(null);
                      setForm(EDIT_FORM_EMPTY);
                    }}
                    className="px-3 py-1 rounded text-xs text-muted-foreground hover:text-foreground transition-colors"
                  >
                    {t("timeline.cancel")}
                  </button>
                  <button
                    onClick={handleCreate}
                    disabled={saving || !form.title.trim()}
                    className="px-3 py-1 rounded bg-primary text-primary-foreground text-xs font-medium hover:opacity-90 transition-opacity disabled:opacity-50"
                  >
                    {saving ? t("timeline.creating") : t("timeline.create")}
                  </button>
                </div>
              </div>
            )}

            {loadFailed ? (
              <p className="text-xs text-destructive py-4">
                {t("timeline.loadFailed")}
              </p>
            ) : grouped.length === 0 ? (
              <div className="text-center py-12">
                <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-full bg-secondary text-muted-foreground">
                  <Target className="h-5 w-5" />
                </div>
                <p className="mt-2 text-sm text-muted-foreground">
                  {filter === "all"
                    ? t("timeline.noForeshadowing")
                    : t("timeline.noMatchingEntries")}
                </p>
              </div>
            ) : (
              <div className="space-y-6">
                {beforeCount > 0 && (
                  <button
                    onClick={() => shiftWindow(-ENTRY_WINDOW)}
                    className="w-full rounded-lg border border-dashed border-border bg-card/60 px-4 py-2.5 text-xs text-muted-foreground hover:bg-card hover:border-border hover:text-foreground transition-colors"
                  >
                    ←{" "}
                    {t("storyarc.earlierChapters", {
                      start: beforeChapters[0]?.[0],
                      end: beforeChapters[beforeChapters.length - 1]?.[0],
                    })}{" "}
                    · {beforeCount} {t("timeline.countUnit")}
                  </button>
                )}

                {visibleChapters.map(([ch, items]) => (
                  <div key={ch}>
                    <div className="flex items-center gap-1.5 mb-2">
                      <span className="text-xs font-medium text-muted-foreground">
                        {t("sidebar.chapterN", { n: ch })}
                      </span>
                      <span className="text-[11px] text-muted-foreground">
                        {items.length} {t("timeline.countUnit")}
                      </span>
                    </div>
                    <div className="space-y-2">
                      {items.map((entry) => {
                        const s = statusStyle(entry.status);
                        const c = catStyle(entry.category);
                        const CatIcon = c.icon;
                        const isEditing =
                          editMode?.type === "edit" &&
                          editMode.entry.id === entry.id;

                        return isEditing ? (
                          <div
                            key={entry.id}
                            className="rounded-lg border border-border bg-card p-4"
                          >
                            <div className="flex items-center justify-between mb-3">
                              <span className="text-xs font-semibold text-foreground">
                                {t("storyarc.editing")}
                                {entry.title}
                              </span>
                              <button
                                onClick={() => {
                                  setEditMode(null);
                                  setForm(EDIT_FORM_EMPTY);
                                }}
                                className="p-0.5 rounded text-muted-foreground hover:text-foreground"
                              >
                                <X className="h-3.5 w-3.5" />
                              </button>
                            </div>
                            {renderFormFields(false, true)}
                            <div className="flex items-center gap-2 justify-end mt-3">
                              <button
                                onClick={() => handleDelete(entry.id)}
                                className="px-3 py-1 rounded text-xs text-destructive hover:bg-destructive/10 transition-colors"
                                disabled={saving}
                              >
                                <Trash2 className="h-3 w-3 inline mr-1" />
                                {t("timeline.delete")}
                              </button>
                              <button
                                onClick={() => {
                                  setEditMode(null);
                                  setForm(EDIT_FORM_EMPTY);
                                }}
                                className="px-3 py-1 rounded text-xs text-muted-foreground hover:text-foreground transition-colors"
                              >
                                {t("timeline.cancel")}
                              </button>
                              <button
                                onClick={handleUpdate}
                                disabled={saving || !form.title.trim()}
                                className="px-3 py-1 rounded bg-primary text-primary-foreground text-xs font-medium hover:opacity-90 transition-opacity disabled:opacity-50"
                              >
                                {saving
                                  ? t("timeline.saving")
                                  : t("timeline.save")}
                              </button>
                            </div>
                          </div>
                        ) : (
                          <div
                            key={entry.id}
                            data-entry-id={entry.id}
                            className={`rounded-lg border border-border bg-card hover:border-border hover:shadow-sm transition-shadow group ${highlightedId === entry.id ? "ring-2 ring-primary" : ""}`}
                          >
                            <div className="flex items-center gap-3 px-4 py-3">
                              <span
                                className={`shrink-0 flex h-7 w-7 items-center justify-center rounded ${c.bg}`}
                              >
                                <CatIcon className={`h-3.5 w-3.5 ${c.color}`} />
                              </span>
                              <div className="flex-1 min-w-0">
                                <div className="flex items-center gap-2">
                                  <span className="text-sm font-medium text-foreground truncate">
                                    {entry.title}
                                  </span>
                                  <span
                                    className={`shrink-0 rounded px-1.5 py-0.5 text-[11px] font-medium ${s.bg} ${s.text}`}
                                  >
                                    {s.label}
                                  </span>
                                </div>
                                <div className="flex items-center gap-2 mt-0.5 text-[11px] text-muted-foreground">
                                  <span className="text-tag-amber-foreground text-[11px]">
                                    {importStars(entry.importance)}
                                  </span>
                                  <span>
                                    {t("timeline.targetChapterN", {
                                      n: entry.target_chapter,
                                    })}
                                  </span>
                                  {entry.source_chapter > 0 && (
                                    <span>
                                      ·{" "}
                                      {t("timeline.plantedInChapter", {
                                        n: entry.source_chapter,
                                      })}
                                    </span>
                                  )}
                                  {entry.resolved_chapter > 0 && (
                                    <span className="text-tag-green-foreground">
                                      ·{" "}
                                      {t("timeline.recoveredInChapter", {
                                        n: entry.resolved_chapter,
                                      })}
                                    </span>
                                  )}
                                  <span className="text-muted-foreground">
                                    ·{" "}
                                    {entry.source === "ai"
                                      ? t("timeline.ai")
                                      : t("timeline.user")}
                                  </span>
                                </div>
                              </div>
                              {/* Quick actions */}
                              <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                                {entry.status === "pending" && (
                                  <button
                                    onClick={() =>
                                      handleQuickStatus(entry, "resolved")
                                    }
                                    className="p-1 rounded text-muted-foreground hover:text-tag-green-foreground hover:bg-tag-green/20 transition-colors"
                                    title={t("timeline.markRecovered")}
                                  >
                                    <span className="text-[11px]">✓</span>
                                  </button>
                                )}
                                <button
                                  onClick={() => openEdit(entry)}
                                  className="p-1 rounded text-muted-foreground hover:text-foreground hover:bg-secondary transition-colors"
                                  title={t("timeline.edit")}
                                >
                                  <Pencil className="h-3.5 w-3.5" />
                                </button>
                                <button
                                  onClick={() => handleDelete(entry.id)}
                                  className="p-1 rounded text-muted-foreground hover:text-destructive hover:bg-destructive/10 transition-colors"
                                  title={t("timeline.delete")}
                                >
                                  <Trash2 className="h-3.5 w-3.5" />
                                </button>
                              </div>
                            </div>
                            {entry.content && (
                              <div className="border-t border-border px-4 py-3">
                                <p className="text-xs text-muted-foreground leading-relaxed whitespace-pre-wrap line-clamp-3">
                                  {entry.content}
                                </p>
                              </div>
                            )}
                          </div>
                        );
                      })}
                    </div>
                  </div>
                ))}

                {afterCount > 0 && (
                  <button
                    onClick={() => shiftWindow(ENTRY_WINDOW)}
                    className="w-full rounded-lg border border-dashed border-border bg-card/60 px-4 py-2.5 text-xs text-muted-foreground hover:bg-card hover:border-border hover:text-foreground transition-colors"
                  >
                    →{" "}
                    {t("storyarc.laterChapters", {
                      start: afterChapters[0]?.[0],
                      end: afterChapters[afterChapters.length - 1]?.[0],
                    })}{" "}
                    · {afterCount} {t("timeline.countUnit")}
                  </button>
                )}
              </div>
            )}
          </section>
        </div>
      )}

      <ConfirmDialog
        open={deleteTarget !== null}
        title={t("common.confirmDelete")}
        message={t("timeline.confirmDelete")}
        danger
        loading={deleting}
        confirmText={t("common.delete")}
        onClose={() => setDeleteTarget(null)}
        onConfirm={confirmDelete}
      />
    </main>
  );
}

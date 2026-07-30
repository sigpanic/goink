import { useCallback, useEffect, useMemo, useState } from "react";
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
import { useApp } from "@/hooks/useApp";
import { useRefresh } from "@/hooks/useRefresh";
import type { timeline } from "@/hooks/useApp";
import { toastError } from "@/utils/toast";
import { toErrorMessage } from "@/utils/error";
import AutoGrowTextarea from "@/components/ui/AutoGrowTextarea";
import ConfirmDialog from "@/components/ui/ConfirmDialog";

interface Props {
  novelId: number;
  focusEntryId?: number;
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
const CATEGORIES = [
  { value: "foreshadowing", label: "timeline.foreshadowing" },
  { value: "user_directive", label: "timeline.userInstruction" },
];
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
  detail_json: string;
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
  detail_json: "",
  status: "pending",
  resolved_chapter: 0,
};

export default function TimelineView({ novelId, focusEntryId }: Props) {
  const app = useApp();
  const { t } = useTranslation();
  const { bumpRefresh, refreshNonce } = useRefresh();

  const [plans, setPlans] = useState<timeline.ChapterPlan[]>([]);
  const [entries, setEntries] = useState<timeline.TimelineEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [loadFailed, setLoadFailed] = useState(false);
  const [planTab, setPlanTab] = useState<Tab>("next");
  const [filter, setFilter] = useState<Filter>("all");
  const [windowCenter, setWindowCenter] = useState(0);
  const [editMode, setEditMode] = useState<EditMode>(null);
  const [form, setForm] = useState<EditForm>(EDIT_FORM_EMPTY);
  const [createCat, setCreateCat] = useState("foreshadowing");
  const [saving, setSaving] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<number | null>(null);
  const [deleting, setDeleting] = useState(false);

  const load = useCallback(async () => {
    if (!novelId) {
      setPlans([]);
      setEntries([]);
      setLoadFailed(false);
      return;
    }
    setLoading(true);
    setLoadFailed(false);
    try {
      const [planList, entryList, maxCh] = await Promise.all([
        app.GetChapterPlans(novelId),
        app.GetTimelineEntries(novelId, 0, 0),
        app.GetMaxChapterNumber(novelId),
      ]);
      setPlans(planList ?? []);
      setEntries(entryList ?? []);
      setWindowCenter(Math.max(1, maxCh));
    } catch (err) {
      setLoadFailed(true);
      toastError(
        t("timeline.loadFailed") +
          ": " +
          toErrorMessage(err),
      );
      console.error(err);
    } finally {
      setLoading(false);
    }
  }, [app, novelId, t]);

  useEffect(() => {
    load();
  }, [load, refreshNonce]);

  useEffect(() => {
    if (focusEntryId && focusEntryId > 0 && entries.length > 0) {
      const entry = entries.find((e) => e.id === focusEntryId);
      if (entry) {
        setWindowCenter(entry.target_chapter || entry.source_chapter || 1);
      }
    }
  }, [focusEntryId, entries]);

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
          label: t("timeline.foreshadowing"),
        };
      case "user_directive":
        return {
          icon: Lightbulb,
          color: "text-tag-purple-foreground",
          bg: "bg-tag-purple",
          label: t("timeline.userInstruction"),
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
      detail_json: entry.detail_json || "",
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
    setSaving(true);
    try {
      await app.UpdateChapterPlan(novelId, {
        scope: editMode.scope,
        content: form.content,
      });
      setEditMode(null);
      await load();
      bumpRefresh();
    } catch (err) {
      toastError(
        t("timeline.savePlanFailed") +
          ": " +
          toErrorMessage(err),
      );
      console.error(err);
    } finally {
      setSaving(false);
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
    setSaving(true);
    try {
      await app.CreateTimelineEntry(novelId, {
        category: createCat,
        title: form.title,
        content: form.content,
        target_chapter: form.target_chapter,
        importance: form.importance,
        source_chapter: 0,
        detail_json: form.detail_json,
        source: "user",
      });
      setEditMode(null);
      await load();
      bumpRefresh();
    } catch (err) {
      toastError(
        t("timeline.createFailed") +
          ": " +
          toErrorMessage(err),
      );
      console.error(err);
    } finally {
      setSaving(false);
    }
  }

  async function handleUpdate() {
    if (!editMode || editMode.type !== "edit") return;
    if (!form.title.trim()) {
      toastError(t("timeline.pleaseEnterTitle"));
      return;
    }
    setSaving(true);
    try {
      const payload = {
        title: form.title,
        content: form.content,
        detail_json: form.detail_json,
        target_chapter: form.target_chapter,
        importance: form.importance,
        status: form.status,
        resolved_chapter:
          form.status === "resolved"
            ? form.resolved_chapter || form.target_chapter
            : 0,
      };
      await app.UpdateTimelineEntry(novelId, editMode.entry.id, payload);
      setEditMode(null);
      await load();
      bumpRefresh();
    } catch (err) {
      toastError(
        t("timeline.updateFailed") +
          ": " +
          toErrorMessage(err),
      );
      console.error(err);
    } finally {
      setSaving(false);
    }
  }

  function handleDelete(entryId: number) {
    setDeleteTarget(entryId);
  }

  async function confirmDelete() {
    if (deleteTarget === null) return;
    setDeleting(true);
    try {
      await app.DeleteTimelineEntry(novelId, deleteTarget);
      setDeleteTarget(null);
      await load();
      bumpRefresh();
    } catch (err) {
      toastError(
        t("timeline.deleteFailed") +
          ": " +
          toErrorMessage(err),
      );
      console.error(err);
    } finally {
      setDeleting(false);
    }
  }

  async function handleQuickStatus(
    entry: timeline.TimelineEntry,
    newStatus: string,
  ) {
    setSaving(true);
    try {
      await app.UpdateTimelineEntry(novelId, entry.id, {
        title: entry.title,
        content: entry.content || "",
        detail_json: entry.detail_json || "",
        target_chapter: entry.target_chapter,
        importance: entry.importance,
        status: newStatus,
        resolved_chapter: newStatus === "resolved" ? entry.target_chapter : 0,
      });
      await load();
      bumpRefresh();
    } catch (err) {
      toastError(
        t("timeline.updateStatusFailed") +
          ": " +
          toErrorMessage(err),
      );
      console.error(err);
    } finally {
      setSaving(false);
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
                <option key={c.value} value={c.value}>
                  {t(c.label)}
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
                  {planMap[planTab] ? (
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
                  onClick={load}
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
                            className="rounded-lg border border-border bg-card hover:border-border hover:shadow-sm transition-shadow group"
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

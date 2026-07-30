import { useCallback, useEffect, useMemo, useState } from "react";
import {
  AlertTriangle,
  BookOpen,
  Clock,
  Plus,
  Pencil,
  Trash2,
  X,
  Eye,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import { useApp } from "@/hooks/useApp";
import { useRefresh } from "@/hooks/useRefresh";
import type { reader } from "@/hooks/useApp";
import { toastError } from "@/utils/toast";
import { toErrorMessage } from "@/utils/error";
import AutoGrowTextarea from "@/components/ui/AutoGrowTextarea";
import ConfirmDialog from "@/components/ui/ConfirmDialog";

interface Props {
  novelId: number;
  focusId?: number;
}

type TypeFilter = "all" | "known" | "suspense" | "misconception";
type StatusFilter = "all" | "unrevealed" | "revealed";

const WINDOW = 20;

const TYPE_FILTERS: {
  key: TypeFilter;
  label: string;
  icon: typeof BookOpen;
  color: string;
}[] = [
  {
    key: "all",
    label: "reader.all",
    icon: BookOpen,
    color: "text-muted-foreground",
  },
  {
    key: "known",
    label: "reader.known",
    icon: BookOpen,
    color: "text-tag-green-foreground",
  },
  {
    key: "suspense",
    label: "reader.suspense",
    icon: Clock,
    color: "text-tag-amber-foreground",
  },
  {
    key: "misconception",
    label: "reader.misunderstanding",
    icon: AlertTriangle,
    color: "text-tag-rose-foreground",
  },
];

const STATUS_FILTERS: { key: StatusFilter; label: string }[] = [
  { key: "all", label: "reader.all" },
  { key: "unrevealed", label: "reader.unrecovered" },
  { key: "revealed", label: "reader.recovered" },
];

const TYPES = [
  { value: "known", label: "reader.known" },
  { value: "suspense", label: "reader.suspense" },
  { value: "misconception", label: "reader.misunderstanding" },
];

type EditMode =
  { type: "create" } | { type: "edit"; item: reader.ReaderPerspective } | null;

type EditForm = {
  type: string;
  content: string;
  related_truth: string;
  planted_chapter: number;
  revealed_chapter: number;
};

const EMPTY_FORM: EditForm = {
  type: "known",
  content: "",
  related_truth: "",
  planted_chapter: 1,
  revealed_chapter: 0,
};

function typeMeta(type: string, t: (key: string) => string) {
  switch (type) {
    case "known":
      return {
        icon: BookOpen,
        color: "text-tag-green-foreground",
        bg: "bg-tag-green",
        label: t("reader.known"),
      };
    case "suspense":
      return {
        icon: Clock,
        color: "text-tag-amber-foreground",
        bg: "bg-tag-amber",
        label: t("reader.suspense"),
      };
    case "misconception":
      return {
        icon: AlertTriangle,
        color: "text-tag-rose-foreground",
        bg: "bg-tag-rose",
        label: t("reader.misunderstanding"),
      };
    default:
      return {
        icon: BookOpen,
        color: "text-muted-foreground",
        bg: "bg-muted",
        label: type,
      };
  }
}

export default function ReaderView({ novelId, focusId }: Props) {
  const app = useApp();
  const { t } = useTranslation();
  const { bumpRefresh, refreshNonce } = useRefresh();

  const [entries, setEntries] = useState<reader.ReaderPerspective[]>([]);
  const [loading, setLoading] = useState(false);
  const [loadFailed, setLoadFailed] = useState(false);
  const [typeFilter, setTypeFilter] = useState<TypeFilter>("all");
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("all");
  const [expandedId, setExpandedId] = useState<number | null>(null);
  const [windowCenter, setWindowCenter] = useState(0);
  const [editMode, setEditMode] = useState<EditMode>(null);
  const [form, setForm] = useState<EditForm>(EMPTY_FORM);
  const [saving, setSaving] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<number | null>(null);
  const [deleting, setDeleting] = useState(false);

  const load = useCallback(async () => {
    if (!novelId) {
      setEntries([]);
      setLoadFailed(false);
      return;
    }
    setLoading(true);
    setLoadFailed(false);
    try {
      const items = await app.GetReaderPerspectives(novelId);
      setEntries(items ?? []);
      if (items && items.length > 0) {
        const maxCh = Math.max(...items.map((e) => e.planted_chapter));
        setWindowCenter((prev) => prev || maxCh);
      }
    } catch (err) {
      setLoadFailed(true);
      toastError(
        t("reader.loadFailed") +
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
    if (focusId && focusId > 0 && entries.length > 0) {
      const entry = entries.find((e) => e.id === focusId);
      if (entry) setWindowCenter(entry.planted_chapter);
    }
  }, [focusId, entries]);

  const filtered = useMemo(() => {
    let items = entries;
    if (typeFilter !== "all")
      items = items.filter((e) => e.type === typeFilter);
    if (statusFilter === "unrevealed")
      items = items.filter((e) => e.revealed_chapter === 0);
    if (statusFilter === "revealed")
      items = items.filter((e) => e.revealed_chapter > 0);
    return items;
  }, [entries, typeFilter, statusFilter]);

  const groupedDesc = useMemo(() => {
    const map = new Map<number, reader.ReaderPerspective[]>();
    for (const e of filtered) {
      const ch = e.planted_chapter;
      if (!map.has(ch)) map.set(ch, []);
      map.get(ch)!.push(e);
    }
    return [...map.entries()].sort(([a], [b]) => b - a);
  }, [filtered]);

  const windowFrom = Math.max(1, windowCenter - WINDOW);
  const windowTo = windowCenter;

  const visibleChapters = groupedDesc.filter(
    ([ch]) => ch >= windowFrom && ch <= windowTo,
  );
  const beforeChapters = groupedDesc.filter(([ch]) => ch < windowFrom);

  const beforeCount = beforeChapters.reduce(
    (s, [, items]) => s + items.length,
    0,
  );
  const maxChapter = groupedDesc.length > 0 ? groupedDesc[0][0] : 0;

  function shiftWindow(delta: number) {
    setWindowCenter((prev) =>
      Math.max(WINDOW, Math.min(maxChapter, prev + delta)),
    );
  }

  // ── CRUD handlers ────────────────────────────────────

  function openCreate() {
    setForm({ ...EMPTY_FORM, planted_chapter: Math.max(1, windowCenter) });
    setEditMode({ type: "create" });
  }

  function openEdit(item: reader.ReaderPerspective) {
    setForm({
      type: item.type,
      content: item.content,
      related_truth: item.related_truth || "",
      planted_chapter: item.planted_chapter,
      revealed_chapter: item.revealed_chapter,
    });
    setEditMode({ type: "edit", item });
  }

  async function handleCreate() {
    if (!form.content.trim()) {
      toastError(t("reader.pleaseEnterContent"));
      return;
    }
    if (!form.type) {
      toastError(t("reader.pleaseSelectType"));
      return;
    }
    setSaving(true);
    try {
      const created = await app.CreateReaderPerspective(novelId, {
        type: form.type,
        content: form.content,
        planted_chapter: form.planted_chapter,
        related_truth: form.related_truth,
        revealed_chapter: form.revealed_chapter,
      });
      setEditMode(null);
      setForm(EMPTY_FORM);
      await load();
      setExpandedId(created.id);
      bumpRefresh();
    } catch (err) {
      toastError(
        t("reader.createFailed") +
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
    if (!form.content.trim()) {
      toastError(t("reader.pleaseEnterContent"));
      return;
    }
    const entryId = editMode.item.id;
    setSaving(true);
    try {
      await app.UpdateReaderPerspective(entryId, novelId, {
        type: form.type,
        content: form.content,
        related_truth: form.related_truth,
        planted_chapter: form.planted_chapter,
        revealed_chapter: form.revealed_chapter,
      });
      setEditMode(null);
      setForm(EMPTY_FORM);
      await load();
      setExpandedId(entryId);
      bumpRefresh();
    } catch (err) {
      toastError(
        t("reader.updateFailed") +
          ": " +
          toErrorMessage(err),
      );
      console.error(err);
    } finally {
      setSaving(false);
    }
  }

  function handleDelete(id: number) {
    setDeleteTarget(id);
  }

  async function confirmDelete() {
    if (deleteTarget === null) return;
    setDeleting(true);
    try {
      await app.DeleteReaderPerspective(deleteTarget, novelId);
      if (expandedId === deleteTarget) setExpandedId(null);
      setDeleteTarget(null);
      await load();
      bumpRefresh();
    } catch (err) {
      toastError(
        t("reader.deleteFailed") +
          ": " +
          toErrorMessage(err),
      );
      console.error(err);
    } finally {
      setDeleting(false);
    }
  }

  async function handleQuickReveal(item: reader.ReaderPerspective) {
    setSaving(true);
    try {
      await app.UpdateReaderPerspective(item.id, novelId, {
        type: item.type,
        content: item.content,
        related_truth: item.related_truth || "",
        planted_chapter: item.planted_chapter,
        revealed_chapter: item.planted_chapter,
      });
      await load();
      bumpRefresh();
    } catch (err) {
      toastError(
        t("reader.updateFailed") +
          ": " +
          toErrorMessage(err),
      );
      console.error(err);
    } finally {
      setSaving(false);
    }
  }

  // ── Form fields ──────────────────────────────────────

  function renderFormFields() {
    return (
      <div className="space-y-3">
        <div>
          <label className="text-xs font-medium text-muted-foreground mb-1 block">
            {t("reader.type")}
          </label>
          <select
            value={form.type}
            onChange={(e) => setForm((f) => ({ ...f, type: e.target.value }))}
            className="w-full rounded-md border border-border bg-background px-2.5 py-1.5 text-xs text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            {TYPES.map((opt) => (
              <option key={opt.value} value={opt.value}>
                {t(opt.label)}
              </option>
            ))}
          </select>
        </div>
        <div>
          <label className="text-xs font-medium text-muted-foreground mb-1 block">
            {t("reader.contentLabel")}
          </label>
          <AutoGrowTextarea
            value={form.content}
            onChange={(e) =>
              setForm((f) => ({ ...f, content: e.target.value }))
            }
            minHeight={60}
            maxHeight={160}
            className="w-full rounded-md border border-border bg-background px-2.5 py-1.5 text-xs text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            placeholder={t("reader.contentPlaceholder")}
          />
        </div>
        <div>
          <label className="text-xs font-medium text-muted-foreground mb-1 block">
            {t("reader.authorTruth")}
          </label>
          <AutoGrowTextarea
            value={form.related_truth}
            onChange={(e) =>
              setForm((f) => ({ ...f, related_truth: e.target.value }))
            }
            minHeight={40}
            maxHeight={160}
            className="w-full rounded-md border border-border bg-background px-2.5 py-1.5 text-xs text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            placeholder={t("reader.authorTruthPlaceholder")}
          />
        </div>
        <div className="flex gap-3">
          <div className="flex-1">
            <label className="text-xs font-medium text-muted-foreground mb-1 block">
              {t("reader.plantedChapter")}
            </label>
            <input
              type="number"
              value={form.planted_chapter}
              onChange={(e) =>
                setForm((f) => ({
                  ...f,
                  planted_chapter: parseInt(e.target.value) || 1,
                }))
              }
              min={1}
              className="w-full rounded-md border border-border bg-background px-2.5 py-1.5 text-xs text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
          </div>
          <div className="flex-1">
            <label className="text-xs font-medium text-muted-foreground mb-1 block">
              {t("reader.recoveredChapterLabel")}
            </label>
            <input
              type="number"
              value={form.revealed_chapter}
              onChange={(e) =>
                setForm((f) => ({
                  ...f,
                  revealed_chapter: parseInt(e.target.value) || 0,
                }))
              }
              min={0}
              className="w-full rounded-md border border-border bg-background px-2.5 py-1.5 text-xs text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
          </div>
        </div>
      </div>
    );
  }

  return (
    <main className="flex-1 min-w-0 overflow-y-auto overscroll-contain bg-background">
      {loading ? (
        <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
          {t("reader.loading")}
        </div>
      ) : (
        <div className="max-w-3xl mx-auto px-5 py-6 space-y-5">
          {/* Header */}
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <Eye className="h-4 w-4 text-tag-blue-foreground" />
              <h2 className="text-sm font-semibold text-foreground">
                {t("reader.readerPerspective")}
                <span className="ml-2 text-xs font-normal text-muted-foreground">
                  {filtered.length} {t("reader.countUnit")}
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
                {t("reader.refresh")}
              </button>
              <button
                onClick={openCreate}
                className="inline-flex items-center gap-1 px-2.5 py-1 rounded text-xs font-medium bg-primary text-primary-foreground hover:opacity-90 transition-opacity"
              >
                <Plus className="h-3 w-3" />
                {t("reader.new")}
              </button>
            </div>
          </div>

          {/* Type filter */}
          <div className="flex gap-1">
            {TYPE_FILTERS.map((f) => {
              const Icon = f.icon;
              return (
                <button
                  key={f.key}
                  onClick={() => setTypeFilter(f.key)}
                  className={`inline-flex items-center gap-1 px-3 py-1 rounded text-xs transition-colors ${
                    typeFilter === f.key
                      ? "bg-card border border-border text-foreground shadow-sm"
                      : "text-muted-foreground hover:text-foreground"
                  }`}
                >
                  <Icon
                    className={`h-3 w-3 ${typeFilter === f.key ? f.color : ""}`}
                  />
                  {t(f.label)}
                  {f.key !== "all" && (
                    <span className="text-muted-foreground">
                      ({entries.filter((e) => e.type === f.key).length})
                    </span>
                  )}
                </button>
              );
            })}
          </div>

          {/* Status filter */}
          <div className="flex gap-1">
            {STATUS_FILTERS.map((f) => (
              <button
                key={f.key}
                onClick={() => setStatusFilter(f.key)}
                className={`px-3 py-1 rounded text-xs transition-colors ${
                  statusFilter === f.key
                    ? "bg-card border border-border text-foreground shadow-sm"
                    : "text-muted-foreground hover:text-foreground"
                }`}
              >
                {t(f.label)}
                {f.key === "unrevealed" && (
                  <span className="ml-1 text-muted-foreground">
                    ({entries.filter((e) => e.revealed_chapter === 0).length})
                  </span>
                )}
                {f.key === "revealed" && (
                  <span className="ml-1 text-muted-foreground">
                    ({entries.filter((e) => e.revealed_chapter > 0).length})
                  </span>
                )}
              </button>
            ))}
          </div>

          {/* Create form */}
          {editMode?.type === "create" && (
            <div className="rounded-lg border border-border bg-card p-4">
              <div className="flex items-center justify-between mb-3">
                <span className="text-xs font-semibold text-foreground">
                  {t("reader.newReaderEntry")}
                </span>
                <button
                  onClick={() => {
                    setEditMode(null);
                    setForm(EMPTY_FORM);
                  }}
                  className="p-0.5 rounded text-muted-foreground hover:text-foreground"
                >
                  <X className="h-3.5 w-3.5" />
                </button>
              </div>
              {renderFormFields()}
              <div className="flex items-center gap-2 justify-end mt-3">
                <button
                  onClick={() => {
                    setEditMode(null);
                    setForm(EMPTY_FORM);
                  }}
                  className="px-3 py-1 rounded text-xs text-muted-foreground hover:text-foreground transition-colors"
                >
                  {t("reader.cancel")}
                </button>
                <button
                  onClick={handleCreate}
                  disabled={saving || !form.content.trim()}
                  className="px-3 py-1 rounded text-xs font-medium bg-primary text-primary-foreground hover:opacity-90 transition-opacity disabled:opacity-50"
                >
                  {saving ? t("reader.creating") : t("reader.create")}
                </button>
              </div>
            </div>
          )}

          {/* Entries */}
          {loadFailed ? (
            <p className="text-xs text-destructive py-4">
              {t("reader.loadFailed")}
            </p>
          ) : groupedDesc.length === 0 ? (
            <div className="text-center py-12">
              <p className="text-sm text-muted-foreground">
                {entries.length === 0
                  ? t("reader.noReaderData")
                  : t("reader.noMatchingEntries")}
              </p>
            </div>
          ) : (
            <div className="space-y-6">
              {beforeCount > 0 && (
                <button
                  onClick={() => shiftWindow(-WINDOW)}
                  className="w-full rounded-lg border border-dashed border-border bg-card/60 px-4 py-2.5 text-xs text-muted-foreground hover:bg-card hover:border-border hover:text-foreground transition-colors"
                >
                  ←{" "}
                  {t("storyarc.earlierChapters", {
                    start: beforeChapters[beforeChapters.length - 1]?.[0],
                    end: beforeChapters[0]?.[0],
                  })}{" "}
                  · {beforeCount} {t("reader.countUnit")}
                </button>
              )}

              {visibleChapters.map(([ch, items]) => (
                <div key={ch}>
                  <div className="flex items-center gap-1.5 mb-2">
                    <span className="text-xs font-medium text-muted-foreground">
                      {t("sidebar.chapterN", { n: ch })}
                    </span>
                    <span className="text-[10px] text-muted-foreground">
                      {items.length} {t("reader.countUnit")}
                    </span>
                  </div>
                  <div className="space-y-2">
                    {items.map((entry) => {
                      const meta = typeMeta(entry.type, t);
                      const Icon = meta.icon;
                      const isEditing =
                        editMode?.type === "edit" &&
                        editMode.item.id === entry.id;
                      const isExpanded = expandedId === entry.id && !isEditing;
                      const isRevealed = entry.revealed_chapter > 0;

                      return isEditing ? (
                        <div
                          key={entry.id}
                          className="rounded-lg border border-border bg-card p-4"
                        >
                          <div className="flex items-center justify-between mb-3">
                            <span className="text-xs font-semibold text-foreground">
                              {t("reader.editEntry")}
                            </span>
                            <button
                              onClick={() => {
                                setEditMode(null);
                                setForm(EMPTY_FORM);
                              }}
                              className="p-0.5 rounded text-muted-foreground hover:text-foreground"
                            >
                              <X className="h-3.5 w-3.5" />
                            </button>
                          </div>
                          {renderFormFields()}
                          <div className="flex items-center gap-2 justify-end mt-3">
                            <button
                              onClick={() => handleDelete(entry.id)}
                              className="px-3 py-1 rounded text-xs text-destructive hover:bg-destructive/10 transition-colors"
                              disabled={saving}
                            >
                              <Trash2 className="h-3 w-3 inline mr-1" />
                              {t("reader.delete")}
                            </button>
                            <button
                              onClick={() => {
                                setEditMode(null);
                                setForm(EMPTY_FORM);
                              }}
                              className="px-3 py-1 rounded text-xs text-muted-foreground hover:text-foreground transition-colors"
                            >
                              {t("reader.cancel")}
                            </button>
                            <button
                              onClick={handleUpdate}
                              disabled={saving || !form.content.trim()}
                              className="px-3 py-1 rounded text-xs font-medium bg-primary text-primary-foreground hover:opacity-90 transition-opacity disabled:opacity-50"
                            >
                              {saving ? t("reader.saving") : t("reader.save")}
                            </button>
                          </div>
                        </div>
                      ) : (
                        <div
                          key={entry.id}
                          onClick={() =>
                            setExpandedId(isExpanded ? null : entry.id)
                          }
                          className={`rounded-lg border bg-card transition-shadow cursor-pointer ${
                            isExpanded
                              ? "border-border shadow-sm"
                              : "border-border hover:border-border hover:shadow-sm"
                          } group`}
                        >
                          <div className="flex items-center gap-3 px-4 py-3">
                            <span
                              className={`shrink-0 flex h-7 w-7 items-center justify-center rounded ${meta.bg}`}
                            >
                              <Icon className={`h-3.5 w-3.5 ${meta.color}`} />
                            </span>
                            <div className="flex-1 min-w-0">
                              <div className="flex items-center gap-2">
                                <span className="text-sm font-medium text-foreground truncate">
                                  {entry.content.length > 40
                                    ? entry.content.slice(0, 40) + "…"
                                    : entry.content}
                                </span>
                                <span
                                  className={`shrink-0 rounded px-1.5 py-0.5 text-[10px] font-medium ${meta.bg} ${meta.color}`}
                                >
                                  {meta.label}
                                </span>
                                {isRevealed ? (
                                  <span className="shrink-0 rounded px-1.5 py-0.5 text-[10px] font-medium bg-tag-green text-tag-green-foreground">
                                    {t("reader.recoveredInChapter", {
                                      n: entry.revealed_chapter,
                                    })}
                                  </span>
                                ) : (
                                  <span className="shrink-0 rounded px-1.5 py-0.5 text-[10px] font-medium bg-tag-blue text-tag-blue-foreground">
                                    {t("reader.unrecovered")}
                                  </span>
                                )}
                              </div>
                              <div className="flex items-center gap-2 mt-0.5 text-[11px] text-muted-foreground">
                                <span>
                                  {t("reader.plantedInChapter", {
                                    n: entry.planted_chapter,
                                  })}
                                </span>
                                {entry.related_truth && (
                                  <span className="text-muted-foreground">
                                    · {t("reader.hasTruth")}
                                  </span>
                                )}
                              </div>
                            </div>
                            {/* Quick actions */}
                            <div
                              className="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity"
                              onClick={(e) => e.stopPropagation()}
                            >
                              {!isRevealed && (
                                <button
                                  onClick={() => handleQuickReveal(entry)}
                                  className="p-1 rounded text-muted-foreground hover:text-tag-green-foreground hover:bg-tag-green/20 transition-colors"
                                  title={t("reader.markRecovered")}
                                >
                                  <span className="text-[11px]">✓</span>
                                </button>
                              )}
                              <button
                                onClick={() => {
                                  setExpandedId(null);
                                  openEdit(entry);
                                }}
                                className="p-1 rounded text-muted-foreground hover:text-foreground hover:bg-secondary transition-colors"
                                title={t("reader.edit")}
                              >
                                <Pencil className="h-3.5 w-3.5" />
                              </button>
                              <button
                                onClick={() => handleDelete(entry.id)}
                                className="p-1 rounded text-muted-foreground hover:text-destructive hover:bg-destructive/10 transition-colors"
                                title={t("reader.delete")}
                              >
                                <Trash2 className="h-3.5 w-3.5" />
                              </button>
                            </div>
                            <span
                              className={`text-[10px] transition-transform ${isExpanded ? "rotate-180" : ""}`}
                            >
                              ▼
                            </span>
                          </div>

                          {isExpanded && (
                            <div className="border-t border-border px-4 py-3 space-y-3">
                              <div>
                                <p className="text-xs text-muted-foreground mb-1">
                                  {t("reader.contentLabel")}
                                </p>
                                <p className="text-xs text-muted-foreground leading-relaxed whitespace-pre-wrap">
                                  {entry.content}
                                </p>
                              </div>
                              {entry.related_truth && (
                                <div>
                                  <p className="text-xs text-muted-foreground mb-1">
                                    {t("reader.authorTruthLabel")}
                                  </p>
                                  <p className="text-xs text-muted-foreground leading-relaxed whitespace-pre-wrap">
                                    {entry.related_truth}
                                  </p>
                                </div>
                              )}
                              {entry.revealed_chapter > 0 && (
                                <div>
                                  <p className="text-xs text-muted-foreground mb-1">
                                    {t("reader.recoveredChapterLabel")}
                                  </p>
                                  <p className="text-xs text-muted-foreground">
                                    {t("reader.chapterN", {
                                      n: entry.revealed_chapter,
                                    })}
                                  </p>
                                </div>
                              )}
                            </div>
                          )}
                        </div>
                      );
                    })}
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      <ConfirmDialog
        open={deleteTarget !== null}
        title={t("common.confirmDelete")}
        message={t("reader.confirmDelete")}
        danger
        loading={deleting}
        confirmText={t("common.delete")}
        onClose={() => setDeleteTarget(null)}
        onConfirm={confirmDelete}
      />
    </main>
  );
}

import { useCallback, useEffect, useState } from "react";
import { Pencil, Plus, Globe, Trash2, X } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useApp } from "@/hooks/useApp";
import type { setting } from "@/hooks/useApp";
import { toastError } from "@/lib/utils";

interface Props {
  novelId: number;
  focusId?: number;
}

type EditMode =
  | { type: "create" }
  | { type: "edit"; item: setting.SettingItem }
  | null;

type EditForm = {
  category: string;
  content: string;
};

const EMPTY_FORM: EditForm = { category: "", content: "" };

export default function NovelSettingView({ novelId }: Props) {
  const app = useApp();
  const { t } = useTranslation();

  const [items, setItems] = useState<setting.SettingItem[]>([]);
  const [tokenCount, setTokenCount] = useState(0);
  const [overBudget, setOverBudget] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [editMode, setEditMode] = useState<EditMode>(null);
  const [form, setForm] = useState<EditForm>(EMPTY_FORM);
  const [saving, setSaving] = useState(false);

  const load = useCallback(async () => {
    if (!novelId) {
      setItems([]);
      setTokenCount(0);
      setOverBudget(false);
      return;
    }
    setLoading(true);
    setError(null);
    try {
      const result = await app.GetNovelSettings(novelId);
      setItems(result.items ?? []);
      setTokenCount(result.token_count ?? 0);
      setOverBudget(result.over_budget ?? false);
    } catch (err) {
      setError(
        err instanceof Error ? err.message : t("novelSetting.loadFailed"),
      );
    } finally {
      setLoading(false);
    }
  }, [app, novelId, t]);

  useEffect(() => {
    load();
  }, [load]);

  // ── CRUD handlers ────────────────────────────────────

  function openCreate() {
    setError(null);
    setForm({ ...EMPTY_FORM });
    setEditMode({ type: "create" });
  }

  function openEdit(item: setting.SettingItem) {
    setError(null);
    setForm({ category: item.category, content: item.content });
    setEditMode({ type: "edit", item });
  }

  function closeForm() {
    setEditMode(null);
    setForm(EMPTY_FORM);
  }

  async function handleSave() {
    if (!editMode) return;
    if (!form.content.trim()) {
      toastError(t("novelSetting.pleaseEnterContent"));
      return;
    }

    setSaving(true);
    try {
      if (editMode.type === "create") {
        await app.CreateNovelSetting(novelId, {
          category: form.category || t("novelSetting.uncategorized"),
          content: form.content,
        });
      } else {
        await app.UpdateNovelSetting(novelId, editMode.item.id, {
          category: form.category,
          content: form.content,
        });
      }
      setEditMode(null);
      setForm(EMPTY_FORM);
      await load();
    } catch (err) {
      toastError(
        t("novelSetting.saveFailed") +
          ": " +
          (err instanceof Error ? err.message : String(err)),
      );
      console.error(err);
    } finally {
      setSaving(false);
    }
  }

  async function handleDelete(id: number) {
    if (!confirm(t("novelSetting.confirmDeleteSetting"))) return;
    setSaving(true);
    try {
      await app.DeleteNovelSetting(id);
      await load();
    } catch (err) {
      toastError(
        t("novelSetting.deleteFailed") +
          ": " +
          (err instanceof Error ? err.message : String(err)),
      );
      console.error(err);
    } finally {
      setSaving(false);
    }
  }

  // ── Render ───────────────────────────────────────────

  function renderFormFields() {
    return (
      <div className="space-y-3">
        <div>
          <label className="text-xs font-medium text-muted-foreground mb-1 block">
            {t("novelSetting.category")}
          </label>
          <input
            value={form.category}
            onChange={(e) =>
              setForm((f) => ({ ...f, category: e.target.value }))
            }
            placeholder={t("novelSetting.categoryPlaceholder")}
            className="w-full rounded-md border border-border bg-background px-2.5 py-1.5 text-xs text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
        </div>
        <div>
          <label className="text-xs font-medium text-muted-foreground mb-1 block">
            {t("novelSetting.content")}
          </label>
          <textarea
            value={form.content}
            onChange={(e) =>
              setForm((f) => ({ ...f, content: e.target.value }))
            }
            placeholder={t("novelSetting.contentPlaceholder")}
            rows={3}
            className="w-full rounded-md border border-border bg-background px-2.5 py-1.5 text-xs text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring resize-y"
          />
        </div>
      </div>
    );
  }

  return (
    <main className="flex-1 min-w-0 overflow-y-auto overscroll-contain bg-background">
      {loading ? (
        <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
          {t("novelSetting.loading")}
        </div>
      ) : error ? (
        <div className="flex h-full items-center justify-center text-sm text-destructive">
          {error}
        </div>
      ) : (
        <div className="max-w-3xl mx-auto px-5 py-6 space-y-8">
          <div className="flex items-center gap-2">
            <Globe className="h-4 w-4 text-muted-foreground" />
            <h2 className="text-sm font-semibold text-foreground">
              {t("novelSetting.title")}
              <span className="ml-2 text-xs font-normal text-muted-foreground">
                {items.length} {t("novelSetting.countUnit")}
              </span>
            </h2>
            {overBudget && (
              <span className="ml-auto text-xs text-amber-600 dark:text-amber-500">
                {t("novelSetting.tokenOverBudget", { count: tokenCount })}
              </span>
            )}
          </div>

          <section>
            <div className="flex items-center justify-between mb-3">
              <h3 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
                {t("novelSetting.sectionTitle")}
              </h3>
              {editMode?.type !== "create" && (
                <button
                  onClick={openCreate}
                  className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-muted-foreground transition-colors"
                >
                  <Plus className="h-3 w-3" /> {t("novelSetting.add")}
                </button>
              )}
            </div>

            {items.length === 0 && editMode?.type !== "create" ? (
              <p className="text-xs text-muted-foreground py-4">
                {t("novelSetting.noSetting")}
              </p>
            ) : (
              <div className="space-y-2">
                {items.map((item) => {
                  const isEditing =
                    editMode?.type === "edit" && editMode.item.id === item.id;

                  return isEditing ? (
                    <div
                      key={item.id}
                      className="rounded-lg border border-border bg-card p-4"
                    >
                      <div className="flex items-center justify-between mb-3">
                        <span className="text-xs font-semibold text-foreground">
                          {t("novelSetting.editSetting")}
                        </span>
                        <button
                          onClick={closeForm}
                          className="p-0.5 rounded text-muted-foreground hover:text-foreground"
                        >
                          <X className="h-3.5 w-3.5" />
                        </button>
                      </div>
                      {renderFormFields()}
                      <div className="flex items-center gap-2 justify-end mt-3">
                        <button
                          onClick={() => handleDelete(item.id)}
                          className="px-3 py-1 rounded text-xs text-destructive hover:bg-destructive/10 transition-colors"
                          disabled={saving}
                        >
                          <Trash2 className="h-3 w-3 inline mr-1" />
                          {t("novelSetting.delete")}
                        </button>
                        <button
                          onClick={closeForm}
                          className="px-3 py-1 rounded text-xs text-muted-foreground hover:text-foreground transition-colors"
                        >
                          {t("novelSetting.cancel")}
                        </button>
                        <button
                          onClick={handleSave}
                          disabled={saving || !form.content.trim()}
                          className="px-3 py-1 rounded text-xs font-medium bg-primary text-primary-foreground hover:opacity-90 transition-opacity disabled:opacity-50"
                        >
                          {saving
                            ? t("novelSetting.saving")
                            : t("novelSetting.save")}
                        </button>
                      </div>
                    </div>
                  ) : (
                    <div
                      key={item.id}
                      className="rounded-lg border border-border bg-card hover:border-border hover:shadow-sm transition-shadow group"
                    >
                      <div className="flex items-start gap-3 px-4 py-3">
                        <span className="shrink-0 rounded px-1.5 py-0.5 text-[10px] font-medium bg-secondary text-muted-foreground">
                          {item.category || t("novelSetting.uncategorized")}
                        </span>
                        <p className="flex-1 text-sm text-foreground leading-relaxed whitespace-pre-wrap">
                          {item.content}
                        </p>
                        <div className="shrink-0 flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                          <button
                            onClick={() => openEdit(item)}
                            className="p-1 rounded text-muted-foreground hover:text-foreground hover:bg-secondary transition-colors"
                            title={t("novelSetting.edit")}
                          >
                            <Pencil className="h-3.5 w-3.5" />
                          </button>
                          <button
                            onClick={() => handleDelete(item.id)}
                            className="p-1 rounded text-muted-foreground hover:text-destructive hover:bg-destructive/10 transition-colors"
                            title={t("novelSetting.delete")}
                          >
                            <Trash2 className="h-3.5 w-3.5" />
                          </button>
                        </div>
                      </div>
                    </div>
                  );
                })}

                {editMode?.type === "create" && (
                  <div className="rounded-lg border border-dashed border-border bg-card/60 p-4">
                    <div className="flex items-center justify-between mb-3">
                      <span className="text-xs font-semibold text-foreground">
                        {t("novelSetting.newSetting")}
                      </span>
                      <button
                        onClick={closeForm}
                        className="p-0.5 rounded text-muted-foreground hover:text-foreground"
                      >
                        <X className="h-3.5 w-3.5" />
                      </button>
                    </div>
                    {renderFormFields()}
                    <div className="flex items-center gap-2 justify-end mt-3">
                      <button
                        onClick={closeForm}
                        className="px-3 py-1 rounded text-xs text-muted-foreground hover:text-foreground transition-colors"
                      >
                        {t("common.cancel")}
                      </button>
                      <button
                        onClick={handleSave}
                        disabled={saving || !form.content.trim()}
                        className="px-3 py-1 rounded text-xs font-medium bg-primary text-primary-foreground hover:opacity-90 transition-opacity disabled:opacity-50"
                      >
                        {saving
                          ? t("novelSetting.creating")
                          : t("novelSetting.create")}
                      </button>
                    </div>
                  </div>
                )}
              </div>
            )}
          </section>
        </div>
      )}
    </main>
  );
}

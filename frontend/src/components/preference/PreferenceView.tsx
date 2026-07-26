import { useCallback, useEffect, useState } from "react";
import { Pencil, Plus, Settings, Trash2, X } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useApp } from "@/hooks/useApp";
import type { preference } from "@/hooks/useApp";

interface Props {
  novelId: number;
  focusId?: number;
}

type EditMode =
  | { type: "create"; isGlobal: boolean }
  | { type: "edit"; item: preference.PreferenceItem }
  | null;

type EditForm = {
  category: string;
  content: string;
  isGlobal: boolean;
};

const EMPTY_FORM: EditForm = { category: "", content: "", isGlobal: false };

export default function PreferenceView({ novelId }: Props) {
  const app = useApp();
  const { t } = useTranslation();

  const [global, setGlobal] = useState<preference.PreferenceItem[]>([]);
  const [novelPrefs, setNovelPrefs] = useState<preference.PreferenceItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [editMode, setEditMode] = useState<EditMode>(null);
  const [form, setForm] = useState<EditForm>(EMPTY_FORM);
  const [saving, setSaving] = useState(false);

  const load = useCallback(async () => {
    if (!novelId) {
      setGlobal([]);
      setNovelPrefs([]);
      return;
    }
    setLoading(true);
    setError(null);
    try {
      const result = await app.GetPreferences(novelId);
      setGlobal(result.global ?? []);
      setNovelPrefs(result.novel ?? []);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("preference.loadFailed"));
    } finally {
      setLoading(false);
    }
  }, [app, novelId, t]);

  useEffect(() => {
    load();
  }, [load]);

  // ── CRUD handlers ────────────────────────────────────

  function openCreate(isGlobal: boolean) {
    setError(null);
    setForm({ ...EMPTY_FORM, isGlobal });
    setEditMode({ type: "create", isGlobal });
  }

  function openEdit(item: preference.PreferenceItem) {
    setError(null);
    setForm({
      category: item.category,
      content: item.content,
      isGlobal: item.is_global,
    });
    setEditMode({ type: "edit", item });
  }

  function closeForm() {
    setEditMode(null);
    setForm(EMPTY_FORM);
  }

  async function handleSave() {
    if (!editMode) return;
    if (!form.content.trim()) {
      setError(t("preference.pleaseEnterContent"));
      return;
    }

    setSaving(true);
    try {
      if (editMode.type === "create") {
        await app.CreatePreference(novelId, {
          is_global: form.isGlobal,
          category: form.category || t("preference.uncategorized"),
          content: form.content,
        });
      } else {
        await app.UpdatePreference(novelId, editMode.item.id, {
          category: form.category,
          content: form.content,
          is_global: form.isGlobal,
        });
      }
      setEditMode(null);
      setForm(EMPTY_FORM);
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : t("preference.saveFailed"));
    } finally {
      setSaving(false);
    }
  }

  async function handleDelete(id: number) {
    if (!confirm(t("preference.confirmDeletePreference"))) return;
    setSaving(true);
    try {
      await app.DeletePreference(id);
      await load();
    } catch (err) {
      setError(
        err instanceof Error ? err.message : t("preference.deleteFailed"),
      );
    } finally {
      setSaving(false);
    }
  }

  // ── Render ───────────────────────────────────────────

  function renderSection(
    title: string,
    items: preference.PreferenceItem[],
    isGlobal: boolean,
  ) {
    const isCreating =
      editMode?.type === "create" && editMode.isGlobal === isGlobal;

    return (
      <section>
        <div className="flex items-center justify-between mb-3">
          <h3 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
            {title}
          </h3>
          {!isCreating && (
            <button
              onClick={() => openCreate(isGlobal)}
              className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-muted-foreground transition-colors"
            >
              <Plus className="h-3 w-3" /> {t("preference.add")}
            </button>
          )}
        </div>

        {items.length === 0 && !isCreating ? (
          <p className="text-xs text-muted-foreground py-4">
            {isGlobal
              ? t("preference.noGlobalPreference")
              : t("preference.noBookPreference")}
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
                      {t("preference.editPreference")}
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
                      {t("preference.delete")}
                    </button>
                    <button
                      onClick={closeForm}
                      className="px-3 py-1 rounded text-xs text-muted-foreground hover:text-foreground transition-colors"
                    >
                      {t("preference.cancel")}
                    </button>
                    <button
                      onClick={handleSave}
                      disabled={saving || !form.content.trim()}
                      className="px-3 py-1 rounded text-xs font-medium bg-primary text-primary-foreground hover:opacity-90 transition-opacity disabled:opacity-50"
                    >
                      {saving ? t("preference.saving") : t("preference.save")}
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
                      {item.category || t("preference.uncategorized")}
                    </span>
                    <p className="flex-1 text-sm text-foreground leading-relaxed whitespace-pre-wrap">
                      {item.content}
                    </p>
                    <div className="shrink-0 flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                      <button
                        onClick={() => openEdit(item)}
                        className="p-1 rounded text-muted-foreground hover:text-foreground hover:bg-secondary transition-colors"
                        title={t("preference.edit")}
                      >
                        <Pencil className="h-3.5 w-3.5" />
                      </button>
                      <button
                        onClick={() => handleDelete(item.id)}
                        className="p-1 rounded text-muted-foreground hover:text-destructive hover:bg-destructive/10 transition-colors"
                        title={t("preference.delete")}
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </button>
                    </div>
                  </div>
                </div>
              );
            })}

            {isCreating && (
              <div className="rounded-lg border border-dashed border-border bg-card/60 p-4">
                <div className="flex items-center justify-between mb-3">
                  <span className="text-xs font-semibold text-foreground">
                    {t("preference.newPreference")}
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
                    {saving ? t("preference.creating") : t("preference.create")}
                  </button>
                </div>
              </div>
            )}
          </div>
        )}
      </section>
    );
  }

  function renderFormFields() {
    return (
      <div className="space-y-3">
        {editMode?.type === "edit" && (
          <div>
            <label className="text-xs font-medium text-muted-foreground mb-1 block">
              {t("preference.scope")}
            </label>
            <div className="flex gap-2">
              <button
                type="button"
                onClick={() => setForm((f) => ({ ...f, isGlobal: true }))}
                className={`px-3 py-1 rounded text-xs transition-colors ${form.isGlobal ? "bg-primary text-primary-foreground" : "bg-secondary text-muted-foreground hover:text-foreground"}`}
              >
                {t("preference.global")}
              </button>
              <button
                type="button"
                onClick={() => setForm((f) => ({ ...f, isGlobal: false }))}
                className={`px-3 py-1 rounded text-xs transition-colors ${!form.isGlobal ? "bg-primary text-primary-foreground" : "bg-secondary text-muted-foreground hover:text-foreground"}`}
              >
                {t("preference.book")}
              </button>
            </div>
          </div>
        )}
        <div>
          <label className="text-xs font-medium text-muted-foreground mb-1 block">
            {t("preference.category")}
          </label>
          <input
            value={form.category}
            onChange={(e) =>
              setForm((f) => ({ ...f, category: e.target.value }))
            }
            placeholder={t("preference.categoryPlaceholder")}
            className="w-full rounded-md border border-border bg-background px-2.5 py-1.5 text-xs text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
        </div>
        <div>
          <label className="text-xs font-medium text-muted-foreground mb-1 block">
            {t("preference.content")}
          </label>
          <textarea
            value={form.content}
            onChange={(e) =>
              setForm((f) => ({ ...f, content: e.target.value }))
            }
            placeholder={t("preference.contentPlaceholder")}
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
          {t("preference.loading")}
        </div>
      ) : error ? (
        <div className="flex h-full items-center justify-center text-sm text-destructive">
          {error}
        </div>
      ) : (
        <div className="max-w-3xl mx-auto px-5 py-6 space-y-8">
          <div className="flex items-center gap-2">
            <Settings className="h-4 w-4 text-muted-foreground" />
            <h2 className="text-sm font-semibold text-foreground">
              {t("preference.creativePreference")}
              <span className="ml-2 text-xs font-normal text-muted-foreground">
                {global.length + novelPrefs.length} {t("preference.countUnit")}
              </span>
            </h2>
          </div>

          {renderSection(t("preference.globalPreference"), global, true)}

          <div className="border-t border-border" />

          {renderSection(t("preference.bookPreference"), novelPrefs, false)}
        </div>
      )}
    </main>
  );
}

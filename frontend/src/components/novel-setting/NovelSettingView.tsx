import { useEffect, useState } from "react";
import { Pencil, Plus, Globe, Trash2, X } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { setting } from "@/lib/wailsjs/go/models";
import { toastError } from "@/utils/toast";
import { toErrorMessage } from "@/utils/error";
import AutoGrowTextarea from "@/components/ui/AutoGrowTextarea";
import ConfirmDialog from "@/components/ui/ConfirmDialog";
import { useFocusWithNonce } from "@/hooks/useFocusWithNonce";
import { useNovelSettings } from "./useNovelSettings";
import { useDeleteNovelSetting } from "./useDeleteNovelSetting";
import { useCreateNovelSetting } from "./useCreateNovelSetting";
import { useUpdateNovelSetting } from "./useUpdateNovelSetting";

interface Props {
  novelId: number;
}

type EditMode =
  { type: "create" } | { type: "edit"; item: setting.SettingItem } | null;

type EditForm = {
  category: string;
  content: string;
};

const EMPTY_FORM: EditForm = { category: "", content: "" };

export default function NovelSettingView({ novelId }: Props) {
  const { t } = useTranslation();

  // 4.7.1: settings 走 query（与 NovelSettingList 共享缓存）。
  // 4a: query 错误 toast 由全局中间件接管（queryErrorToast.ts），此处不挂 useEffect。
  const settingsQuery = useNovelSettings(novelId);
  const items = settingsQuery.data?.items ?? [];
  const tokenCount = settingsQuery.data?.token_count ?? 0;
  const overBudget = settingsQuery.data?.over_budget ?? false;
  const loading = settingsQuery.isLoading;
  const loadFailed = settingsQuery.isError;

  // 4b: focus 定位——全局搜索/侧边栏点击触发 focusEntity("novel-settings", id)，
  // NovelSettingView useEffect 定位 + 声明式高亮（setting 单组，不需要跨组 querySelector）。
  const focus = useFocusWithNonce("novel-settings");
  const focusId = focus?.id ?? 0;
  const [highlightedId, setHighlightedId] = useState<number | null>(null);

  useEffect(() => {
    if (!focusId || focusId <= 0) return;
    if (!items.some((e) => e.id === focusId)) return;
    setHighlightedId(focusId);
    document
      .querySelector(`[data-setting-id="${focusId}"]`)
      ?.scrollIntoView({ behavior: "smooth", block: "center" });
    const timer = setTimeout(() => setHighlightedId(null), 2000);
    return () => clearTimeout(timer);
  }, [focusId, items, focus?.nonce]);

  // 4.7.2/4.7.3: CRUD 走 mutation，deleting/saving 由 mutation.isPending 推导（不再用 useState）。
  // onSuccess 失效对应 query（entry CRUD 失效 novel-settings，NovelSettingView + NovelSettingList 共享缓存）。
  const deleteMutation = useDeleteNovelSetting(novelId);
  const createMutation = useCreateNovelSetting(novelId);
  const updateMutation = useUpdateNovelSetting(novelId);
  const deleting = deleteMutation.isPending;
  const saving = createMutation.isPending || updateMutation.isPending;

  const [editMode, setEditMode] = useState<EditMode>(null);
  const [form, setForm] = useState<EditForm>(EMPTY_FORM);
  const [deleteTarget, setDeleteTarget] = useState<number | null>(null);

  // ── CRUD handlers ────────────────────────────────────

  function openCreate() {
    setForm({ ...EMPTY_FORM });
    setEditMode({ type: "create" });
  }

  function openEdit(item: setting.SettingItem) {
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
    // 4.7.3: 走 mutation（onSuccess 失效 novel-settings），删 setSaving/bumpRefresh。
    try {
      if (editMode.type === "create") {
        await createMutation.mutateAsync({
          category: form.category || t("novelSetting.uncategorized"),
          content: form.content,
        });
      } else {
        // 全量回传 input 所有字段（§6 等价 PUT）。
        await updateMutation.mutateAsync({
          id: editMode.item.id,
          input: {
            category: form.category,
            content: form.content,
          },
        });
      }
      setEditMode(null);
      setForm(EMPTY_FORM);
    } catch (err) {
      toastError(t("novelSetting.saveFailed") + ": " + toErrorMessage(err));
      console.error(err);
    }
  }

  function handleDelete(id: number) {
    setDeleteTarget(id);
  }

  async function confirmDelete() {
    if (deleteTarget === null) return;
    // 4.7.2: 走 mutation（onSuccess 失效 novel-settings），删 setDeleting/bumpRefresh。
    try {
      await deleteMutation.mutateAsync(deleteTarget);
      setDeleteTarget(null);
    } catch (err) {
      toastError(t("novelSetting.deleteFailed") + ": " + toErrorMessage(err));
      console.error(err);
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
          <AutoGrowTextarea
            value={form.content}
            onChange={(e) =>
              setForm((f) => ({ ...f, content: e.target.value }))
            }
            placeholder={t("novelSetting.contentPlaceholder")}
            minHeight={60}
            maxHeight={160}
            className="w-full rounded-md border border-border bg-background px-2.5 py-1.5 text-xs text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
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
              <span className="ml-auto text-xs text-warning-foreground">
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

            {loadFailed ? (
              <p className="text-xs text-destructive py-4">
                {t("novelSetting.loadFailed")}
              </p>
            ) : items.length === 0 && editMode?.type !== "create" ? (
              <p className="text-xs text-muted-foreground py-4">
                {t("novelSetting.noSetting")}
              </p>
            ) : (
              <div className="space-y-2">
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
                      data-setting-id={item.id}
                      className={`rounded-lg border border-border bg-card hover:border-border hover:shadow-sm transition-shadow group ${
                        highlightedId === item.id ? "ring-2 ring-primary" : ""
                      }`}
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
              </div>
            )}
          </section>
        </div>
      )}

      <ConfirmDialog
        open={deleteTarget !== null}
        title={t("common.confirmDelete")}
        message={t("novelSetting.confirmDeleteSetting")}
        danger
        loading={deleting}
        confirmText={t("common.delete")}
        onClose={() => setDeleteTarget(null)}
        onConfirm={confirmDelete}
      />
    </main>
  );
}

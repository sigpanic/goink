import { useState } from "react";
import { Pencil, Plus, Trash2, UsersRound, X } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import type { character } from "@/hooks/useApp";
import CharacterGraph from "@/components/character/CharacterGraph";
import TagInput from "@/components/shared/TagInput";
import { toastError } from "@/utils/toast";
import { toErrorMessage } from "@/utils/error";
import AutoGrowTextarea from "@/components/ui/AutoGrowTextarea";
import ConfirmDialog from "@/components/ui/ConfirmDialog";
import { useFocusStore } from "@/stores/useFocusStore";
import { characterKeys } from "@/lib/queryKeys";
import { useCharacters } from "./useCharacters";
import { useCreateCharacter } from "./useCreateCharacter";
import { useUpdateCharacter } from "./useUpdateCharacter";
import { useDeleteCharacter } from "./useDeleteCharacter";
import { useCharacterStore } from "./useCharacterStore";

interface Props {
  novelId: number;
}

type ViewTab = "list" | "graph";

type EditMode =
  | { type: "create" }
  | { type: "edit"; item: character.Character }
  | null;

type CharForm = {
  name: string;
  description: string;
  abilities: string[];
};

const EMPTY_FORM: CharForm = { name: "", description: "", abilities: [] };

function safeJson<T>(json: string, fallback: T): T {
  try {
    return JSON.parse(json);
  } catch {
    return fallback;
  }
}

export default function CharacterListView({ novelId }: Props) {
  const focusId = useFocusStore((s) => s.focusMap.characters ?? 0);
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  // 4.1.1: characters 数据走 useCharacters query，与 CharacterGraph / CharacterList 共享缓存。
  // 删除原 useState<characters> + useEffect + useRefresh 链路；
  // CRUD 后由 invalidateQueries 触发所有订阅者 refetch。
  // 4a: query 错误 toast 由全局中间件接管（queryErrorToast.ts），此处不再挂 useEffect。
  // 中间件在 QueryCache 层 fire 一次，避免多组件订阅同 queryKey 时重复 toast。
  const charsQuery = useCharacters(novelId);
  const characters = charsQuery.data ?? [];
  const loading = charsQuery.isLoading;
  const loadFailed = charsQuery.isError;

  const [viewTab, setViewTab] = useState<ViewTab>("list");
  const [editMode, setEditMode] = useState<EditMode>(null);
  const [form, setForm] = useState<CharForm>(EMPTY_FORM);
  // 4.1.2: create/update/delete 走 mutation，saving 由 mutation.isPending 推导（不再用 useState）。
  // create/update 共用 saving（同一时刻只可能开一个编辑表单）；delete 用 deleteMutation.isPending。
  const createMutation = useCreateCharacter(novelId);
  const updateMutation = useUpdateCharacter(novelId);
  const saving = createMutation.isPending || updateMutation.isPending;
  // 4.1.2: 删除走 useDeleteCharacter mutation + useCharacterStore 共享 deletingCharacterId。
  // CharacterList 点删除只 dispatch setDeletingCharacterId，ConfirmDialog + 执行集中在此处。
  const deleteMutation = useDeleteCharacter(novelId);
  const deletingCharacterId = useCharacterStore((s) => s.deletingCharacterId);
  const setDeletingCharacterId = useCharacterStore(
    (s) => s.setDeletingCharacterId,
  );

  // ── CRUD handlers ─────────────────────────────────────

  function openCreate() {
    setForm(EMPTY_FORM);
    setEditMode({ type: "create" });
  }

  function openEdit(c: character.Character) {
    setForm({
      name: c.name,
      description: c.description || "",
      abilities: safeJson<string[]>(c.abilities, []),
    });
    setEditMode({ type: "edit", item: c });
  }

  function buildPayload(): {
    name: string;
    description: string;
    abilities: string;
  } {
    return {
      name: form.name,
      description: form.description,
      abilities: JSON.stringify(form.abilities),
    };
  }

  async function handleCreate() {
    if (!form.name.trim()) {
      toastError(t("character.pleaseEnterName"));
      return;
    }
    // 4.1.2: create 走 mutation，onSuccess 失效 list；setEditMode + 错误 toast 留 handler。
    try {
      await createMutation.mutateAsync(buildPayload());
      setEditMode(null);
    } catch (err) {
      toastError(t("character.createFailed") + ": " + toErrorMessage(err));
      console.error(err);
    }
  }

  async function handleUpdate() {
    if (!editMode || editMode.type !== "edit") return;
    if (!form.name.trim()) {
      toastError(t("character.pleaseEnterName"));
      return;
    }
    // 4.1.2: update 走 mutation，onSuccess 失效 list；setEditMode + 错误 toast 留 handler。
    try {
      await updateMutation.mutateAsync({
        id: editMode.item.id,
        input: buildPayload(),
      });
      setEditMode(null);
    } catch (err) {
      toastError(t("character.updateFailed") + ": " + toErrorMessage(err));
      console.error(err);
    }
  }

  function handleDelete(charId: number) {
    setDeletingCharacterId(charId);
  }

  async function confirmDelete() {
    if (deletingCharacterId === null) return;
    try {
      await deleteMutation.mutateAsync(deletingCharacterId);
      setDeletingCharacterId(null);
    } catch (err) {
      toastError(t("character.deleteFailed") + ": " + toErrorMessage(err));
      console.error(err);
    }
  }

  // ── Render helpers ────────────────────────────────────

  function renderForm() {
    return (
      <div className="space-y-3">
        <div>
          <label className="text-xs font-medium text-muted-foreground mb-1 block">
            {t("character.name")}
          </label>
          <input
            type="text"
            value={form.name}
            onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
            className="w-full rounded-md border border-border bg-background px-2.5 py-1.5 text-xs text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            placeholder={t("character.characterName")}
          />
        </div>
        <div>
          <label className="text-xs font-medium text-muted-foreground mb-1 block">
            {t("character.description")}
          </label>
          <AutoGrowTextarea
            value={form.description}
            onChange={(e) =>
              setForm((f) => ({ ...f, description: e.target.value }))
            }
            minHeight={40}
            maxHeight={160}
            className="w-full rounded-md border border-border bg-background px-2.5 py-1.5 text-xs text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            placeholder={t("character.characterDesc")}
          />
        </div>
        <div>
          <label className="text-xs font-medium text-muted-foreground mb-1 block">
            {t("character.abilities")}
          </label>
          <TagInput
            tags={form.abilities}
            onChange={(abilities) => setForm((f) => ({ ...f, abilities }))}
            placeholder={t("character.abilityPlaceholder")}
          />
        </div>
      </div>
    );
  }

  function renderFormButtons(
    onSubmit: () => Promise<void>,
    onDelete?: () => void,
  ) {
    return (
      <div className="flex items-center gap-2 justify-end mt-3">
        {onDelete && (
          <button
            onClick={onDelete}
            disabled={saving}
            className="px-3 py-1 rounded text-xs text-destructive hover:bg-destructive/10 transition-colors"
          >
            <Trash2 className="h-3 w-3 inline mr-1" />
            {t("character.delete")}
          </button>
        )}
        <button
          onClick={() => setEditMode(null)}
          className="px-3 py-1 rounded text-xs text-muted-foreground hover:text-foreground transition-colors"
        >
          {t("common.cancel")}
        </button>
        <button
          onClick={onSubmit}
          disabled={saving}
          className="px-3 py-1 rounded bg-primary text-primary-foreground text-xs font-medium hover:opacity-90 transition-opacity disabled:opacity-50"
        >
          {saving ? t("common.saving") : t("common.save")}
        </button>
      </div>
    );
  }

  return (
    <main className="flex-1 min-w-0 flex flex-col overflow-hidden bg-background">
      {/* Tab bar */}
      <div className="flex items-center gap-1 px-5 pt-4 pb-2 shrink-0">
        <button
          onClick={() => setViewTab("list")}
          className={`px-3 py-1.5 rounded text-xs font-medium transition-colors ${
            viewTab === "list"
              ? "bg-card border border-border text-foreground shadow-sm"
              : "text-muted-foreground hover:text-foreground hover:bg-card/60"
          }`}
        >
          {t("character.list")}
        </button>
        <button
          onClick={() => setViewTab("graph")}
          className={`px-3 py-1.5 rounded text-xs font-medium transition-colors ${
            viewTab === "graph"
              ? "bg-card border border-border text-foreground shadow-sm"
              : "text-muted-foreground hover:text-foreground hover:bg-card/60"
          }`}
        >
          {t("character.relationGraph")}
        </button>
      </div>

      {viewTab === "graph" ? (
        <CharacterGraph novelId={novelId} focusId={focusId} />
      ) : loading ? (
        <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
          {t("character.loading")}
        </div>
      ) : (
        <div className="flex-1 overflow-y-auto overscroll-contain">
          <div className="max-w-3xl mx-auto px-5 py-6 space-y-6">
            {/* Header */}
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <UsersRound className="h-4 w-4 text-tag-blue-foreground" />
                <h2 className="text-sm font-semibold text-foreground">
                  {t("character.character")}
                  <span className="ml-2 text-xs font-normal text-muted-foreground">
                    {characters.length} {t("character.person")}
                  </span>
                </h2>
              </div>
              <div className="flex items-center gap-2">
                <button
                  onClick={() =>
                    queryClient.invalidateQueries({
                      queryKey: characterKeys.list(novelId),
                    })
                  }
                  className="text-xs text-muted-foreground hover:text-foreground transition-colors"
                >
                  {t("character.refresh")}
                </button>
                <button
                  onClick={openCreate}
                  className="inline-flex items-center gap-1 px-2.5 py-1 rounded text-xs font-medium bg-primary text-primary-foreground hover:opacity-90 transition-opacity"
                >
                  <Plus className="h-3 w-3" />
                  {t("character.newCharacter")}
                </button>
              </div>
            </div>

            {/* Create form */}
            {editMode?.type === "create" && (
              <div className="rounded-lg border border-border bg-card p-4">
                <div className="flex items-center justify-between mb-3">
                  <span className="text-xs font-semibold text-foreground">
                    新建角色
                  </span>
                  <button
                    onClick={() => setEditMode(null)}
                    className="p-0.5 rounded text-muted-foreground hover:text-foreground"
                  >
                    <X className="h-3.5 w-3.5" />
                  </button>
                </div>
                {renderForm()}
                {renderFormButtons(handleCreate)}
              </div>
            )}

            {/* Character list */}
            {loadFailed ? (
              <p className="text-xs text-destructive py-4">
                {t("character.loadFailed")}
              </p>
            ) : characters.length === 0 ? (
              <div className="text-center py-12">
                <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-full bg-tag-blue">
                  <UsersRound className="h-5 w-5 text-tag-blue-foreground" />
                </div>
                <p className="mt-2 text-sm text-muted-foreground">
                  {t("character.noCharacters")}
                </p>
                <button
                  onClick={openCreate}
                  className="mt-2 text-xs text-primary hover:underline"
                >
                  {t("character.createFirstCharacter")}
                </button>
              </div>
            ) : (
              <div className="space-y-2">
                {characters.map((c) => {
                  const isEditing =
                    editMode?.type === "edit" && editMode.item.id === c.id;
                  const abilities: string[] = safeJson<string[]>(
                    c.abilities,
                    [],
                  );

                  if (isEditing) {
                    return (
                      <div
                        key={c.id}
                        className="rounded-lg border border-border bg-card p-4"
                      >
                        <div className="flex items-center justify-between mb-3">
                          <span className="text-xs font-semibold text-foreground">
                            {t("character.editing")}
                            {c.name}
                          </span>
                          <button
                            onClick={() => setEditMode(null)}
                            className="p-0.5 rounded text-muted-foreground hover:text-foreground"
                          >
                            <X className="h-3.5 w-3.5" />
                          </button>
                        </div>
                        {renderForm()}
                        {renderFormButtons(handleUpdate, () =>
                          handleDelete(c.id),
                        )}
                      </div>
                    );
                  }

                  const desc = c.description?.trim() || "";

                  return (
                    <div
                      key={c.id}
                      className="rounded-lg border border-border bg-card hover:border-border hover:shadow-sm transition-shadow group"
                    >
                      <div className="flex items-start gap-3 px-4 py-3">
                        <span className="shrink-0 w-8 h-8 rounded-full bg-tag-blue text-tag-blue-foreground text-xs font-medium flex items-center justify-center">
                          {(c.name ?? "").charAt(0) || "?"}
                        </span>
                        <div className="flex-1 min-w-0">
                          <div className="flex items-center gap-2">
                            <span className="text-sm font-medium text-foreground">
                              {c.name}
                            </span>
                          </div>
                          {desc && (
                            <p className="mt-1 text-xs text-muted-foreground leading-relaxed line-clamp-2">
                              {desc}
                            </p>
                          )}
                          <div className="flex flex-wrap items-center gap-1 mt-1.5">
                            {abilities.map((a: string, i: number) => (
                              <span
                                key={i}
                                className="rounded px-1.5 py-0.5 text-xs font-medium bg-tag-amber text-tag-amber-foreground"
                              >
                                {a}
                              </span>
                            ))}
                          </div>
                        </div>
                        {/* Hover actions */}
                        <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity shrink-0">
                          <button
                            onClick={() => openEdit(c)}
                            className="p-1 rounded text-muted-foreground hover:text-foreground hover:bg-secondary transition-colors"
                            title={t("common.edit")}
                          >
                            <Pencil className="h-3.5 w-3.5" />
                          </button>
                          <button
                            onClick={() => handleDelete(c.id)}
                            className="p-1 rounded text-muted-foreground hover:text-destructive hover:bg-destructive/10 transition-colors"
                            title={t("character.delete")}
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
          </div>
        </div>
      )}

      <ConfirmDialog
        open={deletingCharacterId !== null}
        title={t("common.confirmDelete")}
        message={t("character.confirmDeleteWithRelation")}
        danger
        loading={deleteMutation.isPending}
        confirmText={t("common.delete")}
        onClose={() => setDeletingCharacterId(null)}
        onConfirm={confirmDelete}
      />
    </main>
  );
}

import { useState, useEffect, useMemo, useRef } from "react";
import { Pencil, Plus, Trash2, UsersRound, X } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import type { character } from "@/lib/wailsjs/go/models";
import CharacterGraph from "@/components/character/CharacterGraph";
import TagInput from "@/components/shared/TagInput";
import { toastError } from "@/utils/toast";
import { toErrorMessage } from "@/utils/error";
import AutoGrowTextarea from "@/components/ui/AutoGrowTextarea";
import ConfirmDialog from "@/components/ui/ConfirmDialog";
import { useFocusWithNonce } from "@/hooks/useFocusWithNonce";
import { characterKeys } from "@/lib/queryKeys";
import { useCharacters } from "./useCharacters";
import { useCreateCharacter } from "./useCreateCharacter";
import { useUpdateCharacter } from "./useUpdateCharacter";
import { useDeleteCharacter } from "./useDeleteCharacter";
import { useCharacterStore } from "./useCharacterStore";
import {
  useCharacterGroups,
  useCharacterGroupMembers,
} from "./useCharacterGroups";
import { useCharacterGroupStore } from "./useCharacterGroupStore";
import {
  indexCharacterMemberships,
  filterCharactersByGroup,
} from "./characterGrouping";
import CharacterGroupToolbar from "./CharacterGroupToolbar";
import CharacterMembershipEditor from "./CharacterMembershipEditor";

interface Props {
  novelId: number;
}

type ViewTab = "list" | "graph";

type EditMode =
  { type: "create" } | { type: "edit"; item: character.Character } | null;

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
  return <CharacterListViewContent key={novelId} novelId={novelId} />;
}

function CharacterListViewContent({ novelId }: Props) {
  const focus = useFocusWithNonce("characters");
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  // 4.1.1: characters 数据走 useCharacters query，与 CharacterGraph / CharacterList 共享缓存。
  // 删除原 useState<characters> + useEffect + useRefresh 链路；
  // CRUD 后由 invalidateQueries 触发所有订阅者 refetch。
  // 4a: query 错误 toast 由全局中间件接管（queryErrorToast.ts），此处不再挂 useEffect。
  // 中间件在 QueryCache 层 fire 一次，避免多组件订阅同 queryKey 时重复 toast。
  const charsQuery = useCharacters(novelId);
  const characters = useMemo(() => charsQuery.data ?? [], [charsQuery.data]);
  const groupsQuery = useCharacterGroups(novelId);
  const membersQuery = useCharacterGroupMembers(novelId);
  const groups = groupsQuery.data ?? [];
  const loading =
    charsQuery.isLoading || groupsQuery.isLoading || membersQuery.isLoading;
  const loadFailed =
    charsQuery.isError || groupsQuery.isError || membersQuery.isError;
  const requestedGroup = useCharacterGroupStore(
    (s) => s.selectedGroups[novelId] ?? null,
  );
  const selectedGroup =
    requestedGroup === 0 || groups.some((g) => g.id === requestedGroup)
      ? requestedGroup
      : null;
  const selectGroup = useCharacterGroupStore((s) => s.selectGroup);
  const viewTab = useCharacterGroupStore((s) => s.viewTabs[novelId] ?? "list");
  const setGroupViewTab = useCharacterGroupStore((s) => s.setViewTab);
  const setViewTab = (tab: ViewTab) => setGroupViewTab(novelId, tab);
  const memberships = useMemo(
    () => indexCharacterMemberships(membersQuery.data ?? []),
    [membersQuery.data],
  );
  const visibleCharacters = useMemo(
    () => filterCharactersByGroup(characters, memberships, selectedGroup),
    [characters, memberships, selectedGroup],
  );
  const [selectedCharacters, setSelectedCharacters] = useState<number[]>([]);
  const selectedIds = selectedCharacters.filter((id) =>
    visibleCharacters.some((c) => c.id === id),
  );
  const [membershipTargets, setMembershipTargets] = useState<number[]>([]);
  const targetIds = membershipTargets.filter((id) =>
    characters.some((c) => c.id === id),
  );
  const handledFocus = useRef<{ id: number; nonce: number } | undefined>(
    undefined,
  );

  const [editMode, setEditMode] = useState<EditMode>(null);
  const [form, setForm] = useState<CharForm>(EMPTY_FORM);
  // 4b: 高亮声明式——focus 触发后由 state 驱动 className，render 阶段应用，
  // 不再用命令式 classList.add/remove（易漏 cleanup）。参考 CharacterGraph 的 selectedCharacter。
  const [highlightedId, setHighlightedId] = useState<number | null>(null);
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
      selectGroup(novelId, null);
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

  useEffect(() => {
    if (
      !focus ||
      loading ||
      loadFailed ||
      (handledFocus.current?.id === focus.id &&
        handledFocus.current?.nonce === focus.nonce) ||
      !characters.some((c) => c.id === focus.id)
    )
      return;
    handledFocus.current = { id: focus.id, nonce: focus.nonce };
    if (
      !visibleCharacters.some((c) => c.id === focus.id) &&
      characters.some((c) => c.id === focus.id)
    ) {
      selectGroup(novelId, null);
    }
  }, [
    focus,
    loading,
    loadFailed,
    visibleCharacters,
    characters,
    selectGroup,
    novelId,
  ]);

  // 4b: list 模式 focusId 定位——搜索点击后 scrollIntoView + 临时高亮。
  // graph 模式定位由 CharacterGraph 内部 useEffect 处理（setSelectedCharacter）。
  useEffect(() => {
    if (!focus || focus.id <= 0 || characters.length === 0) return;
    // 高亮交给 state（render 阶段声明式应用），cleanup 只需 clearTimeout，杜绝漏移 class
    setHighlightedId(focus.id);
    const el = document.querySelector<HTMLElement>(
      `[data-character-id="${focus.id}"]`,
    );
    if (el) el.scrollIntoView({ behavior: "smooth", block: "center" });
    const timer = setTimeout(() => setHighlightedId(null), 2000);
    return () => clearTimeout(timer);
  }, [focus, characters, visibleCharacters]);

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

      {viewTab === "list" && <CharacterGroupToolbar novelId={novelId} />}

      {viewTab === "graph" ? (
        <CharacterGraph novelId={novelId} focus={focus} />
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
                    {visibleCharacters.length} {t("character.person")}
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

            {groups.length > 0 &&
              !loadFailed &&
              visibleCharacters.length > 0 && (
                <div className="flex flex-wrap items-center gap-3 text-xs">
                  <label className="flex items-center gap-2">
                    <input
                      type="checkbox"
                      checked={selectedIds.length === visibleCharacters.length}
                      aria-label={t("characterGroup.selectAll")}
                      onChange={(event) =>
                        setSelectedCharacters(
                          event.target.checked
                            ? visibleCharacters.map((c) => c.id)
                            : [],
                        )
                      }
                      className="accent-primary"
                    />
                    {t("characterGroup.selectAll")}
                  </label>
                  <span className="text-muted-foreground">
                    {t("characterGroup.selectedCount", {
                      count: selectedIds.length,
                    })}
                  </span>
                  <button
                    type="button"
                    disabled={selectedIds.length === 0}
                    onClick={() => setMembershipTargets(selectedIds)}
                    className="rounded border px-2.5 py-1 hover:bg-muted disabled:opacity-50"
                  >
                    {t("characterGroup.batchMemberships")}
                  </button>
                </div>
              )}

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
            ) : visibleCharacters.length === 0 ? (
              <div className="text-center py-12">
                <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-full bg-tag-blue">
                  <UsersRound className="h-5 w-5 text-tag-blue-foreground" />
                </div>
                <p className="mt-2 text-sm text-muted-foreground">
                  {t(
                    selectedGroup === null
                      ? "character.noCharacters"
                      : "characterGroup.empty",
                  )}
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
                {visibleCharacters.map((c) => {
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
                        data-character-id={c.id}
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
                        {groups.length > 0 && (
                          <button
                            type="button"
                            onClick={() => setMembershipTargets([c.id])}
                            className="mt-3 rounded border px-2.5 py-1 text-xs hover:bg-muted"
                          >
                            {t("characterGroup.adjustMemberships")}
                          </button>
                        )}
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
                      data-character-id={c.id}
                      className={`rounded-lg border border-border bg-card hover:border-border hover:shadow-sm transition-shadow group ${highlightedId === c.id ? "ring-2 ring-primary" : ""}`}
                    >
                      <div className="flex items-start gap-3 px-4 py-3">
                        {groups.length > 0 && (
                          <input
                            type="checkbox"
                            aria-label={t("characterGroup.selectCharacter", {
                              name: c.name,
                            })}
                            checked={selectedIds.includes(c.id)}
                            onChange={(event) =>
                              setSelectedCharacters((prev) =>
                                event.target.checked
                                  ? [...prev, c.id]
                                  : prev.filter((id) => id !== c.id),
                              )
                            }
                            className="mt-2 accent-primary"
                          />
                        )}
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
                            {groups
                              .filter((g) => memberships.get(c.id)?.has(g.id))
                              .map((g) => (
                                <button
                                  key={g.id}
                                  type="button"
                                  onClick={() => selectGroup(novelId, g.id)}
                                  className="rounded bg-tag-blue px-1.5 py-0.5 text-xs font-medium text-tag-blue-foreground hover:opacity-80"
                                >
                                  {g.name}
                                </button>
                              ))}
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
                        <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100 focus-within:opacity-100 transition-opacity shrink-0">
                          {groups.length > 0 && (
                            <button
                              type="button"
                              onClick={() => setMembershipTargets([c.id])}
                              title={t("characterGroup.adjustMemberships")}
                              aria-label={t("characterGroup.adjustCharacter", {
                                name: c.name,
                              })}
                              className="rounded p-1 text-muted-foreground hover:bg-secondary hover:text-foreground focus-visible:opacity-100"
                            >
                              <UsersRound className="h-3.5 w-3.5" />
                            </button>
                          )}
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

      {targetIds.length > 0 && (
        <CharacterMembershipEditor
          key={targetIds.join(",")}
          novelId={novelId}
          characterIds={targetIds}
          groups={groups}
          memberships={memberships}
          onClose={() => {
            setMembershipTargets([]);
            setSelectedCharacters([]);
          }}
        />
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

import { useEffect, useMemo, useState } from "react";
import { ChevronRight, MapPin, Pencil, Plus, Trash2, X } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import type { location } from "@/hooks/useApp";
import LocationGraph from "@/components/location/LocationGraph";
import TagInput from "@/components/shared/TagInput";
import { toastError } from "@/utils/toast";
import { toErrorMessage } from "@/utils/error";
import AutoGrowTextarea from "@/components/ui/AutoGrowTextarea";
import ConfirmDialog from "@/components/ui/ConfirmDialog";
import { useFocusWithNonce } from "@/hooks/useFocusWithNonce";
import { locationKeys } from "@/lib/queryKeys";
import { useLocations } from "./useLocations";
import { useLocationStore } from "./useLocationStore";
import { useDeleteLocation } from "./useDeleteLocation";
import { useCreateLocation } from "./useCreateLocation";
import { useUpdateLocation } from "./useUpdateLocation";

interface Props {
  novelId: number;
}

type ViewTab = "list" | "graph";

type EditMode =
  { type: "create" } | { type: "edit"; item: location.Location } | null;

type LocForm = {
  name: string;
  location_type: string;
  description: string;
  parent_location_id?: number;
  tags: string[];
};

const EMPTY_FORM: LocForm = {
  name: "",
  location_type: "",
  description: "",
  tags: [],
};

function safeJson<T>(json: string, fallback: T): T {
  try {
    return JSON.parse(json);
  } catch {
    return fallback;
  }
}

export default function LocationListView({ novelId }: Props) {
  const focus = useFocusWithNonce("locations");
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  // 4.2.1: locations 走 useLocations query（与 LocationList / LocationGraph 共享缓存）。
  // 删原 useState<locations[]> + load() + useEffect + useRefresh；CRUD 后由 invalidateQueries 触发 refetch。
  // 4a: query 错误 toast 由全局中间件接管（queryErrorToast.ts），此处不再挂 useEffect。
  const locsQuery = useLocations(novelId);
  const locations = locsQuery.data ?? [];
  const loading = locsQuery.isLoading;
  const loadFailed = locsQuery.isError;

  const [viewTab, setViewTab] = useState<ViewTab>("list");
  const [editMode, setEditMode] = useState<EditMode>(null);
  const [form, setForm] = useState<LocForm>(EMPTY_FORM);
  // 4b: 高亮声明式——focus 触发后由 state 驱动 className，render 阶段应用。
  // 参考 CharacterListView 的 highlightedId。
  const [highlightedId, setHighlightedId] = useState<number | null>(null);
  // 4.2.2: create/update/delete 走 mutation，saving 由 mutation.isPending 推导（不再用 useState）。
  // create/update 共用 saving（同一时刻只可能开一个编辑表单）；delete 用 deleteMutation.isPending。
  // 删除合并：deletingLocationId 走 useLocationStore，LocationList 侧边栏 dispatch 同一 store。
  const createMutation = useCreateLocation(novelId);
  const updateMutation = useUpdateLocation(novelId);
  const deleteMutation = useDeleteLocation(novelId);
  const saving = createMutation.isPending || updateMutation.isPending;
  const deletingLocationId = useLocationStore((s) => s.deletingLocationId);
  const setDeletingLocationId = useLocationStore(
    (s) => s.setDeletingLocationId,
  );

  const nameMap = useMemo(() => {
    const m = new Map<number, string>();
    for (const loc of locations) m.set(loc.id, loc.name);
    return m;
  }, [locations]);

  // 4b: list 模式 focusId 定位——搜索点击后 scrollIntoView + 临时高亮（声明式）。
  // graph 模式定位由 LocationGraph 内部 useEffect 处理（setSelectedLocation）。
  useEffect(() => {
    if (!focus || focus.id <= 0 || locations.length === 0) return;
    setHighlightedId(focus.id);
    const el = document.querySelector<HTMLElement>(
      `[data-location-id="${focus.id}"]`,
    );
    if (el) el.scrollIntoView({ behavior: "smooth", block: "center" });
    const timer = setTimeout(() => setHighlightedId(null), 2000);
    return () => clearTimeout(timer);
  }, [focus?.id, focus?.nonce, locations]);

  const locationTypeTag = (typeName: string) => {
    // Build typeToTag from i18n keywords + fixed English keywords
    const cnKeywords: Record<string, string[]> = {
      nature: t("location.typeKeywords.nature", {
        returnObjects: true,
      }) as unknown as string[],
      settlement: t("location.typeKeywords.settlement", {
        returnObjects: true,
      }) as unknown as string[],
      palace: t("location.typeKeywords.palace", {
        returnObjects: true,
      }) as unknown as string[],
      water: t("location.typeKeywords.water", {
        returnObjects: true,
      }) as unknown as string[],
    };
    const enKeywords: Record<string, string[]> = {
      nature: [
        "Forest",
        "Cave",
        "Mountain",
        "Swamp",
        "forest",
        "cave",
        "mountain",
        "swamp",
      ],
      settlement: [
        "City",
        "Town",
        "Village",
        "Market",
        "city",
        "town",
        "village",
        "market",
      ],
      palace: [
        "Palace",
        "Castle",
        "Temple",
        "Dungeon",
        "palace",
        "castle",
        "temple",
        "dungeon",
      ],
      water: ["Ocean", "River", "Lake", "ocean", "river", "lake"],
    };
    const typeToTag: Record<string, string> = {};
    for (const [tag, words] of Object.entries(cnKeywords)) {
      if (Array.isArray(words))
        words.forEach((w) => {
          typeToTag[w] = tag;
        });
    }
    for (const [tag, words] of Object.entries(enKeywords)) {
      words.forEach((w) => {
        typeToTag[w] = tag;
      });
    }
    const tagColorMap: Record<string, string> = {
      nature: "bg-tag-green text-tag-green-foreground",
      settlement: "bg-tag-amber text-tag-amber-foreground",
      palace: "bg-tag-purple text-tag-purple-foreground",
      water: "bg-tag-blue text-tag-blue-foreground",
    };
    const tag = typeToTag[typeName];
    return tag ? tagColorMap[tag] : "bg-secondary text-muted-foreground";
  };

  // ── CRUD handlers ─────────────────────────────────────

  function openCreate(parentId?: number) {
    setForm({ ...EMPTY_FORM, parent_location_id: parentId });
    setEditMode({ type: "create" });
  }

  function openEdit(loc: location.Location) {
    setForm({
      name: loc.name,
      location_type: loc.location_type || "",
      description: loc.description || "",
      parent_location_id: loc.parent_location_id ?? undefined,
      tags: safeJson<string[]>(loc.tags, []),
    });
    setEditMode({ type: "edit", item: loc });
  }

  function buildPayload(): {
    name: string;
    location_type: string;
    description: string;
    parent_location_id?: number;
    clear_parent?: boolean;
    tags: string;
  } {
    return {
      name: form.name,
      location_type: form.location_type,
      description: form.description,
      parent_location_id:
        form.parent_location_id && form.parent_location_id !== 0
          ? form.parent_location_id
          : undefined,
      clear_parent: !form.parent_location_id ? true : undefined,
      tags: JSON.stringify(form.tags),
    };
  }

  async function handleCreate() {
    if (!form.name.trim()) {
      toastError(t("location.pleaseEnterName"));
      return;
    }
    // 4.2.2: create 走 mutation，onSuccess 失效 list；setEditMode + 错误 toast 留 handler。
    try {
      await createMutation.mutateAsync(buildPayload());
      setEditMode(null);
    } catch (err) {
      toastError(t("location.createFailed") + ": " + toErrorMessage(err));
      console.error(err);
    }
  }

  async function handleUpdate() {
    if (!editMode || editMode.type !== "edit") return;
    if (!form.name.trim()) {
      toastError(t("location.pleaseEnterName"));
      return;
    }
    // 4.2.2: update 走 mutation，onSuccess 失效 list；setEditMode + 错误 toast 留 handler。
    try {
      await updateMutation.mutateAsync({
        id: editMode.item.id,
        input: buildPayload(),
      });
      setEditMode(null);
    } catch (err) {
      toastError(t("location.updateFailed") + ": " + toErrorMessage(err));
      console.error(err);
    }
  }

  function handleDelete(locId: number) {
    setDeletingLocationId(locId);
  }

  // 4.2.2: delete 走 useDeleteLocation mutation（onSuccess 失效 list + relations，
  // 后端 DeleteLocation 事务级联删 LocationRelation，两个缓存都要刷）。
  async function confirmDelete() {
    if (deletingLocationId === null) return;
    try {
      await deleteMutation.mutateAsync(deletingLocationId);
      setDeletingLocationId(null);
    } catch (err) {
      toastError(t("location.deleteFailed") + ": " + toErrorMessage(err));
      console.error(err);
    }
  }

  // ── Render helpers ────────────────────────────────────

  function renderForm() {
    return (
      <div className="space-y-3">
        <div>
          <label className="text-xs font-medium text-muted-foreground mb-1 block">
            {t("location.name")}
          </label>
          <input
            type="text"
            value={form.name}
            onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
            className="w-full rounded-md border border-border bg-background px-2.5 py-1.5 text-xs text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            placeholder={t("location.locationName")}
          />
        </div>
        <div className="flex gap-3">
          <div className="flex-1">
            <label className="text-xs font-medium text-muted-foreground mb-1 block">
              {t("location.type")}
            </label>
            <input
              type="text"
              value={form.location_type}
              onChange={(e) =>
                setForm((f) => ({ ...f, location_type: e.target.value }))
              }
              className="w-full rounded-md border border-border bg-background px-2.5 py-1.5 text-xs text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              placeholder={t("location.typeExample")}
            />
          </div>
          <div className="flex-1">
            <label className="text-xs font-medium text-muted-foreground mb-1 block">
              {t("location.parentLocation")}
            </label>
            <select
              value={form.parent_location_id ?? ""}
              onChange={(e) => {
                const v = e.target.value;
                setForm((f) => ({
                  ...f,
                  parent_location_id: v ? parseInt(v) : undefined,
                }));
              }}
              className="w-full rounded-md border border-border bg-background px-2.5 py-1.5 text-xs text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              <option value="">{t("location.noParent")}</option>
              {locations
                .filter(
                  (l) => editMode?.type !== "edit" || l.id !== editMode.item.id,
                )
                .map((l) => (
                  <option key={l.id} value={l.id}>
                    {l.name}
                  </option>
                ))}
            </select>
          </div>
        </div>
        <div>
          <label className="text-xs font-medium text-muted-foreground mb-1 block">
            {t("location.description")}
          </label>
          <AutoGrowTextarea
            value={form.description}
            onChange={(e) =>
              setForm((f) => ({ ...f, description: e.target.value }))
            }
            minHeight={40}
            maxHeight={160}
            className="w-full rounded-md border border-border bg-background px-2.5 py-1.5 text-xs text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            placeholder={t("location.locationDesc")}
          />
        </div>
        <div>
          <label className="text-xs font-medium text-muted-foreground mb-1 block">
            {t("location.tags")}
          </label>
          <TagInput
            tags={form.tags}
            onChange={(tags) => setForm((f) => ({ ...f, tags }))}
            placeholder={t("location.tagPlaceholder")}
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
            {t("location.delete")}
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
          {t("location.list")}
        </button>
        <button
          onClick={() => setViewTab("graph")}
          className={`px-3 py-1.5 rounded text-xs font-medium transition-colors ${
            viewTab === "graph"
              ? "bg-card border border-border text-foreground shadow-sm"
              : "text-muted-foreground hover:text-foreground hover:bg-card/60"
          }`}
        >
          {t("location.relationGraph")}
        </button>
      </div>

      {viewTab === "graph" ? (
        <LocationGraph novelId={novelId} focus={focus} />
      ) : loading ? (
        <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
          {t("location.loading")}
        </div>
      ) : (
        <div className="flex-1 overflow-y-auto overscroll-contain">
          <div className="max-w-3xl mx-auto px-5 py-6 space-y-6">
            {/* Header */}
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <MapPin className="h-4 w-4 text-tag-green-foreground" />
                <h2 className="text-sm font-semibold text-foreground">
                  {t("location.location")}
                  <span className="ml-2 text-xs font-normal text-muted-foreground">
                    {locations.length} {t("location.place")}
                  </span>
                </h2>
              </div>
              <div className="flex items-center gap-2">
                <button
                  onClick={() =>
                    queryClient.invalidateQueries({
                      queryKey: locationKeys.list(novelId),
                    })
                  }
                  className="text-xs text-muted-foreground hover:text-foreground transition-colors"
                >
                  {t("location.refresh")}
                </button>
                <button
                  onClick={() => openCreate()}
                  className="inline-flex items-center gap-1 px-2.5 py-1 rounded text-xs font-medium bg-primary text-primary-foreground hover:opacity-90 transition-opacity"
                >
                  <Plus className="h-3 w-3" />
                  {t("location.newLocation")}
                </button>
              </div>
            </div>

            {/* Create form */}
            {editMode?.type === "create" && (
              <div className="rounded-lg border border-border bg-card p-4">
                <div className="flex items-center justify-between mb-3">
                  <span className="text-xs font-semibold text-foreground">
                    {t("location.newLocation")}
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

            {/* Location list */}
            {loadFailed ? (
              <p className="text-xs text-destructive py-4">
                {t("location.loadFailed")}
              </p>
            ) : locations.length === 0 ? (
              <div className="text-center py-12">
                <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-full bg-tag-green">
                  <MapPin className="h-5 w-5 text-tag-green-foreground" />
                </div>
                <p className="mt-2 text-sm text-muted-foreground">
                  {t("location.noLocations")}
                </p>
                <button
                  onClick={() => openCreate()}
                  className="mt-2 text-xs text-primary hover:underline"
                >
                  {t("location.createFirstLocation")}
                </button>
              </div>
            ) : (
              <div className="space-y-2">
                {locations.map((loc) => {
                  const isEditing =
                    editMode?.type === "edit" && editMode.item.id === loc.id;
                  const tags: string[] = safeJson<string[]>(loc.tags, []);
                  const desc = loc.description?.trim() || "";
                  const parentName = loc.parent_location_id
                    ? nameMap.get(loc.parent_location_id)
                    : null;

                  if (isEditing) {
                    return (
                      <div
                        key={loc.id}
                        className="rounded-lg border border-border bg-card p-4"
                      >
                        <div className="flex items-center justify-between mb-3">
                          <span className="text-xs font-semibold text-foreground">
                            {t("location.editing")}
                            {loc.name}
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
                          handleDelete(loc.id),
                        )}
                      </div>
                    );
                  }

                  return (
                    <div
                      key={loc.id}
                      data-location-id={loc.id}
                      className={`rounded-lg border border-border bg-card hover:border-border hover:shadow-sm transition-shadow group ${highlightedId === loc.id ? "ring-2 ring-primary" : ""}`}
                    >
                      <div className="flex items-start gap-3 px-4 py-3">
                        <span className="shrink-0 flex h-8 w-8 items-center justify-center rounded bg-tag-green text-tag-green-foreground">
                          <MapPin className="h-4 w-4" />
                        </span>
                        <div className="flex-1 min-w-0">
                          <div className="flex items-center gap-2 flex-wrap">
                            <span className="text-sm font-medium text-foreground">
                              {loc.name}
                            </span>
                            {loc.location_type && (
                              <span
                                className={`shrink-0 rounded px-1.5 py-0.5 text-xs font-medium ${locationTypeTag(loc.location_type)}`}
                              >
                                {loc.location_type}
                              </span>
                            )}
                            {parentName && (
                              <span className="text-[11px] text-muted-foreground">
                                <ChevronRight className="h-3 w-3 inline text-muted-foreground/60" />
                                {parentName}
                              </span>
                            )}
                          </div>
                          {desc && (
                            <p className="mt-1 text-xs text-muted-foreground leading-relaxed line-clamp-2">
                              {desc}
                            </p>
                          )}
                          <div className="flex flex-wrap items-center gap-1 mt-1.5">
                            {tags.map((t: string, i: number) => (
                              <span
                                key={i}
                                className="rounded px-1.5 py-0.5 text-xs font-medium bg-tag-blue text-tag-blue-foreground"
                              >
                                {t}
                              </span>
                            ))}
                          </div>
                        </div>
                        {/* Hover actions */}
                        <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity shrink-0">
                          <button
                            onClick={() => openEdit(loc)}
                            className="p-1 rounded text-muted-foreground hover:text-foreground hover:bg-secondary transition-colors"
                            title={t("common.edit")}
                          >
                            <Pencil className="h-3.5 w-3.5" />
                          </button>
                          <button
                            onClick={() => handleDelete(loc.id)}
                            className="p-1 rounded text-muted-foreground hover:text-destructive hover:bg-destructive/10 transition-colors"
                            title={t("location.delete")}
                          >
                            <Trash2 className="h-3.5 w-3.5" />
                          </button>
                          {loc.parent_location_id && (
                            <button
                              onClick={() => openCreate(loc.id)}
                              className="p-1 rounded text-muted-foreground hover:text-foreground hover:bg-secondary transition-colors"
                              title={t("location.addSubLocation")}
                            >
                              <Plus className="h-3 w-3" />
                            </button>
                          )}
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
        open={deletingLocationId !== null}
        title={t("common.confirmDelete")}
        message={t("location.confirmDeleteLocation")}
        danger
        loading={deleteMutation.isPending}
        confirmText={t("common.delete")}
        onClose={() => setDeletingLocationId(null)}
        onConfirm={confirmDelete}
      />
    </main>
  );
}

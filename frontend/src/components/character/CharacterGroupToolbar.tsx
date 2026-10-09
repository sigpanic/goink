import { useState } from "react";
import { Pencil, Plus, Trash2 } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { character } from "@/lib/wailsjs/go/models";
import PopSelect from "@/components/shared/PopSelect";
import ConfirmDialog from "@/components/ui/ConfirmDialog";
import AutoGrowTextarea from "@/components/ui/AutoGrowTextarea";
import { toastError } from "@/utils/toast";
import { toErrorMessage } from "@/utils/error";
import {
  useCharacterGroups,
  useCharacterGroupMutations,
} from "./useCharacterGroups";
import { useCharacterGroupStore } from "./useCharacterGroupStore";

function GroupForm({
  novelId,
  group,
  onClose,
}: {
  novelId: number;
  group?: character.GroupView;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const { create, update } = useCharacterGroupMutations(novelId);
  const selectGroup = useCharacterGroupStore((s) => s.selectGroup);
  const [name, setName] = useState(group?.name ?? "");
  const [description, setDescription] = useState(group?.description ?? "");
  const [error, setError] = useState("");
  const busy = create.isPending || update.isPending;
  async function save() {
    if (!name.trim() || busy) return;
    setError("");
    try {
      const input = { name: name.trim(), description };
      if (group) await update.mutateAsync({ id: group.id, input });
      else {
        const created = await create.mutateAsync(input);
        selectGroup(novelId, created.id);
      }
      onClose();
    } catch (err) {
      setError(toErrorMessage(err));
    }
  }
  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        void save();
      }}
      aria-label={t(group ? "characterGroup.edit" : "characterGroup.create")}
      className="mt-3 space-y-3 rounded-lg border bg-card p-4"
    >
      <h3 className="text-sm font-medium">
        {t(group ? "characterGroup.edit" : "characterGroup.create")}
      </h3>
      <label className="block space-y-1 text-xs text-muted-foreground">
        <span>{t("characterGroup.name")}</span>
        <input
          autoFocus
          required
          value={name}
          disabled={busy}
          onChange={(event) => setName(event.target.value)}
          className="block w-full rounded-md border bg-background px-2.5 py-1.5 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        />
      </label>
      <label className="block space-y-1 text-xs text-muted-foreground">
        <span>{t("characterGroup.description")}</span>
        <AutoGrowTextarea
          value={description}
          disabled={busy}
          onChange={(event) => setDescription(event.target.value)}
          placeholder={t("characterGroup.descriptionPlaceholder")}
          minHeight={50}
          maxHeight={160}
          className="block w-full rounded-md border bg-background px-2.5 py-1.5 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        />
      </label>
      {error && (
        <p role="alert" className="text-xs text-destructive">
          {error}
        </p>
      )}
      <div className="flex justify-end gap-2">
        <button
          type="button"
          onClick={onClose}
          disabled={busy}
          className="rounded px-3 py-1.5 text-xs hover:bg-muted disabled:opacity-50"
        >
          {t("common.cancel")}
        </button>
        <button
          type="submit"
          disabled={busy || !name.trim()}
          className="rounded bg-primary px-3 py-1.5 text-xs text-primary-foreground disabled:opacity-50"
        >
          {t(busy ? "common.saving" : "common.save")}
        </button>
      </div>
    </form>
  );
}

export default function CharacterGroupToolbar({
  novelId,
}: {
  novelId: number;
}) {
  const { t } = useTranslation();
  const query = useCharacterGroups(novelId);
  const groups = query.data ?? [];
  const selected = useCharacterGroupStore(
    (s) => s.selectedGroups[novelId] ?? null,
  );
  const selectedGroup = groups.find((g) => g.id === selected);
  const selectedId = selected === 0 ? 0 : (selectedGroup?.id ?? null);
  const editingId = useCharacterGroupStore((s) => s.editingGroups[novelId]);
  const editingGroup = groups.find((g) => g.id === editingId);
  const deletingId = useCharacterGroupStore((s) => s.deletingGroups[novelId]);
  const deletingGroup = groups.find((g) => g.id === deletingId);
  const selectGroup = useCharacterGroupStore((s) => s.selectGroup);
  const editGroup = useCharacterGroupStore((s) => s.editGroup);
  const deleteGroup = useCharacterGroupStore((s) => s.deleteGroup);
  const { remove } = useCharacterGroupMutations(novelId);
  async function confirmDelete() {
    if (!deletingGroup || remove.isPending) return;
    try {
      await remove.mutateAsync(deletingGroup.id);
      if (selected === deletingGroup.id) selectGroup(novelId, null);
      deleteGroup(novelId, undefined);
    } catch (err) {
      toastError(`${t("characterGroup.deleteFailed")}: ${toErrorMessage(err)}`);
    }
  }
  return (
    <div className="px-5 pb-3 shrink-0">
      <div className="flex flex-wrap items-center gap-2">
        <PopSelect
          value={selectedId === null ? "all" : String(selectedId)}
          ariaLabel={t("characterGroup.filter")}
          dropUp={false}
          disabled={novelId <= 0 || query.isLoading || query.isError}
          options={[
            { value: "all", label: t("characterGroup.all") },
            { value: "0", label: t("characterGroup.ungrouped") },
            ...groups.map((g) => ({
              value: String(g.id),
              label: `${g.name} (${g.member_count})`,
            })),
          ]}
          onChange={(value) =>
            selectGroup(novelId, value === "all" ? null : Number(value))
          }
        />
        <button
          type="button"
          onClick={() => editGroup(novelId, null)}
          disabled={novelId <= 0 || query.isLoading || query.isError}
          className="inline-flex items-center gap-1 rounded border px-2.5 py-1.5 text-xs hover:bg-muted disabled:opacity-50"
        >
          <Plus className="h-3 w-3" />
          {t("characterGroup.create")}
        </button>
        {selectedGroup && (
          <>
            <button
              type="button"
              onClick={() => editGroup(novelId, selectedGroup.id)}
              title={t("characterGroup.edit")}
              className="rounded p-1.5 text-muted-foreground hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring"
            >
              <Pencil className="h-3.5 w-3.5" />
            </button>
            <button
              type="button"
              onClick={() => deleteGroup(novelId, selectedGroup.id)}
              title={t("characterGroup.delete")}
              className="rounded p-1.5 text-destructive hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring"
            >
              <Trash2 className="h-3.5 w-3.5" />
            </button>
          </>
        )}
      </div>
      {selectedGroup?.description && (
        <p className="mt-2 whitespace-pre-wrap text-xs text-muted-foreground">
          {selectedGroup.description}
        </p>
      )}
      {query.isError && (
        <p className="mt-2 text-xs text-destructive">
          {t("characterGroup.loadFailed")}
        </p>
      )}
      {(editingId === null || editingGroup) && (
        <GroupForm
          key={`${novelId}:${editingId ?? "new"}`}
          novelId={novelId}
          group={editingGroup}
          onClose={() => editGroup(novelId, undefined)}
        />
      )}
      <ConfirmDialog
        open={!!deletingGroup}
        title={t("characterGroup.delete")}
        message={t("characterGroup.confirmDelete", {
          name: deletingGroup?.name ?? "",
        })}
        danger
        loading={remove.isPending}
        onConfirm={confirmDelete}
        onClose={() => {
          if (!remove.isPending) deleteGroup(novelId, undefined);
        }}
      />
    </div>
  );
}

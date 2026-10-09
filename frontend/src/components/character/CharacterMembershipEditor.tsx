import { useState } from "react";
import { Dialog } from "radix-ui";
import { useTranslation } from "react-i18next";
import type { character } from "@/lib/wailsjs/go/models";
import { toErrorMessage } from "@/utils/error";
import { useCharacterGroupMutations } from "./useCharacterGroups";

interface Props {
  novelId: number;
  characterIds: number[];
  groups: character.GroupView[];
  memberships: Map<number, Set<number>>;
  onClose: () => void;
}

export default function CharacterMembershipEditor({
  novelId,
  characterIds,
  groups,
  memberships,
  onClose,
}: Props) {
  const { t } = useTranslation();
  const { memberships: mutation } = useCharacterGroupMutations(novelId);
  const [changes, setChanges] = useState<Record<number, boolean>>({});
  const [error, setError] = useState("");
  const changedGroups = groups.filter((g) => changes[g.id] !== undefined);
  async function save() {
    if (!changedGroups.length || mutation.isPending) return;
    setError("");
    try {
      await mutation.mutateAsync({
        character_ids: characterIds,
        add_group_ids: changedGroups
          .filter((g) => changes[g.id])
          .map((g) => g.id),
        remove_group_ids: changedGroups
          .filter((g) => !changes[g.id])
          .map((g) => g.id),
      });
      onClose();
    } catch (err) {
      setError(toErrorMessage(err));
    }
  }
  return (
    <Dialog.Root
      open
      onOpenChange={(open) => {
        if (!open && !mutation.isPending) onClose();
      }}
    >
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-50 bg-black/40" />
        <Dialog.Content className="fixed left-1/2 top-1/2 z-50 flex max-h-[80vh] w-[420px] max-w-[90vw] -translate-x-1/2 -translate-y-1/2 flex-col rounded-xl border bg-background p-5 shadow-2xl">
          <Dialog.Title className="text-sm font-semibold">
            {t("characterGroup.adjustMemberships")}
          </Dialog.Title>
          <Dialog.Description className="mt-2 text-xs text-muted-foreground">
            {t("characterGroup.membershipHint", { count: characterIds.length })}
          </Dialog.Description>
          <div className="my-4 min-h-0 space-y-2 overflow-y-auto">
            {groups.map((group) => {
              const count = characterIds.filter((id) =>
                memberships.get(id)?.has(group.id),
              ).length;
              const checked =
                changes[group.id] ?? count === characterIds.length;
              const mixed =
                changes[group.id] === undefined &&
                count > 0 &&
                count < characterIds.length;
              return (
                <label
                  key={group.id}
                  className="flex items-start gap-2 rounded-md border px-3 py-2 text-xs"
                >
                  <input
                    type="checkbox"
                    checked={checked}
                    aria-checked={mixed ? "mixed" : checked}
                    ref={(element) => {
                      if (element) element.indeterminate = mixed;
                    }}
                    disabled={mutation.isPending}
                    onChange={(event) =>
                      setChanges((prev) => ({
                        ...prev,
                        [group.id]: event.target.checked,
                      }))
                    }
                    className="mt-0.5 accent-primary"
                  />
                  <span className="min-w-0 flex-1">
                    <span className="block font-medium">{group.name}</span>
                    {group.description && (
                      <span className="mt-1 block whitespace-pre-wrap text-muted-foreground">
                        {group.description}
                      </span>
                    )}
                  </span>
                  {mixed && (
                    <span className="text-muted-foreground">
                      {t("characterGroup.partial")}
                    </span>
                  )}
                </label>
              );
            })}
            {groups.length === 0 && (
              <p className="text-xs text-muted-foreground">
                {t("characterGroup.noGroups")}
              </p>
            )}
          </div>
          {error && (
            <p role="alert" className="mb-3 text-xs text-destructive">
              {error}
            </p>
          )}
          <div className="flex justify-end gap-2">
            <button
              type="button"
              onClick={onClose}
              disabled={mutation.isPending}
              className="rounded px-3 py-1.5 text-xs hover:bg-muted disabled:opacity-50"
            >
              {t("common.cancel")}
            </button>
            <button
              type="button"
              onClick={() => void save()}
              disabled={!changedGroups.length || mutation.isPending}
              className="rounded bg-primary px-3 py-1.5 text-xs text-primary-foreground disabled:opacity-50"
            >
              {t(mutation.isPending ? "common.saving" : "common.save")}
            </button>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

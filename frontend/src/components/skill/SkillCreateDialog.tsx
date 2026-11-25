import { useState } from "react";
import { Dialog } from "radix-ui";
import { useTranslation } from "react-i18next";
import type { skill } from "@/lib/wailsjs/go/models";
import { useSaveContent } from "@/components/content/useSaveContent";
import { isContentConflict } from "@/components/content/editorSaveQueue";
import { toErrorMessage } from "@/utils/error";
import SkillFormFields from "./SkillFormFields";
import {
  emptyDraft,
  serializeDraft,
  validateDraft,
  validateSkillName,
  type SkillDraft,
} from "./skillDraft";

export type SkillTarget = "novel" | "user";

interface Props {
  novelId: number;
  skills: skill.SkillMeta[];
  onClose: () => void;
  onCreated: (name: string, target: SkillTarget) => void;
}

function pathForTarget(target: SkillTarget, name: string): string {
  return target === "user" ? `~/.goink/skills/${name}.md` : `skills/${name}.md`;
}

// SkillCreateDialog 是独立的新建表单：名称只输入一次，层级在提交前选择。
// 创建复用 SaveContent（expected_content="" 保证不覆盖已有文件），不新增后端接口。
export default function SkillCreateDialog({
  novelId,
  skills,
  onClose,
  onCreated,
}: Props) {
  const { t } = useTranslation();
  const saveMutation = useSaveContent();
  const [draft, setDraft] = useState<SkillDraft>(emptyDraft);
  const [target, setTarget] = useState<SkillTarget>("novel");
  const [error, setError] = useState("");
  const saving = saveMutation.isPending;

  const updateDraft = (patch: Partial<SkillDraft>) => {
    if (saving) return;
    setDraft((previous) => ({ ...previous, ...patch }));
  };

  async function handleCreate() {
    if (saving) return;
    const requiredError = validateDraft(draft);
    if (requiredError) {
      setError(t(requiredError));
      return;
    }
    const name = draft.name.trim();
    const nameError = validateSkillName(name);
    if (nameError) {
      setError(t(nameError));
      return;
    }
    if (skills.some((s) => s.name === name && s.source === target)) {
      setError(t("skill.nameExistsAtScope"));
      return;
    }
    setError("");
    try {
      await saveMutation.mutateAsync({
        novel_id: novelId,
        path: pathForTarget(target, name),
        content: serializeDraft({ ...draft, name }),
        expected_content: "",
      });
      onCreated(name, target);
      onClose();
    } catch (err) {
      setError(
        isContentConflict(err)
          ? t("skill.nameExistsAtScope")
          : toErrorMessage(err),
      );
    }
  }

  return (
    <Dialog.Root
      open
      onOpenChange={(open) => {
        if (!open && !saving) onClose();
      }}
    >
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-50 bg-black/40" />
        <Dialog.Content className="fixed left-1/2 top-1/2 z-50 flex max-h-[85vh] w-[560px] max-w-[92vw] -translate-x-1/2 -translate-y-1/2 flex-col overflow-hidden rounded-xl border bg-background p-5 shadow-2xl">
          <Dialog.Title className="text-base font-semibold">
            {t("skill.createTitle")}
          </Dialog.Title>
          <Dialog.Description className="mt-1 text-sm text-muted-foreground">
            {t("skill.createDesc")}
          </Dialog.Description>
          <div className="mt-4 flex min-h-0 flex-1 flex-col">
            <div className="min-h-0 flex-1 space-y-4 overflow-y-auto pr-1">
              {error && (
                <div
                  role="alert"
                  className="px-3 py-2 text-sm font-medium text-destructive-foreground bg-destructive border border-destructive rounded-md"
                >
                  {error}
                </div>
              )}

              <div>
                <span className="block text-xs font-medium text-muted-foreground mb-1.5">
                  {t("skill.scopeLabel")}
                </span>
                <div className="grid grid-cols-2 gap-2">
                  {(["novel", "user"] as const).map((value) => (
                    <button
                      key={value}
                      type="button"
                      aria-pressed={target === value}
                      onClick={() => setTarget(value)}
                      disabled={saving}
                      className={`rounded-md border px-3 py-2 text-left transition-colors disabled:opacity-50 ${
                        target === value
                          ? "border-primary bg-primary/5"
                          : "hover:bg-muted/60"
                      }`}
                    >
                      <span className="block text-sm font-medium">
                        {t(
                          value === "novel"
                            ? "skill.scopeNovel"
                            : "skill.scopeUser",
                        )}
                      </span>
                      <span className="block text-xs text-muted-foreground">
                        {t(
                          value === "novel"
                            ? "skill.scopeNovelHint"
                            : "skill.scopeUserHint",
                        )}
                      </span>
                    </button>
                  ))}
                </div>
              </div>

              <fieldset disabled={saving} className="space-y-4">
                <SkillFormFields
                  draft={draft}
                  onChange={updateDraft}
                  source={target}
                  contentHeight={{ min: 160, max: 320 }}
                />
              </fieldset>
            </div>

            <div className="mt-4 flex justify-end gap-2">
              <button
                type="button"
                onClick={onClose}
                disabled={saving}
                className="h-9 rounded-md border px-4 text-sm hover:bg-muted transition-colors disabled:opacity-50"
              >
                {t("skill.cancel")}
              </button>
              <button
                type="button"
                onClick={() => void handleCreate()}
                disabled={saving}
                className="h-9 rounded-md bg-primary px-4 text-sm text-primary-foreground hover:opacity-90 transition-opacity disabled:opacity-50"
              >
                {saving ? t("skill.saving") : t("skill.create")}
              </button>
            </div>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

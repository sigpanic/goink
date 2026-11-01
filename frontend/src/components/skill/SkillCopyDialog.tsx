import { useState } from "react";
import { Dialog } from "radix-ui";
import { useTranslation } from "react-i18next";
import type { skill } from "@/lib/wailsjs/go/models";
import { useNovels } from "@/components/novel/useNovels";
import { useFileContent } from "@/components/content/useFileContent";
import { useEditorTabsStore } from "@/components/content/useEditorTabsStore";
import PopSelect from "@/components/shared/PopSelect";
import { toErrorMessage } from "@/utils/error";
import { useSkills } from "./useSkills";

interface Props {
  novelId: number;
  currentSkills: skill.SkillMeta[];
  onNewSkill: (name: string, initialContent?: string) => void;
  onClose: () => void;
}

export default function SkillCopyDialog({
  novelId,
  currentSkills,
  onNewSkill,
  onClose,
}: Props) {
  const { t } = useTranslation();
  const {
    data: novels = [],
    isLoading: novelsLoading,
    isError: novelsError,
  } = useNovels();
  const [sourceId, setSourceId] = useState(0);
  const {
    data: skills = [],
    isLoading: skillsLoading,
    isError: skillsError,
  } = useSkills(sourceId);
  const { fetchContent } = useFileContent();
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [confirmOverwrite, setConfirmOverwrite] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const sources = novels.filter((n) => n.id !== novelId);
  const novelSkills = skills.filter((s) => s.source === "novel");
  const existingNames = new Set(
    currentSkills.filter((s) => s.source === "novel").map((s) => s.name),
  );
  const openTabs = useEditorTabsStore((s) => s.byNovel[String(novelId)]?.tabs);
  for (const tab of openTabs ?? []) {
    if (
      tab.type === "file" &&
      tab.path.startsWith("skills/") &&
      tab.path.endsWith(".md")
    )
      existingNames.add(tab.path.slice("skills/".length, -3));
  }
  const selectedSkills = novelSkills.filter((s) => selected.has(s.name));
  const overwritten = selectedSkills.filter((s) => existingNames.has(s.name));

  async function handleCopy() {
    if (loading || !selectedSkills.length) return;
    if (overwritten.length && !confirmOverwrite) {
      setConfirmOverwrite(true);
      return;
    }
    setLoading(true);
    setError("");
    try {
      const contents = await Promise.all(
        selectedSkills.map((s) =>
          fetchContent(sourceId, `skills/${s.name}.md`),
        ),
      );
      selectedSkills.forEach((s, i) => onNewSkill(s.name, contents[i]));
      onClose();
    } catch (err) {
      setError(toErrorMessage(err));
    } finally {
      setLoading(false);
    }
  }

  return (
    <Dialog.Root
      open
      onOpenChange={(open) => {
        if (!open && !loading) onClose();
      }}
    >
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-50 bg-black/40" />
        <Dialog.Content className="fixed left-1/2 top-1/2 z-50 flex max-h-[80vh] w-[480px] max-w-[90vw] -translate-x-1/2 -translate-y-1/2 flex-col rounded-xl border bg-background p-5 shadow-2xl">
          <Dialog.Title className="text-base font-semibold">
            {t("skill.copy.title")}
          </Dialog.Title>
          <Dialog.Description className="mt-2 text-sm text-muted-foreground">
            {t("skill.copy.description")}
          </Dialog.Description>
          {error && (
            <p role="alert" className="mt-3 text-sm text-destructive">
              {error}
            </p>
          )}
          {confirmOverwrite ? (
            <div className="my-4 text-sm">
              <p>{t("skill.copy.overwriteWarning")}</p>
              <ul className="mt-2 list-disc pl-5">
                {overwritten.map((s) => (
                  <li key={s.name}>{s.name}</li>
                ))}
              </ul>
            </div>
          ) : (
            <>
              <span className="mt-4 text-sm">{t("skill.copy.source")}</span>
              <PopSelect
                ariaLabel={t("skill.copy.source")}
                size="form"
                dropUp={false}
                className="mt-1 w-full"
                minWidth="0"
                value={String(sourceId)}
                placeholder={t("skill.copy.selectSource")}
                options={sources.map((n) => ({
                  value: String(n.id),
                  label: n.title,
                }))}
                disabled={loading || novelsLoading}
                onChange={(value) => {
                  setSourceId(Number(value));
                  setSelected(new Set());
                  setError("");
                }}
              />
              <div className="my-4 min-h-16 overflow-y-auto">
                {novelsError || (sourceId > 0 && skillsError) ? (
                  <p role="alert" className="text-sm text-destructive">
                    {t("skill.loadFailed")}
                  </p>
                ) : novelsLoading || (sourceId > 0 && skillsLoading) ? (
                  <p className="text-sm text-muted-foreground">
                    {t("skill.loading")}
                  </p>
                ) : !sources.length ? (
                  <p className="text-sm text-muted-foreground">
                    {t("skill.copy.noOtherNovels")}
                  </p>
                ) : sourceId > 0 && !novelSkills.length ? (
                  <p className="text-sm text-muted-foreground">
                    {t("skill.copy.noNovelSkills")}
                  </p>
                ) : (
                  sourceId > 0 &&
                  novelSkills.map((s) => (
                    <label
                      key={s.name}
                      className="flex cursor-pointer items-start gap-2 rounded-md p-2 hover:bg-muted/60"
                    >
                      <input
                        type="checkbox"
                        checked={selected.has(s.name)}
                        disabled={loading}
                        onChange={(e) =>
                          setSelected((previous) => {
                            const next = new Set(previous);
                            if (e.target.checked) next.add(s.name);
                            else next.delete(s.name);
                            return next;
                          })
                        }
                        className="mt-1"
                      />
                      <span className="min-w-0 text-sm">
                        <span className="break-words font-medium">
                          {s.name}
                        </span>
                        {existingNames.has(s.name) && (
                          <span className="ml-2 text-xs text-destructive">
                            {t("skill.copy.overwrite")}
                          </span>
                        )}
                        <span className="block text-xs text-muted-foreground">
                          {s.description}
                        </span>
                      </span>
                    </label>
                  ))
                )}
              </div>
            </>
          )}
          <div className="mt-2 flex justify-end gap-2">
            <button
              disabled={loading}
              onClick={() =>
                confirmOverwrite ? setConfirmOverwrite(false) : onClose()
              }
              className="h-9 rounded-md border px-3 text-sm hover:bg-muted disabled:opacity-50"
            >
              {confirmOverwrite ? t("common.back") : t("common.cancel")}
            </button>
            <button
              disabled={
                loading || !selectedSkills.length || skillsError || novelsError
              }
              onClick={() => void handleCopy()}
              className="h-9 rounded-md bg-primary px-3 text-sm text-primary-foreground hover:opacity-90 disabled:opacity-50"
            >
              {loading
                ? t("skill.loading")
                : confirmOverwrite
                  ? t("skill.copy.confirmOverwrite")
                  : t("skill.copy.openDrafts")}
            </button>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

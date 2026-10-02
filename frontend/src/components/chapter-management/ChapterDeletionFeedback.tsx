import { useTranslation } from "react-i18next";
import type { chapter, deletion } from "@/lib/wailsjs/go/models";
import ConfirmDialog from "@/components/ui/ConfirmDialog";
import { chapterPath, outlinePath } from "@/components/content/types";
import { useEditorTabsStore } from "@/components/content/useEditorTabsStore";

export type BlockedChapterDeletion = {
  item: chapter.Chapter;
  references: deletion.Blocker[];
};

interface Props {
  novelId: number;
  deleteTarget: chapter.Chapter | null;
  blocked: BlockedChapterDeletion | null;
  loading: boolean;
  onConfirm: () => void | Promise<void>;
  onClose: () => void;
  onDismissBlocked: () => void;
}

export default function ChapterDeletionFeedback({
  novelId,
  deleteTarget,
  blocked,
  loading,
  onConfirm,
  onClose,
  onDismissBlocked,
}: Props) {
  const { t } = useTranslation();
  const deleteHasUnsaved =
    deleteTarget !== null &&
    (useEditorTabsStore.getState().byNovel[String(novelId)]?.tabs ?? []).some(
      (tab) =>
        (tab.path ===
          (deleteTarget.file_path || chapterPath(deleteTarget.id)) ||
          tab.path === outlinePath(deleteTarget.id)) &&
        (tab.isDirty || tab.outlineIsDirty),
    );

  return (
    <>
      {blocked && (
        <section
          role="alert"
          className="rounded-lg border border-destructive/40 bg-destructive/5 p-4"
        >
          <div className="flex items-start justify-between gap-3">
            <h2 className="text-sm font-medium text-destructive">
              {t("chapterManagement.deleteBlocked", {
                title: blocked.item.title,
              })}
            </h2>
            <button
              type="button"
              onClick={onDismissBlocked}
              className="text-sm underline"
            >
              {t("common.close")}
            </button>
          </div>
          <ul className="mt-2 list-inside list-disc text-sm">
            {blocked.references.map((reference) => (
              <li key={`${reference.kind}:${reference.id}`}>
                {t(`chapterManagement.blockerKind.${reference.kind}`, {
                  defaultValue: reference.kind,
                })}{" "}
                · {reference.label} (ID {reference.id})
              </li>
            ))}
          </ul>
        </section>
      )}
      <ConfirmDialog
        open={deleteTarget !== null}
        title={t("chapterManagement.deleteChapter")}
        message={t(
          deleteHasUnsaved
            ? "chapterManagement.deleteChapterConfirmUnsaved"
            : "chapterManagement.deleteChapterConfirm",
          { title: deleteTarget?.title ?? "" },
        )}
        danger
        loading={loading}
        onConfirm={onConfirm}
        onClose={onClose}
      />
    </>
  );
}

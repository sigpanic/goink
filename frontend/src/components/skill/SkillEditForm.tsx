import { useState, useEffect } from "react";
import { useTranslation } from "react-i18next";
import type { SkillSource } from "@/components/content/types";
import SkillFormFields from "./SkillFormFields";
import {
  parseDraft,
  serializeDraft,
  validateDraft,
  type SkillDraft,
} from "./skillDraft";

interface Props {
  content: string;
  isDirty: boolean;
  source?: SkillSource;
  readOnly?: boolean;
  onDraftChange: (content: string) => void;
  onSave: (newContent: string) => Promise<void>;
  onCancel: () => void;
}

export default function SkillEditForm({
  content,
  isDirty,
  source,
  readOnly,
  onDraftChange,
  onSave,
  onCancel,
}: Props) {
  const { t } = useTranslation();

  const [draft, setDraft] = useState(() => parseDraft(content));
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (isDirty) return;
    setDraft(parseDraft(content));
    setError("");
  }, [content, isDirty]);

  const updateDraft = (patch: Partial<SkillDraft>) => {
    if (saving) return;
    const next = { ...draft, ...patch };
    setDraft(next);
    onDraftChange(serializeDraft(next));
  };

  if (readOnly) {
    return (
      <div className="flex items-center justify-center h-full">
        <p className="text-sm text-muted-foreground">
          {t("skill.builtinNotEditable")}
        </p>
      </div>
    );
  }

  const handleSave = async () => {
    if (saving) return;
    const requiredError = validateDraft(draft);
    if (requiredError) {
      setError(t(requiredError));
      return;
    }
    setSaving(true);
    setError("");
    try {
      await onSave(serializeDraft(draft));
    } catch (e: any) {
      setError(
        typeof e === "string"
          ? e
          : e?.message || e?.toString() || t("skill.saveFailed"),
      );
    } finally {
      setSaving(false);
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Escape" && !saving) onCancel();
  };

  return (
    <div className="overflow-y-auto h-full" onKeyDown={handleKeyDown}>
      <fieldset
        disabled={saving}
        className="max-w-2xl mx-auto px-6 py-6 space-y-4"
      >
        {error && (
          <div className="sticky top-0 z-10 px-3 py-2.5 text-sm font-medium text-destructive-foreground bg-destructive border border-destructive rounded-md shadow-sm">
            {error}
          </div>
        )}

        <SkillFormFields draft={draft} onChange={updateDraft} source={source} />

        <div className="flex justify-end gap-2 pt-2">
          <button
            onClick={onCancel}
            className="h-9 px-4 rounded-md text-sm border hover:bg-muted transition-colors"
          >
            {t("skill.cancel")}
          </button>
          <button
            onClick={handleSave}
            disabled={saving}
            className="h-9 px-4 rounded-md text-sm bg-primary text-primary-foreground hover:opacity-90 transition-opacity disabled:opacity-50"
          >
            {saving ? t("skill.saving") : t("skill.save")}
          </button>
        </div>
      </fieldset>
    </div>
  );
}

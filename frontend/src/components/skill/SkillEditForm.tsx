import { useState, useEffect } from "react";
import { useTranslation } from "react-i18next";
import type { SkillSource } from "@/components/content/types";
import { toErrorMessage } from "@/utils/error";
import { AppErr } from "@/utils/wailsResult";
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
  /** 在小说级/用户级之间移动当前技能；由父组件落盘并切换 tab 路径。 */
  onMove?: (to: "novel" | "user") => Promise<void>;
}

export default function SkillEditForm({
  content,
  isDirty,
  source,
  readOnly,
  onDraftChange,
  onSave,
  onCancel,
  onMove,
}: Props) {
  const { t } = useTranslation();

  const [draft, setDraft] = useState(() => parseDraft(content));
  const [saving, setSaving] = useState(false);
  const [moving, setMoving] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (isDirty) return;
    setDraft(parseDraft(content));
    setError("");
  }, [content, isDirty]);

  const updateDraft = (patch: Partial<SkillDraft>) => {
    if (saving || moving) return;
    const next = { ...draft, ...patch };
    setDraft(next);
    onDraftChange(serializeDraft(next));
  };

  // moveErrorText 把后端业务错误码映射到 i18n；未知错误回退到原始 message。
  const moveErrorText = (err: unknown) => {
    if (err instanceof AppErr) {
      switch (err.errCode) {
        case "skill.exists":
          return t("skill.nameExistsAtScope");
        case "skill.not_found":
          return t("skill.moveNotFound");
        case "skill.invalid_content":
          return t("skill.moveInvalidContent");
        case "skill.move_partial":
          return t("skill.movePartial");
        case "skill.invalid_name":
          return t("skill.invalidName");
      }
    }
    return toErrorMessage(err, t("skill.moveFailed"));
  };

  const handleMove = async () => {
    if (!onMove || (source !== "novel" && source !== "user")) return;
    if (saving || moving) return;
    if (isDirty) {
      setError(t("skill.moveSaveFirst"));
      return;
    }
    setMoving(true);
    setError("");
    try {
      await onMove(source === "novel" ? "user" : "novel");
    } catch (e) {
      setError(moveErrorText(e));
    } finally {
      setMoving(false);
    }
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
    if (saving || moving) return;
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
    if (e.key === "Escape" && !saving && !moving) onCancel();
  };

  return (
    <div className="overflow-y-auto h-full" onKeyDown={handleKeyDown}>
      <fieldset
        disabled={saving || moving}
        className="max-w-2xl mx-auto px-6 py-6 space-y-4"
      >
        {error && (
          <div className="sticky top-0 z-10 px-3 py-2.5 text-sm font-medium text-destructive-foreground bg-destructive border border-destructive rounded-md shadow-sm">
            {error}
          </div>
        )}

        {(source === "novel" || source === "user") && onMove && (
          <div>
            <label className="block text-xs font-medium text-muted-foreground mb-1.5">
              {t("skill.scopeLabel")}
            </label>
            <div className="flex items-center justify-between gap-3 rounded-md border bg-muted/30 px-3 py-2">
              <span className="text-sm">
                {source === "user"
                  ? t("skill.scopeUser")
                  : t("skill.scopeNovel")}
              </span>
              <button
                type="button"
                onClick={handleMove}
                disabled={saving || moving}
                className="h-8 px-3 rounded-md text-xs border bg-background hover:bg-muted transition-colors disabled:opacity-50"
              >
                {moving
                  ? t("skill.moving")
                  : source === "user"
                    ? t("skill.moveToNovel")
                    : t("skill.moveToUser")}
              </button>
            </div>
          </div>
        )}

        <SkillFormFields draft={draft} onChange={updateDraft} source={source} />

        <div className="flex justify-end gap-2 pt-2">
          <button
            onClick={onCancel}
            disabled={saving || moving}
            className="h-9 px-4 rounded-md text-sm border hover:bg-muted transition-colors disabled:opacity-50"
          >
            {t("skill.cancel")}
          </button>
          <button
            onClick={handleSave}
            disabled={saving || moving}
            className="h-9 px-4 rounded-md text-sm bg-primary text-primary-foreground hover:opacity-90 transition-opacity disabled:opacity-50"
          >
            {saving ? t("skill.saving") : t("skill.save")}
          </button>
        </div>
      </fieldset>
    </div>
  );
}

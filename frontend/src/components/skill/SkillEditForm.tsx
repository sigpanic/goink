import { useState, useEffect } from "react";
import { useTranslation } from "react-i18next";
import { splitFrontmatter, type SkillSource } from "@/components/content/types";

const KNOWN_FIELDS = [
  "name",
  "description",
  "category",
  "mode",
  "author",
  "version",
];

const MODE_OPTIONS = [
  { value: "auto", labelKey: "skill.modeSmart" },
  { value: "manual", labelKey: "skill.modeCommand" },
  { value: "always", labelKey: "skill.modePermanent" },
];

interface SkillDraft {
  name: string;
  description: string;
  category: string;
  mode: string;
  author: string;
  version: string;
  bodyText: string;
  extraFields: [string, string][];
}

function parseDraft(content: string): SkillDraft {
  const { meta, body } = splitFrontmatter(content);
  const frontmatterEnd = content.startsWith("---")
    ? content.indexOf("\n---", 3)
    : -1;
  const bodyText = frontmatterEnd < 0
    ? body
    : content.slice(frontmatterEnd + 4).replace(/^\n\n?/, "");
  return {
    name: meta.name || "",
    description: meta.description || "",
    category: meta.category || "",
    mode: meta.mode || "auto",
    author: meta.author || "",
    version: meta.version || "1",
    bodyText,
    extraFields: Object.entries(meta).filter(([key]) => !KNOWN_FIELDS.includes(key)),
  };
}

function serializeDraft(draft: SkillDraft): string {
  const lines = [
    "---",
    `name: ${draft.name.trim()}`,
    `description: ${draft.description.trim()}`,
    `category: ${draft.category.trim()}`,
    `mode: ${draft.mode}`,
  ];
  if (draft.author.trim()) lines.push(`author: ${draft.author.trim()}`);
  lines.push(`version: ${parseInt(draft.version) || 1}`);
  for (const [key, value] of draft.extraFields) lines.push(`${key}: ${value}`);
  lines.push("---", "", draft.bodyText);
  return lines.join("\n");
}

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
    if (!draft.name.trim()) {
      setError(t("skill.nameRequired"));
      return;
    }
    if (!draft.description.trim()) {
      setError(t("skill.summaryRequired"));
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
      <fieldset disabled={saving} className="max-w-2xl mx-auto px-6 py-6 space-y-4">
        {error && (
          <div className="sticky top-0 z-10 px-3 py-2.5 text-sm font-medium text-destructive-foreground bg-destructive border border-destructive rounded-md shadow-sm">
            {error}
          </div>
        )}

        <div>
          <label className="block text-xs font-medium text-muted-foreground mb-1.5">
            {t("skill.nameLabel")}
          </label>
          <input
            type="text"
            value={draft.name}
            onChange={(e) => updateDraft({ name: e.target.value })}
            placeholder={t("skill.namePlaceholder")}
            className="w-full h-9 rounded-md border bg-background px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            autoFocus
          />
        </div>

        <div>
          <label className="block text-xs font-medium text-muted-foreground mb-1.5">
            {t("skill.summaryLabel")}
          </label>
          <textarea
            value={draft.description}
            onChange={(e) => updateDraft({ description: e.target.value })}
            placeholder={t("skill.summaryPlaceholder")}
            rows={3}
            className="w-full rounded-md border bg-background px-3 py-2 text-sm resize-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
        </div>

        <div>
          <label className="block text-xs font-medium text-muted-foreground mb-1.5">
            {t("skill.category")}
          </label>
          <input
            type="text"
            value={draft.category}
            onChange={(e) => updateDraft({ category: e.target.value })}
            placeholder={t("skill.categoryPlaceholder")}
            className="w-full h-9 rounded-md border bg-background px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
        </div>

        <div>
          <label className="block text-xs font-medium text-muted-foreground mb-1.5">
            {t("skill.mode")}
          </label>
          <select
            value={draft.mode}
            onChange={(e) => updateDraft({ mode: e.target.value })}
            className="w-full h-9 rounded-md border bg-background px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            {MODE_OPTIONS.map((o) => (
              <option key={o.value} value={o.value}>
                {t(o.labelKey)}
              </option>
            ))}
          </select>
          {draft.mode === "always" && (source === "user" || source === "novel") && (
            <p className="mt-1.5 text-xs text-muted-foreground">
              {source === "user"
                ? t("skill.alwaysScopeUser")
                : t("skill.alwaysScopeNovel")}
            </p>
          )}
        </div>

        <div className="flex gap-4">
          <div className="flex-1">
            <label className="block text-xs font-medium text-muted-foreground mb-1.5">
              {t("skill.author")}
            </label>
            <input
              type="text"
              value={draft.author}
              onChange={(e) => updateDraft({ author: e.target.value })}
              placeholder={t("skill.authorPlaceholder")}
              className="w-full h-9 rounded-md border bg-background px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
          </div>
          <div className="w-24">
            <label className="block text-xs font-medium text-muted-foreground mb-1.5">
              {t("skill.version")}
            </label>
            <input
              type="number"
              value={draft.version}
              onChange={(e) => updateDraft({ version: e.target.value })}
              className="w-full h-9 rounded-md border bg-background px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
          </div>
        </div>

        <div>
          <label className="block text-xs font-medium text-muted-foreground mb-1.5">
            {t("skill.content")}
          </label>
          <textarea
            value={draft.bodyText}
            onChange={(e) => updateDraft({ bodyText: e.target.value })}
            placeholder={t("skill.contentPlaceholder")}
            rows={16}
            className="w-full rounded-md border bg-background px-3 py-2 text-sm resize-y focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
        </div>

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

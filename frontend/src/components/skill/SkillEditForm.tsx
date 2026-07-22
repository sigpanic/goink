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

interface Props {
  content: string;
  source?: SkillSource;
  readOnly?: boolean;
  onSave: (newContent: string) => Promise<void>;
  onCancel: () => void;
}

export default function SkillEditForm({
  content,
  source,
  readOnly,
  onSave,
  onCancel,
}: Props) {
  const { t } = useTranslation();

  const { meta, body } = splitFrontmatter(content);

  const [name, setName] = useState(meta.name || "");
  const [description, setDescription] = useState(meta.description || "");
  const [category, setCategory] = useState(meta.category || "");
  const [mode, setMode] = useState(meta.mode || "auto");
  const [author, setAuthor] = useState(meta.author || "");
  const [version, setVersion] = useState(meta.version || "1");
  const [bodyText, setBodyText] = useState(body || "");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [extraFields, setExtraFields] = useState<[string, string][]>([]);

  useEffect(() => {
    const { meta: m, body: b } = splitFrontmatter(content);
    setName(m.name || "");
    setDescription(m.description || "");
    setCategory(m.category || "");
    setMode(m.mode || "auto");
    setAuthor(m.author || "");
    setVersion(m.version || "1");
    setBodyText(b || "");
    setError("");
    const extras: [string, string][] = [];
    for (const [k, v] of Object.entries(m)) {
      if (!KNOWN_FIELDS.includes(k)) {
        extras.push([k, v]);
      }
    }
    setExtraFields(extras);
  }, [content]);

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
    if (!name.trim()) {
      setError(t("skill.nameRequired"));
      return;
    }
    if (!description.trim()) {
      setError(t("skill.summaryRequired"));
      return;
    }
    setSaving(true);
    setError("");
    try {
      const lines = [
        "---",
        `name: ${name.trim()}`,
        `description: ${description.trim()}`,
        `category: ${category.trim()}`,
        `mode: ${mode}`,
      ];
      if (author.trim()) {
        lines.push(`author: ${author.trim()}`);
      }
      lines.push(`version: ${parseInt(version) || 1}`);
      for (const [k, v] of extraFields) {
        lines.push(`${k}: ${v}`);
      }
      lines.push("---", "", bodyText.trim());
      await onSave(lines.join("\n"));
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
    if (e.key === "Escape") onCancel();
  };

  return (
    <div className="overflow-y-auto h-full" onKeyDown={handleKeyDown}>
      <div className="max-w-2xl mx-auto px-6 py-6 space-y-4">
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
            value={name}
            onChange={(e) => setName(e.target.value)}
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
            value={description}
            onChange={(e) => setDescription(e.target.value)}
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
            value={category}
            onChange={(e) => setCategory(e.target.value)}
            placeholder={t("skill.categoryPlaceholder")}
            className="w-full h-9 rounded-md border bg-background px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
        </div>

        <div>
          <label className="block text-xs font-medium text-muted-foreground mb-1.5">
            {t("skill.mode")}
          </label>
          <select
            value={mode}
            onChange={(e) => setMode(e.target.value)}
            className="w-full h-9 rounded-md border bg-background px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            {MODE_OPTIONS.map((o) => (
              <option key={o.value} value={o.value}>
                {t(o.labelKey)}
              </option>
            ))}
          </select>
          {mode === "always" && (source === "user" || source === "novel") && (
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
              value={author}
              onChange={(e) => setAuthor(e.target.value)}
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
              value={version}
              onChange={(e) => setVersion(e.target.value)}
              className="w-full h-9 rounded-md border bg-background px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
          </div>
        </div>

        <div>
          <label className="block text-xs font-medium text-muted-foreground mb-1.5">
            {t("skill.content")}
          </label>
          <textarea
            value={bodyText}
            onChange={(e) => setBodyText(e.target.value)}
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
      </div>
    </div>
  );
}

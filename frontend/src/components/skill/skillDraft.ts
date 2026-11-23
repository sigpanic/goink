import { splitFrontmatter } from "@/components/content/types";

const KNOWN_FIELDS = [
  "name",
  "description",
  "category",
  "mode",
  "author",
  "version",
];

export const MODE_OPTIONS = [
  { value: "auto", labelKey: "skill.modeSmart" },
  { value: "manual", labelKey: "skill.modeCommand" },
  { value: "always", labelKey: "skill.modePermanent" },
];

export interface SkillDraft {
  name: string;
  description: string;
  category: string;
  mode: string;
  author: string;
  version: string;
  bodyText: string;
  extraFields: [string, string][];
}

// emptyDraft 返回新建技能的初始草稿。默认值与 parseDraft 对缺失字段的兜底保持一致。
export function emptyDraft(): SkillDraft {
  return {
    name: "",
    description: "",
    category: "",
    mode: "auto",
    author: "",
    version: "1",
    bodyText: "",
    extraFields: [],
  };
}

export function parseDraft(content: string): SkillDraft {
  const { meta, body } = splitFrontmatter(content);
  const frontmatterEnd = content.startsWith("---")
    ? content.indexOf("\n---", 3)
    : -1;
  const bodyText =
    frontmatterEnd < 0
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
    extraFields: Object.entries(meta).filter(
      ([key]) => !KNOWN_FIELDS.includes(key),
    ),
  };
}

export function serializeDraft(draft: SkillDraft): string {
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

// validateDraft 返回第一个缺失必填项对应的 i18n key，全部通过时返回 null。
export function validateDraft(
  draft: SkillDraft,
): "skill.nameRequired" | "skill.summaryRequired" | null {
  if (!draft.name.trim()) return "skill.nameRequired";
  if (!draft.description.trim()) return "skill.summaryRequired";
  return null;
}

// validateSkillName 镜像 internal/skill.ValidateName：创建时名称会作为 <name>.md
// 文件名使用，因此拒绝路径分隔符、Windows 非法字符与 .md 后缀。
// 调用方应传入 trim 后的名称。
export function validateSkillName(
  name: string,
): "skill.nameRequired" | "skill.invalidName" | null {
  if (!name.trim()) return "skill.nameRequired";
  if (name !== name.trim()) return "skill.invalidName";
  if (name === "." || name === "..") return "skill.invalidName";
  if (/[/\\]/.test(name)) return "skill.invalidName";
  if (/[:*?"<>|]/.test(name)) return "skill.invalidName";
  if (name.toLowerCase().endsWith(".md")) return "skill.invalidName";
  return null;
}

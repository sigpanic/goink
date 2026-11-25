import { useTranslation } from "react-i18next";
import type { SkillSource } from "@/components/content/types";
import AutoGrowTextarea from "@/components/ui/AutoGrowTextarea";
import PopSelect from "@/components/shared/PopSelect";
import { MODE_OPTIONS, type SkillDraft } from "./skillDraft";

interface Props {
  draft: SkillDraft;
  onChange: (patch: Partial<SkillDraft>) => void;
  source?: SkillSource;
  /** 正文编辑区的高度范围：初始高度与增长上限，超过上限后内部滚动。 */
  contentHeight?: { min: number; max: number };
}

// SkillFormFields 是技能编辑表单与新建弹窗共用的字段区（不含错误条与操作按钮）。
export default function SkillFormFields({
  draft,
  onChange,
  source,
  contentHeight,
}: Props) {
  const { t } = useTranslation();
  const { min: contentMinHeight = 384, max: contentMaxHeight = 640 } =
    contentHeight ?? {};

  return (
    <>
      <div>
        <label className="block text-xs font-medium text-muted-foreground mb-1.5">
          {t("skill.nameLabel")}
        </label>
        <input
          type="text"
          value={draft.name}
          onChange={(e) => onChange({ name: e.target.value })}
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
          onChange={(e) => onChange({ description: e.target.value })}
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
          onChange={(e) => onChange({ category: e.target.value })}
          placeholder={t("skill.categoryPlaceholder")}
          className="w-full h-9 rounded-md border bg-background px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        />
      </div>

      <div>
        <label className="block text-xs font-medium text-muted-foreground mb-1.5">
          {t("skill.mode")}
        </label>
        <PopSelect
          ariaLabel={t("skill.mode")}
          size="form"
          dropUp={false}
          className="w-full"
          minWidth="0"
          value={draft.mode}
          options={MODE_OPTIONS.map((o) => ({
            value: o.value,
            label: t(o.labelKey),
          }))}
          onChange={(value) => onChange({ mode: value })}
        />
        {draft.mode === "always" &&
          (source === "user" || source === "novel") && (
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
            onChange={(e) => onChange({ author: e.target.value })}
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
            onChange={(e) => onChange({ version: e.target.value })}
            className="w-full h-9 rounded-md border bg-background px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
        </div>
      </div>

      <div>
        <label className="block text-xs font-medium text-muted-foreground mb-1.5">
          {t("skill.content")}
        </label>
        <AutoGrowTextarea
          value={draft.bodyText}
          onChange={(e) => onChange({ bodyText: e.target.value })}
          placeholder={t("skill.contentPlaceholder")}
          minHeight={contentMinHeight}
          maxHeight={contentMaxHeight}
          className="w-full resize-none overflow-y-auto rounded-md border bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        />
      </div>
    </>
  );
}

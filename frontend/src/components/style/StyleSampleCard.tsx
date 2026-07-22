import { Check, Trash2 } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { style } from "@/lib/wailsjs/go/models";

function formatDate(v: any): string {
  if (!v) return "";
  if (typeof v === "string")
    return v.length >= 16 ? v.slice(0, 16).replace("T", " ") : v;
  try {
    const d = new Date(v);
    return isNaN(d.getTime())
      ? ""
      : d.toISOString().slice(0, 16).replace("T", " ");
  } catch {
    return "";
  }
}

interface Props {
  sample: style.Sample;
  selected: boolean;
  onToggle: () => void;
  onDelete: () => void;
  onClick: () => void;
}

export default function StyleSampleCard({
  sample,
  selected,
  onToggle,
  onDelete,
  onClick,
}: Props) {
  const { t } = useTranslation();
  return (
    <div
      className={`group relative flex flex-col rounded-[24px] p-5 transition-all duration-300 select-none
        bg-card/80 backdrop-blur-2xl border
        ${
          selected
            ? "ring-2 ring-primary/40 border-primary/30 shadow-[0_8px_32px_rgba(14,165,233,0.12)]"
            : "border-white/15 hover:border-primary/20 hover:shadow-lg hover:-translate-y-0.5"
        }`}
    >
      {/* 头部：点击区域 = 选中/取消 */}
      <div
        onClick={onToggle}
        className="flex items-start justify-between mb-2 cursor-pointer"
      >
        <h3 className="text-base font-semibold text-foreground truncate flex-1 mr-2 pt-0.5">
          {sample.name}
        </h3>

        <div
          className={`w-6 h-6 rounded-lg border-2 flex items-center justify-center transition-all duration-200 shrink-0
            ${
              selected
                ? "bg-primary border-primary text-primary-foreground shadow-sm"
                : "border-muted-foreground/30 group-hover:border-primary/50"
            }`}
        >
          {selected && <Check className="w-3.5 h-3.5" />}
        </div>
      </div>

      {/* 内容：点击区域 = 编辑 */}
      <div onClick={onClick} className="flex-1 flex flex-col cursor-pointer">
        <p className="text-[13px] text-muted-foreground leading-relaxed line-clamp-3 mb-3">
          {sample.preview}
        </p>

        {sample.tags.length > 0 && (
          <div className="flex flex-wrap gap-1.5 mb-3">
            {sample.tags.map((tag) => (
              <span
                key={tag}
                className="inline-block text-[11px] px-2 py-0.5 rounded-full
                  bg-gradient-to-r from-primary/10 to-accent/10
                  text-primary/80 border border-primary/10"
              >
                {tag}
              </span>
            ))}
          </div>
        )}

        <div className="flex items-center justify-between gap-3 text-[11px] text-muted-foreground/60 mt-auto">
          <div className="flex items-center gap-3">
            <span>
              {sample.word_count} {t("styleSample.charCount")}
            </span>
            <span className="w-px h-2.5 bg-border" />
            <span>{formatDate(sample.created_at)}</span>
          </div>
          <button
            onClick={(e) => {
              e.stopPropagation();
              onDelete();
            }}
            className="w-6 h-6 rounded-md flex items-center justify-center
              opacity-0 group-hover:opacity-100 transition-all duration-200
              hover:bg-destructive/10 text-muted-foreground hover:text-destructive shrink-0"
            title={t("styleSample.deleteSample")}
          >
            <Trash2 className="w-3.5 h-3.5" />
          </button>
        </div>
      </div>
    </div>
  );
}

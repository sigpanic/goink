import { useState, useRef, useEffect, useMemo, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { ChevronUp } from "lucide-react";
import type { llm } from "@/lib/wailsjs/go/models";
import ProviderIcon from "@/components/settings/ProviderIcon";

interface FooterAction {
  label: string;
  onClick: () => void;
}

interface Props {
  models: llm.AvailableModel[];
  selectedKey: string;
  // reasoning：传入则启用底部跟随区域。当前选中模型不支持时不显示。
  reasoningEffort?: string;
  onSelectModel: (key: string) => void;
  onSelectEffort?: (effort: string) => void;
  onOpen?: () => void;
  footerAction?: FooterAction;
  minWidth?: string;
  className?: string;
  dropUp?: boolean; // true=向上弹出(默认), false=向下弹出
  placeholder?: string;
}

// ModelPicker: 模型选择 + reasoning 跟随，三处复用（ChatControls / PatternExtractView / StyleView）。
// 列表按 ProviderKey 聚类（同名 model 靠所在 provider 组上下文区分）。
// 弹层结构：model 列表区独立滚动，reasoning + footer 固定底部不滚（reasoning 永远可见）。
// reasoning 区域：仅当 reasoningEffort/onSelectEffort 传入 且 当前选中模型 ReasoningLevels 非空时渲染。
export default function ModelPicker({
  models,
  selectedKey,
  reasoningEffort,
  onSelectModel,
  onSelectEffort,
  onOpen,
  footerAction,
  minWidth = "130px",
  className = "",
  dropUp = true,
  placeholder,
}: Props) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);

  const selected = models.find((m) => m.Key === selectedKey);
  const supportsReasoning =
    !!selected?.ReasoningLevels && selected.ReasoningLevels.length > 0;
  const showReasoning = supportsReasoning && !!onSelectEffort;

  // 按 ProviderKey 聚类，保持原始顺序。同一组内 providerKey/providerName 一致。
  const groups = useMemo(() => {
    const map = new Map<string, llm.AvailableModel[]>();
    for (const m of models) {
      const key = m.ProviderKey || "";
      const arr = map.get(key) ?? [];
      arr.push(m);
      map.set(key, arr);
    }
    return Array.from(map.entries()).map(([providerKey, items]) => ({
      providerKey,
      providerName: items[0]?.ProviderName || providerKey,
      items,
    }));
  }, [models]);

  const handleToggle = () => {
    if (!open && onOpen) onOpen();
    setOpen(!open);
  };

  useEffect(() => {
    if (!open) return;
    const handleClick = (e: MouseEvent) => {
      if (
        containerRef.current &&
        !containerRef.current.contains(e.target as Node)
      ) {
        setOpen(false);
      }
    };
    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, [open]);

  const reasoningOptions = supportsReasoning
    ? selected!.ReasoningLevels!.map((level) => ({
        value: level,
        label:
          level === "high" ? t("chat.highReasoning") : t("chat.maxReasoning"),
      }))
    : [];

  // provider 标题行 icon：内置 LOGOS 匹配则用 logo，否则 fallback 首字母圆形（与设置页 Custom 风格一致）。
  const renderProviderIcon = (providerKey: string, providerName: string): ReactNode => (
    <ProviderIcon
      provider={providerKey}
      className="w-4 h-4 shrink-0 text-muted-foreground"
      fallback={
        <span className="w-4 h-4 rounded-full bg-muted-foreground/20 text-[10px] flex items-center justify-center text-muted-foreground shrink-0">
          {(providerName || "?").charAt(0).toUpperCase()}
        </span>
      }
    />
  );

  return (
    <div ref={containerRef} className={`relative ${className}`}>
      <button
        onClick={handleToggle}
        style={{ minWidth }}
        className="h-[30px] rounded-lg border bg-background px-2.5 text-xs text-muted-foreground flex items-center justify-between gap-1 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
      >
        <span className="truncate">
          {selected?.ModelName || placeholder || t("chat.noModelAvailable")}
        </span>
        <ChevronUp
          className={`w-3 h-3 shrink-0 transition-transform ${open === dropUp ? "rotate-180" : ""}`}
        />
      </button>

      {open && (
        <div
          className={`absolute left-0 w-[240px] flex flex-col max-h-[300px] rounded-lg border bg-background shadow-lg z-50 ${dropUp ? "bottom-full mb-1" : "top-full mt-1"}`}
        >
          {/* 按 provider 聚类的 model 列表 —— 独立滚动区 */}
          <div className="flex-1 overflow-y-auto py-1">
            {groups.map((g) => (
              <div key={g.providerKey} className="py-0.5">
                <div className="flex items-center gap-2 px-2.5 py-1.5 text-xs font-medium text-foreground">
                  {renderProviderIcon(g.providerKey, g.providerName)}
                  <span className="truncate">{g.providerName || "—"}</span>
                </div>
                {g.items.map((m) => (
                  <button
                    key={m.Key}
                    onClick={() => {
                      onSelectModel(m.Key);
                    }}
                    className={`w-full text-left pl-8 pr-2.5 py-1.5 text-xs hover:bg-muted transition-colors ${
                      m.Key === selectedKey
                        ? "bg-primary/10 text-primary font-medium"
                        : "text-muted-foreground"
                    }`}
                  >
                    {m.ModelName}
                  </button>
                ))}
              </div>
            ))}
          </div>

          {/* reasoning 区域：固定底部不滚动，当前选中 model 支持时显示，否则隐藏 */}
          {showReasoning && (
            <>
              <div className="border-t" />
              <div className="px-2.5 py-1.5 shrink-0">
                <div className="text-[10px] uppercase tracking-wide text-muted-foreground/70 mb-1">
                  {t("chat.reasoning")}
                </div>
                <div className="flex gap-1.5">
                  {reasoningOptions.map((opt) => (
                    <button
                      key={opt.value}
                      onClick={() => onSelectEffort!(opt.value)}
                      className={`flex-1 px-2 py-1 rounded-md text-[11px] transition-colors ${
                        opt.value === reasoningEffort
                          ? "bg-primary/10 text-primary border border-primary/30"
                          : "border border-border text-muted-foreground hover:bg-muted"
                      }`}
                    >
                      {opt.label}
                    </button>
                  ))}
                </div>
              </div>
            </>
          )}

          {/* footer：配置入口，固定底部不滚动 */}
          {footerAction && (
            <>
              <div className="border-t" />
              <button
                onClick={() => {
                  footerAction.onClick();
                  setOpen(false);
                }}
                className="w-full text-left px-2.5 py-1.5 text-xs text-muted-foreground hover:bg-muted transition-colors shrink-0"
              >
                ⚙ {footerAction.label}
              </button>
            </>
          )}
        </div>
      )}
    </div>
  );
}

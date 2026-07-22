import { useEffect, useRef, useMemo } from "react";
import { useTranslation } from "react-i18next";
import { createPortal } from "react-dom";
import { Zap, Play, Star } from "lucide-react";
import type { app } from "@/hooks/useApp";

const MODE_ICON: Record<string, React.ReactNode> = {
  auto: <Zap className="w-3.5 h-3.5 text-tag-amber-foreground shrink-0" />,
  manual: <Play className="w-3.5 h-3.5 text-tag-blue-foreground shrink-0" />,
  always: <Star className="w-3.5 h-3.5 text-tag-green-foreground shrink-0" />,
};

const MODE_SELECTED_BG: Record<string, string> = {
  auto: "bg-tag-amber",
  manual: "bg-tag-blue",
  always: "bg-tag-green",
};

interface Props {
  slashItems: app.SlashCommand[];
  selectedIndex: number;
  position: { top: number; left: number; width: number };
  onSelect: (cmd: app.SlashCommand) => void;
  onHover: (index: number) => void;
}

const MENU_MAX_HEIGHT = 260;
const GAP = 8;

export default function SlashMenu({
  slashItems,
  selectedIndex,
  position,
  onSelect,
  onHover,
}: Props) {
  const { t } = useTranslation();
  const scrollRef = useRef<HTMLDivElement>(null);

  const MODE_LABEL: Record<string, string> = {
    auto: t("chat.smart"),
    manual: t("chat.command"),
    always: t("chat.permanent"),
  };

  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    const item = el.children[selectedIndex] as HTMLElement | undefined;
    if (item) {
      item.scrollIntoView({ block: "nearest" });
    }
  }, [selectedIndex]);

  const style = useMemo(() => {
    const spaceAbove = position.top - GAP;
    const maxH = Math.min(MENU_MAX_HEIGHT, spaceAbove);
    const menuWidth = position.width;
    let left = position.left;
    if (left + menuWidth > window.innerWidth - GAP) {
      left = Math.max(GAP, window.innerWidth - menuWidth - GAP);
    }
    return {
      left,
      width: menuWidth,
      maxHeight: maxH,
      bottom: window.innerHeight - position.top + GAP,
    };
  }, [position]);

  if (slashItems.length === 0) {
    return createPortal(
      <div
        className="fixed z-[9999] rounded-lg border bg-background shadow-lg px-3 py-2 text-xs text-muted-foreground"
        style={{
          bottom: style.bottom,
          left: style.left,
          minWidth: style.width,
        }}
      >
        {t("chat.noMatchingCommand")}
      </div>,
      document.body,
    );
  }

  return createPortal(
    <div
      className="fixed z-[9999] rounded-lg border bg-background shadow-lg overflow-hidden flex flex-col"
      style={{
        bottom: style.bottom,
        left: style.left,
        width: style.width,
        maxHeight: style.maxHeight,
      }}
    >
      <div
        ref={scrollRef}
        className="overflow-y-auto"
        style={{ maxHeight: style.maxHeight }}
      >
        {slashItems.map((c, i) => {
          const mode = c.type || "auto";
          const selBg = MODE_SELECTED_BG[mode] || MODE_SELECTED_BG.auto;
          return (
            <button
              key={`${mode}:${c.name}`}
              onMouseDown={(e) => {
                e.preventDefault();
                onSelect(c);
              }}
              onMouseEnter={() => onHover(i)}
              className={`w-full text-left px-3 py-2 transition-colors ${
                i === selectedIndex ? selBg : "hover:bg-muted/60"
              }`}
            >
              <div className="flex items-center gap-2 min-w-0">
                {MODE_ICON[mode] || MODE_ICON.auto}
                <span className="text-sm font-medium text-foreground truncate">
                  {c.name}
                </span>
                <span className="text-[10px] text-muted-foreground shrink-0 ml-auto">
                  {MODE_LABEL[mode] || MODE_LABEL.auto}
                </span>
              </div>
              {c.description && (
                <div className="text-xs text-muted-foreground line-clamp-2 mt-0.5">
                  {c.description}
                </div>
              )}
            </button>
          );
        })}
      </div>
    </div>,
    document.body,
  );
}

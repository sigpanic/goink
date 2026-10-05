import { useRef, useEffect, useLayoutEffect } from "react";
import { X } from "lucide-react";
import { useTranslation } from "react-i18next";

interface Props {
  tabs: { id: string; type: string; title: string }[];
  activeTabId: string | null;
  onSelect: (id: string) => void;
  onClose: (id: string) => void;
}

export default function TabBar({
  tabs,
  activeTabId,
  onSelect,
  onClose,
}: Props) {
  const { t } = useTranslation();
  const scrollRef = useRef<HTMLDivElement>(null);
  const activeTabRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    const onWheel = (e: WheelEvent) => {
      // 仅在 tab 栏横向溢出时把纵向滚轮转为横向滚动，避免 tab 较少时拦截滚轮
      if (el.scrollWidth <= el.clientWidth) return;
      e.preventDefault();
      el.scrollLeft += e.deltaY;
    };
    el.addEventListener("wheel", onWheel, { passive: false });
    return () => el.removeEventListener("wheel", onWheel);
  }, [tabs.length]);

  useLayoutEffect(() => {
    const scroll = scrollRef.current;
    const activeTab = activeTabRef.current;
    if (!scroll || !activeTab) return;

    const scrollBounds = scroll.getBoundingClientRect();
    const tabBounds = activeTab.getBoundingClientRect();
    if (tabBounds.left < scrollBounds.left) {
      scroll.scrollLeft += tabBounds.left - scrollBounds.left;
    } else if (tabBounds.right > scrollBounds.right) {
      scroll.scrollLeft += tabBounds.right - scrollBounds.right;
    }
  }, [activeTabId, tabs.length]);

  if (tabs.length === 0) return null;

  return (
    <div
      ref={scrollRef}
      className="flex items-center bg-muted/30 border-b shrink-0 overflow-x-auto"
    >
      {tabs.map((tab) => {
        const isActive = tab.id === activeTabId;
        return (
          <div
            key={tab.id}
            ref={isActive ? activeTabRef : undefined}
            className={`group flex items-center gap-1 px-3 py-1.5 text-xs cursor-pointer border-r shrink-0 transition-colors select-none ${
              isActive
                ? "bg-background text-foreground border-t-2 border-t-blue-500 -mt-[1px] hover:bg-primary/5"
                : "text-muted-foreground hover:bg-primary/15 hover:text-foreground"
            } ${tab.type === "diff" ? "italic" : ""}`}
            onClick={() => onSelect(tab.id)}
          >
            <span className="truncate max-w-[160px]">{tab.title}</span>
            <button
              aria-label={`${t("common.close")}: ${tab.title}`}
              className={`ml-0.5 p-0.5 rounded hover:bg-primary/25 hover:text-primary transition-[opacity,background-color,color] cursor-pointer focus-visible:outline-2 focus-visible:outline-ring ${
                isActive
                  ? "opacity-100"
                  : "opacity-0 pointer-events-none group-hover:opacity-100 group-hover:pointer-events-auto group-focus-within:opacity-100 group-focus-within:pointer-events-auto"
              }`}
              onClick={(e) => {
                e.stopPropagation();
                onClose(tab.id);
              }}
            >
              <X className="w-3 h-3" />
            </button>
          </div>
        );
      })}
    </div>
  );
}

import { useRef, useEffect } from "react";
import { X } from "lucide-react";

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
  const scrollRef = useRef<HTMLDivElement>(null);

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

  if (tabs.length === 0) return null;

  return (
    <div
      ref={scrollRef}
      className="flex items-center bg-muted/30 border-b shrink-0 overflow-x-auto"
    >
      {tabs.map((tab) => (
        <div
          key={tab.id}
          className={`group flex items-center gap-1 px-3 py-1.5 text-xs cursor-pointer border-r shrink-0 transition-colors select-none ${
            tab.id === activeTabId
              ? "bg-background text-foreground border-t-2 border-t-blue-500 -mt-[1px]"
              : "text-muted-foreground hover:bg-muted/50"
          } ${tab.type === "diff" ? "italic" : ""}`}
          onClick={() => onSelect(tab.id)}
        >
          <span className="truncate max-w-[160px]">{tab.title}</span>
          <button
            className="ml-0.5 p-0.5 rounded opacity-0 group-hover:opacity-100 hover:bg-muted transition-opacity cursor-pointer"
            onClick={(e) => {
              e.stopPropagation();
              onClose(tab.id);
            }}
          >
            <X className="w-3 h-3" />
          </button>
        </div>
      ))}
    </div>
  );
}

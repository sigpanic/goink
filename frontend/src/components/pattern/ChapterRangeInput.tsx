import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ChevronDown, Plus, X } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { chapter } from "@/lib/wailsjs/go/models";

interface Props {
  chapters: chapter.Chapter[];
  onSelect: (selectedIds: Set<number>) => void;
  disabled?: boolean;
}

interface RangePair {
  start: string;
  end: string;
}

export default function ChapterRangeInput({
  chapters,
  onSelect,
  disabled,
}: Props) {
  const { t } = useTranslation();
  const [ranges, setRanges] = useState<RangePair[]>([{ start: "", end: "" }]);
  const [open, setOpen] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);

  const parsePair = useCallback((start: string, end: string): Set<number> => {
    const lo = Number(start);
    const hi = Number(end);
    if (!Number.isFinite(lo) || !Number.isFinite(hi) || lo <= 0 || hi <= 0)
      return new Set();
    const loC = Math.min(lo, hi);
    const hiC = Math.max(lo, hi);
    const nums = new Set<number>();
    for (let i = loC; i <= hiC; i++) nums.add(i);
    return nums;
  }, []);

  const selectedIds = useMemo(() => {
    const allNums = new Set<number>();
    for (const r of ranges) {
      const nums = parsePair(r.start, r.end);
      for (const n of nums) allNums.add(n);
    }
    return new Set(
      chapters
        .filter((ch) => allNums.has(ch.chapter_number))
        .map((ch) => ch.id),
    );
  }, [ranges, chapters, parsePair]);

  const prevSelectedIdsRef = useRef<Set<number> | null>(null);
  useEffect(() => {
    if (prevSelectedIdsRef.current !== null) {
      onSelect(selectedIds);
    }
    prevSelectedIdsRef.current = selectedIds;
  }, [selectedIds, onSelect]);

  const updateRange = useCallback(
    (index: number, field: "start" | "end", value: string) => {
      setRanges((prev) => {
        const next = [...prev];
        next[index] = { ...next[index], [field]: value };
        return next;
      });
    },
    [],
  );

  const addRange = useCallback(() => {
    setRanges((prev) => {
      const next = [...prev, { start: "", end: "" }];
      // 添加后超过 1 对时自动打开下拉框
      if (next.length > 1) setOpen(true);
      return next;
    });
  }, []);

  const removeRange = useCallback((index: number) => {
    setRanges((prev) =>
      prev.length <= 1 ? prev : prev.filter((_, i) => i !== index),
    );
  }, []);

  // 点击外部关闭下拉
  useEffect(() => {
    if (!open) return;
    const handleClickOutside = (e: MouseEvent) => {
      if (
        containerRef.current &&
        !containerRef.current.contains(e.target as Node)
      ) {
        setOpen(false);
      }
    };
    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, [open]);

  const inputClass =
    "h-7 w-14 rounded-md border border-border bg-background px-1.5 text-xs text-center [appearance:textfield] [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none focus:border-primary focus:outline-none";

  const extraCount = ranges.length - 1;

  return (
    <div ref={containerRef} className="relative flex items-center gap-1.5">
      {/* 第一对：始终在行内 */}
      <input
        type="number"
        min={1}
        placeholder={t("extract.rangeStart")}
        value={ranges[0].start}
        onChange={(e) => updateRange(0, "start", e.target.value)}
        disabled={disabled}
        className={inputClass}
      />
      <span className="text-[10px] text-muted-foreground">-</span>
      <input
        type="number"
        min={1}
        placeholder={t("extract.rangeEnd")}
        value={ranges[0].end}
        onChange={(e) => updateRange(0, "end", e.target.value)}
        disabled={disabled}
        className={inputClass}
      />

      {/* 有额外范围时：+N 下拉按钮 */}
      {extraCount > 0 && (
        <button
          onClick={() => setOpen((prev) => !prev)}
          disabled={disabled}
          className="h-7 px-1.5 rounded-md border border-border text-xs text-muted-foreground hover:bg-muted hover:text-foreground disabled:opacity-30 transition-colors flex items-center gap-0.5"
        >
          +{extraCount}
          <ChevronDown
            className={`w-3 h-3 transition-transform ${open ? "rotate-180" : ""}`}
          />
        </button>
      )}

      {/* + 按钮 */}
      <button
        onClick={addRange}
        disabled={disabled}
        className="h-7 w-7 flex items-center justify-center rounded-md border border-dashed border-border text-muted-foreground hover:bg-muted hover:text-foreground disabled:opacity-30 transition-colors"
        title={t("extract.rangeAdd")}
      >
        <Plus className="w-3.5 h-3.5" />
      </button>

      {/* 浮层下拉框：显示额外范围对 */}
      {open && extraCount > 0 && (
        <div className="absolute top-full left-0 mt-1 z-10 rounded-md border bg-popover p-2 shadow-md min-w-[200px]">
          <div className="space-y-1.5">
            {ranges.slice(1).map((r, i) => {
              const idx = i + 1;
              return (
                <div key={idx} className="flex items-center gap-1">
                  <input
                    type="number"
                    min={1}
                    placeholder={t("extract.rangeStart")}
                    value={r.start}
                    onChange={(e) => updateRange(idx, "start", e.target.value)}
                    disabled={disabled}
                    className={inputClass}
                  />
                  <span className="text-[10px] text-muted-foreground">-</span>
                  <input
                    type="number"
                    min={1}
                    placeholder={t("extract.rangeEnd")}
                    value={r.end}
                    onChange={(e) => updateRange(idx, "end", e.target.value)}
                    disabled={disabled}
                    className={inputClass}
                  />
                  <button
                    onClick={() => removeRange(idx)}
                    disabled={disabled}
                    className="h-6 w-6 flex items-center justify-center rounded text-muted-foreground hover:bg-muted hover:text-foreground disabled:opacity-30 transition-colors"
                  >
                    <X className="w-3 h-3" />
                  </button>
                </div>
              );
            })}
          </div>
          <div className="mt-1.5 pt-1.5 border-t border-border">
            <button
              onClick={addRange}
              disabled={disabled}
              className="h-6 w-full flex items-center justify-center rounded-md border border-dashed border-border text-muted-foreground hover:bg-muted hover:text-foreground disabled:opacity-30 transition-colors"
              title={t("extract.rangeAdd")}
            >
              <Plus className="w-3 h-3" />
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

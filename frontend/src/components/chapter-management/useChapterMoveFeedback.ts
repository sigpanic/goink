import { useEffect, useLayoutEffect, useRef, useState } from "react";
import type { chapter } from "@/lib/wailsjs/go/models";

type Position = { top: number; left: number };

function reduceMotion() {
  return (
    window.matchMedia?.("(prefers-reduced-motion: reduce)").matches ?? false
  );
}

export function useChapterMoveFeedback(chapters: chapter.Chapter[]) {
  const containerRef = useRef<HTMLElement | null>(null);
  const previousPositions = useRef<Map<number, Position> | null>(null);
  const [highlightedId, setHighlightedId] = useState<number | null>(null);
  const [revealedId, setRevealedId] = useState<number | null>(null);

  function capturePositions() {
    if (reduceMotion()) return;
    const positions = new Map<number, Position>();
    for (const row of containerRef.current?.querySelectorAll<HTMLElement>(
      "[data-chapter-id]",
    ) ?? []) {
      const rect = row.getBoundingClientRect();
      if (rect.bottom < 0 || rect.top > window.innerHeight) continue;
      positions.set(Number(row.dataset.chapterId), {
        top: rect.top,
        left: rect.left,
      });
    }
    previousPositions.current = positions;
  }

  function cancelMove() {
    previousPositions.current = null;
  }

  function showMovedChapter(id: number) {
    setRevealedId(id);
    setHighlightedId(id);
  }

  function dismissReveal() {
    setRevealedId(null);
  }

  useLayoutEffect(() => {
    const positions = previousPositions.current;
    if (!positions) return;
    previousPositions.current = null;
    if (reduceMotion()) return;
    for (const row of containerRef.current?.querySelectorAll<HTMLElement>(
      "[data-chapter-id]",
    ) ?? []) {
      const previous = positions.get(Number(row.dataset.chapterId));
      if (!previous) continue;
      const rect = row.getBoundingClientRect();
      const dx = previous.left - rect.left;
      const dy = previous.top - rect.top;
      if (Math.abs(dx) + Math.abs(dy) < 2 || Math.abs(dy) > 600) continue;
      row.animate?.(
        [
          { transform: `translate(${dx}px, ${dy}px)` },
          { transform: "translate(0, 0)" },
        ],
        { duration: 360, easing: "cubic-bezier(0.2, 0.8, 0.2, 1)" },
      );
    }
  }, [chapters, highlightedId]);

  useEffect(() => {
    if (highlightedId === null) return;
    const frame = window.requestAnimationFrame?.(() => {
      const row = containerRef.current?.querySelector<HTMLElement>(
        `[data-chapter-id="${highlightedId}"]`,
      );
      row?.scrollIntoView?.({
        behavior: reduceMotion() ? "auto" : "smooth",
        block: "nearest",
      });
    });
    const timeout = window.setTimeout(() => setHighlightedId(null), 1800);
    return () => {
      if (frame !== undefined) window.cancelAnimationFrame?.(frame);
      window.clearTimeout(timeout);
    };
  }, [highlightedId]);

  return {
    containerRef,
    highlightedId,
    revealedId,
    capturePositions,
    cancelMove,
    showMovedChapter,
    dismissReveal,
  };
}

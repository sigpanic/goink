import { useEffect, useLayoutEffect, useRef, useState } from "react";
import type { volume } from "@/lib/wailsjs/go/models";

function reduceMotion() {
  return (
    window.matchMedia?.("(prefers-reduced-motion: reduce)").matches ?? false
  );
}

export function useVolumeMoveFeedback(volumes: volume.Volume[]) {
  const railRef = useRef<HTMLDivElement | null>(null);
  const previousPositions = useRef<Map<number, number> | null>(null);
  const [highlightedId, setHighlightedId] = useState<number | null>(null);

  function capturePositions() {
    if (reduceMotion()) return;
    const positions = new Map<number, number>();
    for (const chip of railRef.current?.querySelectorAll<HTMLElement>(
      "[data-volume-id]",
    ) ?? []) {
      const rect = chip.getBoundingClientRect();
      positions.set(Number(chip.dataset.volumeId), rect.left);
    }
    previousPositions.current = positions;
  }

  function cancelMove() {
    previousPositions.current = null;
  }

  function showMovedVolume(id: number) {
    setHighlightedId(id);
  }

  useLayoutEffect(() => {
    const positions = previousPositions.current;
    if (!positions) return;
    previousPositions.current = null;
    if (reduceMotion()) return;
    for (const chip of railRef.current?.querySelectorAll<HTMLElement>(
      "[data-volume-id]",
    ) ?? []) {
      const previousLeft = positions.get(Number(chip.dataset.volumeId));
      if (previousLeft === undefined) continue;
      const dx = previousLeft - chip.getBoundingClientRect().left;
      if (Math.abs(dx) < 2 || Math.abs(dx) > 700) continue;
      chip.animate?.(
        [{ transform: `translateX(${dx}px)` }, { transform: "translateX(0)" }],
        { duration: 360, easing: "cubic-bezier(0.2, 0.8, 0.2, 1)" },
      );
    }
  }, [volumes, highlightedId]);

  useEffect(() => {
    if (highlightedId === null) return;
    const frame = window.requestAnimationFrame?.(() => {
      railRef.current
        ?.querySelector<HTMLElement>(`[data-volume-id="${highlightedId}"]`)
        ?.scrollIntoView?.({
          behavior: reduceMotion() ? "auto" : "smooth",
          block: "nearest",
          inline: "nearest",
        });
    });
    const timeout = window.setTimeout(() => setHighlightedId(null), 1800);
    return () => {
      if (frame !== undefined) window.cancelAnimationFrame?.(frame);
      window.clearTimeout(timeout);
    };
  }, [highlightedId]);

  return {
    railRef,
    highlightedId,
    capturePositions,
    cancelMove,
    showMovedVolume,
  };
}

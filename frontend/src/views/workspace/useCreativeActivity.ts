import { useEffect } from "react";
import { RecordCreativeActivity } from "@/lib/wailsjs/go/app/App";

const HEARTBEAT_MS = 60_000;
const IDLE_MS = 5 * 60_000;
const INTERACTION_EVENTS = ["keydown", "pointerdown", "pointermove", "wheel"];

export function useCreativeActivity() {
  useEffect(() => {
    const hasFocus = () =>
      typeof document.hasFocus !== "function" || document.hasFocus();
    let focused = hasFocus();
    let visible = document.visibilityState === "visible";
    let lastInteraction: number | null = null;
    let lastSample = Date.now();
    let pendingMs = 0;
    const foreground = () => visible && focused;

    function settle(now: number) {
      const gap = now - lastSample;
      if (
        foreground() &&
        lastInteraction !== null &&
        gap > 0 &&
        gap < IDLE_MS
      ) {
        const activeEnd = Math.min(now, lastInteraction + IDLE_MS);
        pendingMs += Math.min(
          HEARTBEAT_MS,
          Math.max(0, activeEnd - lastSample),
        );
      }
      lastSample = now;
    }

    function onInteraction() {
      if (!foreground()) return;
      const now = Date.now();
      settle(now);
      lastInteraction = now;
    }

    function onVisibilityChange() {
      settle(Date.now());
      visible = document.visibilityState === "visible";
      focused = hasFocus();
    }

    function onFocus() {
      settle(Date.now());
      focused = true;
    }

    function onBlur() {
      settle(Date.now());
      focused = false;
    }

    const options = { capture: true, passive: true };
    for (const event of INTERACTION_EVENTS) {
      document.addEventListener(event, onInteraction, options);
    }
    document.addEventListener("visibilitychange", onVisibilityChange);
    window.addEventListener("focus", onFocus);
    window.addEventListener("blur", onBlur);

    const timer = window.setInterval(async () => {
      settle(Date.now());
      const seconds = Math.min(60, Math.floor(pendingMs / 1000));
      pendingMs %= 1000;
      if (seconds <= 0) return;
      try {
        await RecordCreativeActivity(seconds);
      } catch {
        // 近似统计允许漏记，不重试或打断创作。
      }
    }, HEARTBEAT_MS);

    return () => {
      window.clearInterval(timer);
      for (const event of INTERACTION_EVENTS) {
        document.removeEventListener(event, onInteraction, options);
      }
      document.removeEventListener("visibilitychange", onVisibilityChange);
      window.removeEventListener("focus", onFocus);
      window.removeEventListener("blur", onBlur);
    };
  }, []);
}

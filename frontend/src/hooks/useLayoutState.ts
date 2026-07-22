import { useState, useCallback, useEffect, useRef } from "react";

const SIDEPANEL_KEY = "goink_sidepanel_width";
const CHATPANEL_KEY = "goink_chatpanel_width";

const SIDEPANEL_DEFAULT = 224;
const CHATPANEL_DEFAULT = 360;

function loadNumber(
  key: string,
  fallback: number,
  min: number,
  max: number,
): number {
  try {
    const raw = localStorage.getItem(key);
    if (raw !== null) {
      const v = parseInt(raw, 10);
      if (!isNaN(v) && v >= min && v <= max) return v;
    }
  } catch {
    /* ignore */
  }
  return fallback;
}

export function useLayoutState() {
  const [sidePanelWidth, setSidePanelWidthRaw] = useState(() =>
    loadNumber(SIDEPANEL_KEY, SIDEPANEL_DEFAULT, 180, 480),
  );
  const [chatPanelWidth, setChatPanelWidthRaw] = useState(() =>
    loadNumber(CHATPANEL_KEY, CHATPANEL_DEFAULT, 280, 800),
  );

  // 防抖写入 localStorage，避免拖拽时频繁同步 I/O
  const sideTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const chatTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    if (sideTimerRef.current) clearTimeout(sideTimerRef.current);
    sideTimerRef.current = setTimeout(() => {
      localStorage.setItem(SIDEPANEL_KEY, String(sidePanelWidth));
    }, 300);
    return () => {
      if (sideTimerRef.current) clearTimeout(sideTimerRef.current);
    };
  }, [sidePanelWidth]);

  useEffect(() => {
    if (chatTimerRef.current) clearTimeout(chatTimerRef.current);
    chatTimerRef.current = setTimeout(() => {
      localStorage.setItem(CHATPANEL_KEY, String(chatPanelWidth));
    }, 300);
    return () => {
      if (chatTimerRef.current) clearTimeout(chatTimerRef.current);
    };
  }, [chatPanelWidth]);

  const setSidePanelWidth = useCallback((w: number) => {
    setSidePanelWidthRaw(Math.min(480, Math.max(180, Math.round(w))));
  }, []);

  const setChatPanelWidth = useCallback((w: number) => {
    setChatPanelWidthRaw(Math.min(800, Math.max(280, Math.round(w))));
  }, []);

  return {
    sidePanelWidth,
    chatPanelWidth,
    setSidePanelWidth,
    setChatPanelWidth,
  };
}

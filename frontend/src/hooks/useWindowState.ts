import { useState, useEffect } from "react";
import {
  WindowToggleMaximise,
  WindowIsMaximised,
  WindowGetSize,
  WindowGetPosition,
  WindowSetSize,
  WindowSetPosition,
} from "@/lib/wailsjs/runtime/runtime";

const W = "goink_window_width";
const H = "goink_window_height";
const X = "goink_window_x";
const Y = "goink_window_y";
const M = "goink_window_maximised";

export function useWindowState() {
  const [isMaximised, setIsMaximised] = useState(false);

  useEffect(() => {
    async function restore() {
      const maximised = localStorage.getItem(M) === "1";
      if (maximised) {
        const isCurrentlyMaximised = await WindowIsMaximised();
        if (!isCurrentlyMaximised) WindowToggleMaximise();
      }
      setIsMaximised(maximised);

      const sw = parseInt(localStorage.getItem(W) || "", 10);
      const sh = parseInt(localStorage.getItem(H) || "", 10);
      const sx = parseInt(localStorage.getItem(X) || "", 10);
      const sy = parseInt(localStorage.getItem(Y) || "", 10);
      if (isNaN(sw) || isNaN(sh) || isNaN(sx) || isNaN(sy)) return;

      const availW = window.screen.availWidth;
      const availH = window.screen.availHeight;
      const rx = Math.max(-sw + 100, Math.min(sx, availW - 100));
      const ry = Math.max(-sh + 100, Math.min(sy, availH - 100));

      if (!maximised) {
        WindowSetSize(sw, sh);
        WindowSetPosition(rx, ry);
      } else {
        localStorage.setItem(W, String(sw));
        localStorage.setItem(H, String(sh));
        localStorage.setItem(X, String(rx));
        localStorage.setItem(Y, String(ry));
      }
    }

    restore();

    let lastSaved = "";
    function save() {
      Promise.all([WindowGetSize(), WindowGetPosition(), WindowIsMaximised()])
        .then(([size, pos, max]) => {
          const payload = max
            ? "M1"
            : `${size.w},${size.h},${pos.x},${pos.y}`;
          if (payload === lastSaved) return;
          lastSaved = payload;
          if (max) {
            localStorage.setItem(M, "1");
          } else {
            localStorage.removeItem(M);
            localStorage.setItem(W, String(size.w));
            localStorage.setItem(H, String(size.h));
            localStorage.setItem(X, String(pos.x));
            localStorage.setItem(Y, String(pos.y));
          }
        })
        .catch(() => {});
    }

    // 不依赖 beforeunload 保存：WebView2 关窗时 beforeunload 里的异步 IPC 来不及
    // 完成，生产版 localStorage 中 goink_window_* 从未写入（tabs/面板宽度因
    // debounce 定期写盘而正常）。改为事件驱动 + 定期轮询持续落盘：
    //   - resize 事件：缩放/最大化/还原（防抖 300ms）
    //   - 5s 轮询：窗口拖动（无 resize 事件）时兜底
    //   - beforeunload：仅作最后兜底
    let timer: ReturnType<typeof setTimeout> | null = null;
    function debouncedSave() {
      if (timer) clearTimeout(timer);
      timer = setTimeout(() => {
        timer = null;
        save();
      }, 300);
    }

    window.addEventListener("resize", debouncedSave);
    window.addEventListener("beforeunload", save);
    const interval = setInterval(save, 5000);
    save(); // 基线

    return () => {
      window.removeEventListener("resize", debouncedSave);
      window.removeEventListener("beforeunload", save);
      clearInterval(interval);
      if (timer) clearTimeout(timer);
    };
  }, []);

  return { isMaximised, setIsMaximised };
}

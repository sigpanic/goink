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

    function save() {
      Promise.all([WindowGetSize(), WindowGetPosition(), WindowIsMaximised()])
        .then(([size, pos, max]) => {
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

    window.addEventListener("beforeunload", save);
    return () => window.removeEventListener("beforeunload", save);
  }, []);

  return { isMaximised, setIsMaximised };
}

import { useState, useEffect, useCallback } from "react";

const ATTR = "data-theme";

const THEMES = ["light", "dark"] as const;
export type Theme = (typeof THEMES)[number];

function isTheme(s: string | null): s is Theme {
  return THEMES.includes(s as Theme);
}

const NEXT: Record<Theme, Theme> = { light: "dark", dark: "light" };

function sysTheme(matches: boolean): Theme {
  if (matches) return "dark";
  return "light";
}

function resolveTheme(): Theme {
  const stored = localStorage.getItem("theme");
  if (isTheme(stored)) return stored;
  return sysTheme(window.matchMedia("(prefers-color-scheme: dark)").matches);
}

function applyTheme(t: Theme) {
  document.documentElement.setAttribute(ATTR, t);
}

export function useTheme() {
  const [theme, setThemeState] = useState<Theme>(() => {
    const t = resolveTheme();
    applyTheme(t);
    return t;
  });

  // 跨组件同步：任一组件 toggle → DOM 属性变化 → 所有监听者更新
  useEffect(() => {
    const observer = new MutationObserver(() => {
      const v = document.documentElement.getAttribute(ATTR);
      if (isTheme(v)) setThemeState(v);
    });
    observer.observe(document.documentElement, {
      attributes: true,
      attributeFilter: [ATTR],
    });
    return () => observer.disconnect();
  }, []);

  useEffect(() => {
    const mq = window.matchMedia("(prefers-color-scheme: dark)");
    const onChange = () => {
      if (localStorage.getItem("theme") === null) {
        const t = sysTheme(mq.matches);
        applyTheme(t);
        setThemeState(t);
      }
    };
    mq.addEventListener("change", onChange);
    return () => mq.removeEventListener("change", onChange);
  }, []);

  const setTheme = useCallback((t: Theme) => {
    applyTheme(t);
    localStorage.setItem("theme", t);
    setThemeState(t);
  }, []);

  const toggle = useCallback(() => {
    setThemeState((prev) => {
      const next = NEXT[prev];
      applyTheme(next);
      localStorage.setItem("theme", next);
      return next;
    });
  }, []);

  return { theme, setTheme, toggle } as const;
}

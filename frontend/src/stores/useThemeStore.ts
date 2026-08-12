import { create } from "zustand";
import { persist, createJSONStorage } from "zustand/middleware";

const ATTR = "data-theme";

const THEMES = ["light", "dark"] as const;
export type Theme = (typeof THEMES)[number];

const NEXT: Record<Theme, Theme> = { light: "dark", dark: "light" };

function isTheme(s: unknown): s is Theme {
  return typeof s === "string" && THEMES.includes(s as Theme);
}

function applyTheme(t: Theme) {
  document.documentElement.setAttribute(ATTR, t);
}

interface ThemeState {
  theme: Theme;
  setTheme: (t: Theme) => void;
  toggle: () => void;
}

// 7.1：useTheme 迁移到 zustand store + persist。原 MutationObserver 跨组件同步 hack 删除
// （store 单一数据源天然跨组件同步）。原 localStorage 手写读写由 persist 中间件接管。
// 行为变更：首次启动不再跟随系统主题（原 localStorage.getItem("theme") === null fallback 删除），
// 默认 light；用户选过后持久化保持。接受此行为变更以换取 store 简化。
export const useThemeStore = create<ThemeState>()(
  persist(
    (set, get) => ({
      theme: "light",
      setTheme: (t) => {
        applyTheme(t);
        set({ theme: t });
      },
      toggle: () => {
        const next = NEXT[get().theme];
        applyTheme(next);
        set({ theme: next });
      },
    }),
    {
      name: "goink-theme",
      storage: createJSONStorage(() => localStorage),
      partialize: (s) => ({ theme: s.theme }),
      onRehydrateStorage: () => (state) => {
        if (state && isTheme(state.theme)) {
          applyTheme(state.theme);
        }
      },
    },
  ),
);

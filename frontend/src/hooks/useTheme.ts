// 7.1 过渡壳：useTheme 已迁移到 useThemeStore + persist。
// 此文件仅 re-export 兼容任何遗漏的 import，7.4 阶段删除本文件。
export { useThemeStore as useTheme, type Theme } from "@/stores/useThemeStore";

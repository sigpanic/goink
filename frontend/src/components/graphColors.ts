import { useTheme } from "@/hooks/useTheme";

const C = {
  light: {
    bg: "#fafbfc",
    edge: "#3b82f6",
    edgeDim: "#cbd5e1",
    dimFill: "#f1f5f9",
    dimStroke: "#cbd5e1",
    dimText: "#94a3b8",
    card: "#ffffff",
    softBg: "#f8fafc",
    hardText: "#475569",
  },
  dark: {
    bg: "#161b22",
    edge: "#58a6ff",
    edgeDim: "#30363d",
    dimFill: "#21262d",
    dimStroke: "#30363d",
    dimText: "#8b949e",
    card: "#1c2128",
    softBg: "#161b22",
    hardText: "#c9d1d9",
  },
} as const;

export function useGraphColors() {
  const { theme } = useTheme();
  return C[theme];
}

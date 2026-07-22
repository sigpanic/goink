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
    nodeFill: "#e0f2fe",
    nodeStroke: "#38a8df",
    nodeText: "#0c4a6e",
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
    nodeFill: "#1c3a5e",
    nodeStroke: "#58a6ff",
    nodeText: "#c9d1d9",
  },
} as const;

export function useGraphColors() {
  const { theme } = useTheme();
  return C[theme];
}

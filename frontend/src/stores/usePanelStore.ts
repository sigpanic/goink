import { create } from "zustand";
import type { PanelId, SidebarPanelId } from "@/types/panel";

interface PanelState {
  activePanel: PanelId;
  sidebarPanel: SidebarPanelId | null;
  sidebarClosed: boolean;
  setActivePanel: (panel: PanelId) => void;
  setSidebarPanel: (panel: SidebarPanelId | null) => void;
  setSidebarClosed: (closed: boolean) => void;
}

// activePanel 默认 "novels"；WorkspaceView mount 时用 initialNovelId 覆盖为 "chapters"。
// handleActivitySelect 的折叠/展开逻辑是 condition-based，由调用方用 3 个 setter 组合实现，
// 不提供 toggleSidebar（无简单 toggle 语义）。
export const usePanelStore = create<PanelState>((set) => ({
  activePanel: "novels",
  sidebarPanel: null,
  sidebarClosed: false,
  setActivePanel: (panel) => set({ activePanel: panel }),
  setSidebarPanel: (panel) => set({ sidebarPanel: panel }),
  setSidebarClosed: (closed) => set({ sidebarClosed: closed }),
}));

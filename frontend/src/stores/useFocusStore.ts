import { create } from "zustand";
import type { PanelId } from "@/types/panel";

interface FocusEntry {
  id: number;
  nonce: number;
}

interface FocusState {
  focusMap: Partial<Record<PanelId, FocusEntry>>;
  focusEntity: (panelId: PanelId, id: number) => void;
  clearFocus: (panelId: PanelId) => void;
}

// focusMap 整体替换（非单 key merge），等价 2.6 的 setFocusMap 语义：
// 搜索导航时清掉其他面板的旧 focusId，只保留当前面板的。
// focusEntity 写入时带 nonce，让「重新点同一条」也触发 useEffect（id 不变 nonce 变）。
// clearFocus 在 View unmount 时清自己（由 useFocusWithNonce hook 注册 cleanup），
// 避免切走再切回 remount 时 focusId 残留触发不期望的重新定位。
export const useFocusStore = create<FocusState>((set) => ({
  focusMap: {},
  focusEntity: (panelId, id) =>
    set({
      focusMap: {
        [panelId]: { id, nonce: Date.now() },
      } as Partial<Record<PanelId, FocusEntry>>,
    }),
  clearFocus: (panelId) =>
    set((s) => {
      const next = { ...s.focusMap };
      delete next[panelId];
      return { focusMap: next };
    }),
}));

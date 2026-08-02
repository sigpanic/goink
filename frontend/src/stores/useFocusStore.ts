import { create } from "zustand";
import type { PanelId } from "@/types/panel";

interface FocusState {
  focusMap: Partial<Record<PanelId, number>>;
  focusEntity: (panelId: PanelId, id: number) => void;
}

// focusMap 整体替换（非单 key merge），等价 2.6 的 setFocusMap 语义：
// 搜索导航时清掉其他面板的旧 focusId，只保留当前面板的。
// 无 clear()——无调用场景（YAGNI）。
export const useFocusStore = create<FocusState>((set) => ({
  focusMap: {},
  focusEntity: (panelId, id) =>
    set({ focusMap: { [panelId]: id } as Partial<Record<PanelId, number>> }),
}));

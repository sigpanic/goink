import { create } from "zustand";

// useLocationStore: location 领域 UI 状态（参考 useCharacterStore）。
// 当前只放 deletingLocationId —— 删除合并用：LocationList（侧边栏）和 LocationListView
// （主区）共用唯一 ConfirmDialog + 执行入口（LocationListView），通过 store 共享删除目标。
// 组件内部自用状态（editMode/form/viewTab 等）留组件内，不进 store。
interface LocationStoreState {
  deletingLocationId: number | null;
  setDeletingLocationId: (id: number | null) => void;
}

export const useLocationStore = create<LocationStoreState>((set) => ({
  deletingLocationId: null,
  setDeletingLocationId: (id) => set({ deletingLocationId: id }),
}));

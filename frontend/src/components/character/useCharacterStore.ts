import { create } from "zustand";

// useCharacterStore: 角色领域跨组件共享的删除目标状态。
// 仅放 deletingCharacterId —— 主区 CharacterListView（挂唯一 ConfirmDialog + 执行删除）
// 与侧边栏 CharacterList（点删除只 dispatch）共享，合并成单一删除确认流程。
// editMode/form/viewTab/search 等组件内自用状态不进 store（README 总原则 10）。
// 数据走 useCharacters query；create/update/delete 走 mutation（4.1.2）。
interface CharacterUIState {
  deletingCharacterId: number | null;
  setDeletingCharacterId: (id: number | null) => void;
}

export const useCharacterStore = create<CharacterUIState>((set) => ({
  deletingCharacterId: null,
  setDeletingCharacterId: (id) => set({ deletingCharacterId: id }),
}));

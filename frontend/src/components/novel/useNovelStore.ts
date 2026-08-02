import { create } from "zustand";
import type { novel } from "@/lib/wailsjs/go/models";

// useNovelStore: 小说领域 UI 状态（activeNovelId + 4 个对话框开关）。
// 数据走 useNovels query；CRUD 走 mutation（3.3-3.5）；本 store 只管 UI 协调状态。
// switchToNovel action 3.7 才迁入；本版仅暴露 state + setter。
//
// 不放 src/stores/ 的原因：领域聚合（00-conventions §4）——小说领域专属状态
// 与 useNovels/NovelDialogs 同目录。src/stores/ 只放跨领域应用级 store（panel/focus/tab）。
interface NovelUIState {
  activeNovelId: number;
  editingNovel: novel.Novel | null;
  deletingNovel: novel.Novel | null;
  showCreateDialog: boolean;
  exportNovelId: number | null;
  setActiveNovelId: (id: number) => void;
  setEditingNovel: (n: novel.Novel | null) => void;
  setDeletingNovel: (n: novel.Novel | null) => void;
  setShowCreateDialog: (b: boolean) => void;
  setExportNovelId: (id: number | null) => void;
}

export const useNovelStore = create<NovelUIState>((set) => ({
  activeNovelId: 0,
  editingNovel: null,
  deletingNovel: null,
  showCreateDialog: false,
  exportNovelId: null,
  setActiveNovelId: (id) => set({ activeNovelId: id }),
  setEditingNovel: (n) => set({ editingNovel: n }),
  setDeletingNovel: (n) => set({ deletingNovel: n }),
  setShowCreateDialog: (b) => set({ showCreateDialog: b }),
  setExportNovelId: (id) => set({ exportNovelId: id }),
}));

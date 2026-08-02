import { create } from "zustand";
import type { novel } from "@/lib/wailsjs/go/models";
import { SetActiveNovel } from "@/lib/wailsjs/go/app/App";

// useNovelStore: 小说领域 UI 状态（activeNovelId + 4 个对话框开关）+ switchNovel action。
// 数据走 useNovels query；CRUD 走 mutation（3.3-3.5）；本 store 管 UI 协调状态 + 切小说 action。
//
// switchNovel action（3.7）：只管 novel state + 后端调用（SetActiveNovel）。
// ContentPanel 的 tab 切换由 useEditorTabs 内部订阅 novelId 自动处理（不在此 action）。
// tabTarget/activeContent/selectedGitFile 重置留 WorkspaceView 瘦 wrapper（归 SidePanel/StatusBar/GitCommitView）。
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
  // 切小说：set activeNovelId + 通知后端。ContentPanel/tab 重置不归此 action。
  switchNovel: (id: number) => Promise<void>;
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
  switchNovel: async (id) => {
    set({ activeNovelId: id });
    await SetActiveNovel({ novel_id: id });
  },
}));

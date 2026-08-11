import { create } from "zustand";
import type { git } from "@/lib/wailsjs/go/models";

// useGitStore: git 领域内部跨组件 UI 状态——选中的 diff 文件。
// 写方 GitHistoryList（侧栏文件列表点选）、读方 GitCommitView（主区 diff 渲染）。
// 两者都在 git 领域，按「领域 store 放领域目录」原则放此（同 useNovelStore 模式）。
// reset() 由 WorkspaceView 监听 useNovelStore.activeNovelId 变化时自动调用——
// 切小说后清旧小说的 diff 残留，等价原 switchToNovel wrapper 内的 setSelectedGitFile(null)。
interface GitState {
  selectedGitFile: git.FileDiff | null;
  setSelectedGitFile: (file: git.FileDiff | null) => void;
  reset: () => void;
}

export const useGitStore = create<GitState>((set) => ({
  selectedGitFile: null,
  setSelectedGitFile: (file) => set({ selectedGitFile: file }),
  reset: () => set({ selectedGitFile: null }),
}));

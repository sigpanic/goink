import { create } from "zustand";

// useEditorStore: 编辑器/内容选择相关跨领域 UI 状态。
// - tabTarget：SidePanel 选中章节/goink 的高亮指示（ChapterList 消费）
// - activeContent：ContentPanel 当前 tab 内容（StatusBar 字数统计消费）
// - isDirty：ContentPanel 当前 tab dirty 状态（StatusBar 保存状态点消费）
//
// 3 个 state 都跨组件共享（写方与读方分属不同领域组件），故放 src/stores/。
// reset() 由 WorkspaceView 监听 useNovelStore.activeNovelId 变化时自动调用——
// 切小说后清旧小说的高亮/字数/dirty 残留，等价原 switchToNovel wrapper 内的 3 行 setState。
interface EditorState {
  tabTarget: { path: string; title: string } | null;
  activeContent: string;
  isDirty: boolean;
  setTabTarget: (target: { path: string; title: string } | null) => void;
  setActiveContent: (content: string) => void;
  setIsDirty: (dirty: boolean) => void;
  reset: () => void;
}

export const useEditorStore = create<EditorState>((set) => ({
  tabTarget: null,
  activeContent: "",
  isDirty: false,
  setTabTarget: (target) => set({ tabTarget: target }),
  setActiveContent: (content) => set({ activeContent: content }),
  setIsDirty: (dirty) => set({ isDirty: dirty }),
  reset: () => set({ tabTarget: null, activeContent: "", isDirty: false }),
}));

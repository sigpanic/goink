import { create } from "zustand";

interface SearchState {
  query: string;
  setQuery: (q: string) => void;
}

// 5.5 commit 2：搜索词外置 store，消除 WorkspaceView→SidePanel→SearchPanel 透传链路。
// store 全局持有，SearchPanel unmount 后切回仍保留搜索词（规则 7 UI/UX 不变）。
// 搜索结果仍在 useQuery 缓存（commit 1 useSearch），本 store 只持 query。
export const useSearchStore = create<SearchState>((set) => ({
  query: "",
  setQuery: (q) => set({ query: q }),
}));

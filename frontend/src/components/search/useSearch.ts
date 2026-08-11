import { useQuery } from "@tanstack/react-query";
import { SearchAll } from "@/lib/wailsjs/go/app/App";
import { searchKeys } from "@/lib/queryKeys";
import type { search } from "@/lib/wailsjs/go/models";

// useSearch: 全局跨实体搜索 query（SearchAll 后端统一入口：跨实体 + 正文 + RAG）。
// queryKey 含 query 字符串驱动 refetch + 让 query 内置竞态保护接管（替代旧 reqIdRef）。
// enabled 守卫：novelId=0 或空 query 不 fetch。
// staleTime=0（搜索是用户主动期望最新结果的操作，输入新词必 refetch）。
// refetchOnMount=false + refetchOnWindowFocus=false：切走搜索面板再切回用缓存不 refetch，
//   匹配旧实现 searchResults 在 WorkspaceView holder 永久保留、切回直接显示不 spinner 的行为。
// gcTime=10min：覆盖正常切面板场景，避免切回时缓存已 GC 触发 spinner。
// 消费方：SearchPanel；GET 错误由全局中间件接管（queryErrorToast.ts），组件不挂 toastError。
// 5.5 commit 1。
export type SearchResult = search.Result;

export function useSearch(input: { novelId: number; query: string }) {
  return useQuery({
    queryKey: searchKeys.list(input.novelId, input.query),
    queryFn: async () => {
      const r = (await SearchAll(
        input.novelId,
        input.query.trim(),
      )) as unknown as SearchResult[];
      return r ?? [];
    },
    enabled: !!input.novelId && !!input.query.trim(),
    staleTime: 0,
    gcTime: 600_000,
    refetchOnMount: false,
    refetchOnWindowFocus: false,
  });
}

import { useInfiniteQuery } from "@tanstack/react-query";
import { GetSessions } from "@/lib/wailsjs/go/app/App";
import { sessionKeys } from "@/lib/queryKeys";

// useInfiniteSessions: 会话列表无限滚动 query（SessionHistory size=20 + search）。
// page 由 useInfiniteQuery 的 pageParam 管理（不进 queryKey）；search 变化触发新 query。
// data.pages 是已加载各页的 PageResult 数组，消费者 flatMap(p => p.items) 得到累积列表。
// getNextPageParam: 当前页 < 总页数时返回下一页 pageParam，否则 undefined（无更多页）。
// enabled: open && !!novelId（面板关闭时不 fetch）。
export function useInfiniteSessions(input: {
  novelId: number;
  size: number;
  search: string;
  enabled: boolean;
}) {
  return useInfiniteQuery({
    queryKey: sessionKeys.infiniteList(
      input.novelId,
      input.size,
      input.search,
    ),
    queryFn: async ({ pageParam }) => {
      const r = await GetSessions({
        novel_id: input.novelId,
        page: pageParam,
        size: input.size,
        search: input.search,
      });
      return r;
    },
    initialPageParam: 1,
    getNextPageParam: (lastPage) =>
      lastPage.page < lastPage.total_pages ? lastPage.page + 1 : undefined,
    enabled: input.enabled && !!input.novelId,
  });
}

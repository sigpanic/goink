import { useInfiniteQuery } from "@tanstack/react-query";
import { ListStyleSamples } from "@/lib/wailsjs/go/app/App";
import { styleSampleKeys } from "@/lib/queryKeys";

// useInfiniteStyleSamples: 风格样本无限滚动 query（StyleSampleList size=50 + search）。
// page 由 useInfiniteQuery 的 pageParam 管理（不进 queryKey）；search 变化触发新 query。
// data.pages 是已加载各页的 PageResult 数组，消费者 flatMap(p => p.items) 得到累积列表。
// getNextPageParam: 当前页 < 总页数时返回下一页 pageParam，否则 undefined（无更多页）。
// 不设 enabled 守卫：novel_id=0 是合法的全局 samples 查询（is_global），始终 fetch。对齐 useInfiniteSessions 模式。
// 5.3 commit 1：GET 错误由全局中间件接管（queryErrorToast.ts），组件不挂 toastError。
export function useInfiniteStyleSamples(input: {
  novelId: number;
  size: number;
  search: string;
}) {
  return useInfiniteQuery({
    queryKey: styleSampleKeys.infiniteList(
      input.novelId,
      input.size,
      input.search,
    ),
    queryFn: async ({ pageParam }) => {
      const r = await ListStyleSamples({
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
  });
}

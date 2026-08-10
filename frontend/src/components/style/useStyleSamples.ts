import { useQuery } from "@tanstack/react-query";
import { ListStyleSamples } from "@/lib/wailsjs/go/app/App";
import { styleSampleKeys } from "@/lib/queryKeys";

// useStyleSamples: 风格样本单页列表 query（StyleView 上一页/下一页分页，PAGE_SIZE=15）。
// queryKey 含 page/size/search，与 useInfiniteStyleSamples 的无限滚动缓存区分。
// StyleSampleList 的无限滚动走 useInfiniteStyleSamples（page 由 pageParam 管理）。
// 不设 enabled 守卫：novel_id=0 是合法的全局 samples 查询（is_global），始终 fetch。
// 消费方：StyleView；CRUD 后由 mutation 的 invalidateQueries 同步（commit 2 抽 mutation）。
// 5.3 commit 1：GET 错误由全局中间件接管（queryErrorToast.ts），组件不挂 toastError。
export function useStyleSamples(input: {
  novelId: number;
  page: number;
  size: number;
  search: string;
}) {
  return useQuery({
    queryKey: styleSampleKeys.list(
      input.novelId,
      input.page,
      input.size,
      input.search,
    ),
    queryFn: async () => {
      const r = await ListStyleSamples({
        novel_id: input.novelId,
        page: input.page,
        size: input.size,
        search: input.search,
      });
      return r;
    },
  });
}

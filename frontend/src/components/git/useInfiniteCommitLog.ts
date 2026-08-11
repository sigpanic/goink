import { useInfiniteQuery } from "@tanstack/react-query";
import { GetCommitLog } from "@/lib/wailsjs/go/app/App";
import { gitCommitKeys } from "@/lib/queryKeys";

// useInfiniteCommitLog: git 提交历史无限滚动 query（GitHistoryList size=50）。
//
// 与 useInfiniteSessions/useInfiniteStyleSamples 不同，git 走**游标分页**（afterHash）
// 而非 page 分页——git log 没有页码/total_pages 概念，提交历史是链表结构，
// 只能以 hash 为游标拉下一页（git log --after=<hash> 的天然分页方式）。
//
// - initialPageParam="" 表示从 HEAD 开始拉
// - pageParam 是 afterHash 字符串（不进 queryKey，由 useInfiniteQuery 管理）
// - getNextPageParam: 上一页满 size 时返回该页最后一条 commit 的 hash 作为下一页 cursor，
//   否则 undefined（无更多页）
// - data.pages 是已加载各页的 CommitInfo[] 数组，消费者 flatMap(p => p) 得到累积列表
//
// 5.4 commit 5：GET 错误由全局中间件接管（queryErrorToast.ts），组件不挂 toastError。
export function useInfiniteCommitLog(input: {
  novelId: number;
  size: number;
  enabled: boolean;
}) {
  return useInfiniteQuery({
    queryKey: gitCommitKeys.infiniteList(input.novelId, input.size),
    queryFn: async ({ pageParam }) => {
      const list = await GetCommitLog(
        input.novelId,
        input.size,
        pageParam ?? "",
      );
      return list ?? [];
    },
    initialPageParam: "",
    getNextPageParam: (lastPage) =>
      lastPage.length >= input.size
        ? lastPage[lastPage.length - 1].hash
        : undefined,
    enabled: input.enabled && !!input.novelId,
  });
}

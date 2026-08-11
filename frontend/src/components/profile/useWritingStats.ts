import { useQuery } from "@tanstack/react-query";
import { GetWritingStats } from "@/lib/wailsjs/go/app/App";
import { writingStatsKeys } from "@/lib/queryKeys";
import type { writing } from "@/lib/wailsjs/go/models";

// useWritingStats: 全局写作统计 query（累计字数/写作天数/连续天数/作品数/章节数）。
// queryFn 直接 import wailsjs GetWritingStats（不用 useApp）。
// GET 错误由全局中间件接管（queryErrorToast.ts），组件不挂 toastError。
// 5.7 commit 1。
export type WritingStats = writing.WritingStats;

export function useWritingStats() {
  return useQuery({
    queryKey: writingStatsKeys.all,
    queryFn: async () => {
      return (await GetWritingStats()) as WritingStats;
    },
  });
}

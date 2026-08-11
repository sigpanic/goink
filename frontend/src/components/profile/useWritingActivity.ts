import { useQuery } from "@tanstack/react-query";
import { GetWritingActivity } from "@/lib/wailsjs/go/app/App";
import { writingActivityKeys } from "@/lib/queryKeys";
import type { writing } from "@/lib/wailsjs/go/models";

// useWritingActivity: 过去 N 个月写作日历 query（ProfileView 绿格子数据）。
// queryFn 直接 import wailsjs GetWritingActivity（不用 useApp）。
// 数据兜底返回 []（无写作记录时空数组）。
// GET 错误由全局中间件接管（queryErrorToast.ts），组件不挂 toastError。
// 5.7 commit 1。
export type WritingActivity = writing.DailyActivity;

export function useWritingActivity(months: number) {
  return useQuery({
    queryKey: writingActivityKeys.detail(months),
    queryFn: async () => {
      const r = (await GetWritingActivity(months)) as WritingActivity[];
      return r ?? [];
    },
  });
}

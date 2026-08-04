import { useQuery } from "@tanstack/react-query";
import { GetChapterPlans } from "@/lib/wailsjs/go/app/App";
import { chapterPlanKeys } from "@/lib/queryKeys";

// useChapterPlans: 章节计划 3-slot（next/near/far）query。
// GetChapterPlans(novelId) 独立 API，与 useTimelineEntries 并列（各自失效互不影响）。
// 仅 TimelineView 消费（planTab 显示）。
export function useChapterPlans(novelId: number) {
  return useQuery({
    queryKey: chapterPlanKeys.list(novelId),
    queryFn: async () => {
      const list = await GetChapterPlans(novelId);
      return list ?? [];
    },
    enabled: !!novelId,
  });
}

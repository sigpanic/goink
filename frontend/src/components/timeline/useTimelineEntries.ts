import { useQuery } from "@tanstack/react-query";
import { GetTimelineEntries } from "@/lib/wailsjs/go/app/App";
import { timelineKeys } from "@/lib/queryKeys";

// useTimelineEntries: 时间线条目列表 query。
// 后端 GetTimelineEntries(novelId, fromChapter, toChapter) 第二三参数是章节窗口，
// 传 0,0 拿全量（同 arc-nodes 模式），queryKey 用 ["timeline", novelId] 全量缓存，
// invalidate 一次刷全部。TimelineView / TimelineList 共享缓存。
export function useTimelineEntries(novelId: number) {
  return useQuery({
    queryKey: timelineKeys.list(novelId),
    queryFn: async () => {
      const list = await GetTimelineEntries(novelId, 0, 0);
      return list ?? [];
    },
    enabled: !!novelId,
  });
}

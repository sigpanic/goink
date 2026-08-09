import { useQuery } from "@tanstack/react-query";
import { GetTimelineEntries } from "@/lib/wailsjs/go/app/App";
import { timelineKeys } from "@/lib/queryKeys";

// useTimelineEntries: 时间线条目列表 query。
// 后端 GetTimelineEntries(novelId) 拿全量（4b: 废弃 from/to 章节窗口，前端内存切窗口），
// queryKey 用 ["timeline", novelId] 全量缓存，invalidate 一次刷全部。
// TimelineView / TimelineList 共享缓存。
export function useTimelineEntries(novelId: number) {
  return useQuery({
    queryKey: timelineKeys.list(novelId),
    queryFn: async () => {
      const list = await GetTimelineEntries(novelId);
      return list ?? [];
    },
    enabled: !!novelId,
  });
}

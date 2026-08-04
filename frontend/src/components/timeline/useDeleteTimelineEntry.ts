import { useMutation, useQueryClient } from "@tanstack/react-query";
import { DeleteTimelineEntry } from "@/lib/wailsjs/go/app/App";
import { timelineKeys } from "@/lib/queryKeys";

// useDeleteTimelineEntry: 删除时间线条目 mutation。
// mutationFn 直接 import wailsjs DeleteTimelineEntry（不用 useApp）。
// 消费方：TimelineView.confirmDelete（mutateAsync 抛错由 handler try/catch 接住）。
// handler 负责 setDeleteTarget(null) + 错误 toast（副作用不放进 mutation）。
// onSuccess 失效 timeline：entry 删除后列表同步（TimelineView + TimelineList 共享缓存）。
// 不失效 chapter-plans：plan 是独立数据，entry 删除不影响 plan。
// 不失效 max-chapter：entry 删除不影响小说最大章节号。
export function useDeleteTimelineEntry(novelId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (entryId: number) => DeleteTimelineEntry(novelId, entryId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: timelineKeys.list(novelId) });
    },
  });
}

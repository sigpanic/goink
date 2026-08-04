import { useMutation, useQueryClient } from "@tanstack/react-query";
import { CreateTimelineEntry } from "@/lib/wailsjs/go/app/App";
import type { app } from "@/lib/wailsjs/go/models";
import { timelineKeys } from "@/lib/queryKeys";

// useCreateTimelineEntry: 创建时间线条目 mutation。
// mutationFn 直接 import wailsjs CreateTimelineEntry（不用 useApp），返回 timeline.TimelineEntry。
// handler 负责 setEditMode(null) + 错误 throw（副作用各异，不放进 mutation）。
// onSuccess 失效 timeline：新 entry 入列表，TimelineView / TimelineList 同步。
// 不失效 chapter-plans/max-chapter：新建 entry 不影响 plan 和小说最大章节号。
export function useCreateTimelineEntry(novelId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: app.CreateTimelineEntryInput) =>
      CreateTimelineEntry(novelId, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: timelineKeys.list(novelId) });
    },
  });
}

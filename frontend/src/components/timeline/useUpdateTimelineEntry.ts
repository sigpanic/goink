import { useMutation, useQueryClient } from "@tanstack/react-query";
import { UpdateTimelineEntry } from "@/lib/wailsjs/go/app/App";
import type { app } from "@/lib/wailsjs/go/models";
import { timelineKeys } from "@/lib/queryKeys";

// useUpdateTimelineEntry: 更新时间线条目 mutation。
// mutationFn 直接 import wailsjs UpdateTimelineEntry（不用 useApp）。
// 入参 {id, input}：input 用 app.UpdateTimelineEntryInput（全 optional，PATCH 语义），
// 但 handler 全量回传 input 所有字段（§6 等价 PUT），含 handleQuickStatus 全量回传。
// handler 负责 setEditMode(null) + 错误 throw（副作用各异，不放进 mutation）。
// onSuccess 失效 timeline：entry 字段变更入列表。
// 不失效 chapter-plans/max-chapter：entry 字段不影响 plan 和小说最大章节号。
export function useUpdateTimelineEntry(novelId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      id,
      input,
    }: {
      id: number;
      input: app.UpdateTimelineEntryInput;
    }) => UpdateTimelineEntry(novelId, id, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: timelineKeys.list(novelId) });
    },
  });
}

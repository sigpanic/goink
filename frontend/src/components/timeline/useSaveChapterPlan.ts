import { useMutation, useQueryClient } from "@tanstack/react-query";
import { UpdateChapterPlan } from "@/lib/wailsjs/go/app/App";
import type { app } from "@/lib/wailsjs/go/models";
import { chapterPlanKeys } from "@/lib/queryKeys";

// useSaveChapterPlan: 保存章节计划 mutation（plan 3-slot next/near/far）。
// mutationFn 直接 import wailsjs UpdateChapterPlan（不用 useApp）。
// 后端 UpdateChapterPlan 按 scope upsert（next/near/far 各一条），无独立 Create API。
// handler 负责 setEditMode(null) + 错误 throw（副作用不放进 mutation）。
// onSuccess 失效 chapter-plans：plan 内容变更后 plan 区刷新。
// 不失效 timeline/max-chapter：plan 不影响 entry 列表和小说最大章节号。
export function useSaveChapterPlan(novelId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: app.UpdateChapterPlanInput) =>
      UpdateChapterPlan(novelId, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: chapterPlanKeys.list(novelId) });
    },
  });
}

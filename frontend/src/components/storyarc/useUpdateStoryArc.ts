import { useMutation, useQueryClient } from "@tanstack/react-query";
import { UpdateStoryArc } from "@/lib/wailsjs/go/app/App";
import type { app } from "@/lib/wailsjs/go/models";
import { storyarcKeys } from "@/lib/queryKeys";

// useUpdateStoryArc: 更新故事弧线 mutation。
// mutationFn 直接 import wailsjs UpdateStoryArc（不用 useApp）。
// 入参 {id, input}：input 用 app.UpdateStoryArcInput（全 optional，PATCH 语义），
// 但 handler 全量回传 input 所有字段（§6 等价 PUT）。
// handler 负责 setEditMode(null) + 错误 throw（副作用各异，不放进 mutation）。
// onSuccess 失效 storyarcs：arc 名/类型/重要度/status 变更入列表。
// 不失效 arc-nodes：arc 字段不影响 node 缓存（node 只存 story_arc_id 引用）。
export function useUpdateStoryArc(novelId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      id,
      input,
    }: {
      id: number;
      input: app.UpdateStoryArcInput;
    }) => UpdateStoryArc(novelId, id, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: storyarcKeys.list(novelId) });
    },
  });
}

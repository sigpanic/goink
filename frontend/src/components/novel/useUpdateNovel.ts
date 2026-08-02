import { useMutation, useQueryClient } from "@tanstack/react-query";
import { UpdateNovel } from "@/lib/wailsjs/go/app/App";
import type { app } from "@/lib/wailsjs/go/models";
import { novelKeys } from "@/lib/queryKeys";

// useUpdateNovel: 更新小说 mutation。
// mutationFn 直接 import wailsjs UpdateNovel（不用 useApp），onSuccess 失效 novelKeys.all。
// 入参 {id, input}：input 全量回传所有字段（PUT 语义，见 00-conventions §6）。
// handler 负责 setEditingNovel(null) + 错误 throw（副作用各异，不放进 mutation）。
export function useUpdateNovel() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, input }: { id: number; input: app.UpdateNovelInput }) =>
      UpdateNovel(id, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: novelKeys.all });
    },
  });
}

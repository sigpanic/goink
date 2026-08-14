import { useMutation, useQueryClient } from "@tanstack/react-query";
import { UpdateArcNode } from "@/lib/wailsjs/go/app/App";
import type { app } from "@/lib/wailsjs/go/models";
import { arcNodeKeys } from "@/lib/queryKeys";

// useUpdateArcNode: 更新弧线节点 mutation。
// mutationFn 直接 import wailsjs UpdateArcNode（不用 useApp）。
// 入参 {id, input}：input 用 app.UpdateArcNodeInput（PUT 语义，全量回传）。
// handler 负责 setEditMode(null)/setExpandedId + 错误 throw（副作用各异，不放进 mutation）。
// onSuccess 失效 arc-nodes：node 字段变更入列表。
// 不失效 storyarcs：node 字段不影响 arc 列表。
export function useUpdateArcNode(novelId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      id,
      input,
    }: {
      id: number;
      input: app.UpdateArcNodeInput;
    }) => UpdateArcNode(novelId, id, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: arcNodeKeys.list(novelId) });
    },
  });
}

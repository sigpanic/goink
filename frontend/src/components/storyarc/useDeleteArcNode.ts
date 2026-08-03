import { useMutation, useQueryClient } from "@tanstack/react-query";
import { DeleteArcNode } from "@/lib/wailsjs/go/app/App";
import { arcNodeKeys } from "@/lib/queryKeys";

// useDeleteArcNode: 删除弧线节点 mutation。
// mutationFn 直接 import wailsjs DeleteArcNode（不用 useApp）。
// 消费方：ArcListView.confirmDelete（mutateAsync 抛错由 handler try/catch 接住）。
// handler 负责 setExpandedId(null) + setDeleteTarget(null) + 错误 toast（副作用不放进 mutation）。
// onSuccess 只失效 arc-nodes：删 node 不影响 arcs 列表（useStoryArcs）也不影响 max-chapter。
// ArcListView/StoryArcGraph 的 useArcNodes 都 refetch，缓存干净。
export function useDeleteArcNode(novelId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => DeleteArcNode(novelId, id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: arcNodeKeys.list(novelId) });
    },
  });
}

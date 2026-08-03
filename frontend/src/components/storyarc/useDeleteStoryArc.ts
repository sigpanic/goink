import { useMutation, useQueryClient } from "@tanstack/react-query";
import { DeleteStoryArc } from "@/lib/wailsjs/go/app/App";
import { storyarcKeys, arcNodeKeys } from "@/lib/queryKeys";

// useDeleteStoryArc: 删除叙事弧线 mutation。
// mutationFn 直接 import wailsjs DeleteStoryArc（不用 useApp）。
// 消费方：ArcListView.confirmDelete（mutateAsync 抛错由 handler try/catch 接住）。
// handler 负责 setExpandedId(null) + setDeleteTarget(null) + 错误 toast（副作用不放进 mutation）。
// onSuccess 失效 storyarcs + arc-nodes：后端 DeleteStoryArc 事务级联删该 arc 的所有 nodes，
// 前端两个 query 缓存都要刷新（ArcListView/ArcList 的 useStoryArcs + ArcListView/StoryArcGraph 的 useArcNodes 都 refetch，缓存干净）。
// 不失效 max-chapter：删 arc 不影响小说最大章节号。
export function useDeleteStoryArc(novelId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => DeleteStoryArc(novelId, id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: storyarcKeys.list(novelId) });
      qc.invalidateQueries({ queryKey: arcNodeKeys.list(novelId) });
    },
  });
}

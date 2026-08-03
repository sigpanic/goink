import { useMutation, useQueryClient } from "@tanstack/react-query";
import { DeleteLocation } from "@/lib/wailsjs/go/app/App";
import { locationKeys } from "@/lib/queryKeys";

// useDeleteLocation: 删除地点 mutation（参考 useDeleteCharacter）。
// mutationFn 直接 import wailsjs DeleteLocation（不用 useApp）。
// 两处共用：LocationListView 执行删除（confirmDelete 调 mutateAsync）；LocationList 只 dispatch store 不执行。
// handler 负责 setDeletingLocationId(null) + 错误 throw（副作用各异，不放进 mutation）。
// onSuccess 失效 list + relations：后端 DeleteLocation 事务级联删 LocationRelation
// （location_a 或 location_b = locID 的都删），前端两个 query 缓存都要刷新
// （LocationGraph 的 useLocationRelations 也 refetch，缓存干净）。
export function useDeleteLocation(novelId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => DeleteLocation(novelId, id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: locationKeys.list(novelId) });
      qc.invalidateQueries({ queryKey: locationKeys.relations(novelId) });
    },
  });
}

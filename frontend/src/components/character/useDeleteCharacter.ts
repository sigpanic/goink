import { useMutation, useQueryClient } from "@tanstack/react-query";
import { DeleteCharacter } from "@/lib/wailsjs/go/app/App";
import { characterKeys } from "@/lib/queryKeys";

// useDeleteCharacter: 删除角色 mutation。
// mutationFn 直接 import wailsjs DeleteCharacter（不用 useApp）。
// 两处共用：CharacterListView 执行删除（confirmDelete 调 mutateAsync）；CharacterList 只 dispatch store 不执行。
// handler 负责 setDeletingCharacterId(null) + 错误 throw（副作用各异，不放进 mutation）。
// onSuccess 失效 list + relations：后端 DeleteCharacter 事务级联删关系记录，
// 前端两个 query 缓存都要刷新（CharacterGraph 的 useCharacterRelations 也 refetch，缓存干净）。
export function useDeleteCharacter(novelId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => DeleteCharacter(novelId, id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: characterKeys.list(novelId) });
      qc.invalidateQueries({ queryKey: characterKeys.relations(novelId) });
    },
  });
}

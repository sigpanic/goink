import { useQuery } from "@tanstack/react-query";
import { GetCharacterRelations } from "@/lib/wailsjs/go/app/App";
import { characterKeys } from "@/lib/queryKeys";

// useCharacterRelations: 角色关系图 query（character.CharacterRelation[]）。
// enabled: !!novelId 守卫；不设 staleTime（默认 0，跟 useNovels 一致）。
// 消费方：CharacterGraph。CRUD 后由 mutation 的 invalidateQueries 同步（4.1.2 抽 mutation）。
export function useCharacterRelations(novelId: number) {
  return useQuery({
    queryKey: characterKeys.relations(novelId),
    queryFn: async () => {
      const list = await GetCharacterRelations(novelId);
      return list ?? [];
    },
    enabled: !!novelId,
  });
}

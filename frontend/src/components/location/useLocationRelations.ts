import { useQuery } from "@tanstack/react-query";
import { GetLocationRelations } from "@/lib/wailsjs/go/app/App";
import { locationKeys } from "@/lib/queryKeys";

// useLocationRelations: 地点空间关系 query（location.LocationRelation[]）。
// enabled: !!novelId 守卫；不设 staleTime（继承全局 30s，跟 useNovels/useCharacters 一致）。
// 消费方：LocationGraph。CRUD 后由 mutation 的 invalidateQueries 同步（4.2.2 抽 mutation）。
export function useLocationRelations(novelId: number) {
  return useQuery({
    queryKey: locationKeys.relations(novelId),
    queryFn: async () => {
      const list = await GetLocationRelations(novelId);
      return list ?? [];
    },
    enabled: !!novelId,
  });
}

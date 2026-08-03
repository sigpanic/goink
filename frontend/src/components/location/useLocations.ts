import { useQuery } from "@tanstack/react-query";
import { GetLocations } from "@/lib/wailsjs/go/app/App";
import { locationKeys } from "@/lib/queryKeys";

// useLocations: 地点列表 query。
// queryFn 直接 import wailsjs GetLocations（不用 useApp），不设 staleTime（继承全局 30s，跟 useNovels/useCharacters 一致）。
// enabled: !!novelId 守卫，novelId=0 时不 fetch（数据兜底空数组）。
// 消费方：LocationListView / LocationList / LocationGraph 共享缓存，
// CRUD 后由 mutation 的 invalidateQueries 同步（4.2.2 抽 mutation）。
export function useLocations(novelId: number) {
  return useQuery({
    queryKey: locationKeys.list(novelId),
    queryFn: async () => {
      const list = await GetLocations(novelId);
      return list ?? [];
    },
    enabled: !!novelId,
  });
}

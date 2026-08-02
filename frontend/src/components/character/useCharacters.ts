import { useQuery } from "@tanstack/react-query";
import { GetCharacters } from "@/lib/wailsjs/go/app/App";
import { characterKeys } from "@/lib/queryKeys";

// useCharacters: 角色列表 query。
// queryFn 直接 import wailsjs GetCharacters（不用 useApp），不设 staleTime（继承全局 30s，跟 useNovels 一致）。
// enabled: !!novelId 守卫，novelId=0 时不 fetch（数据兜底空数组）。
// 消费方：CharacterListView / CharacterGraph / CharacterList 共享缓存，
// CRUD 后由 mutation 的 invalidateQueries 同步（4.1.2 抽 mutation）。
export function useCharacters(novelId: number) {
  return useQuery({
    queryKey: characterKeys.list(novelId),
    queryFn: async () => {
      const list = await GetCharacters(novelId);
      return list ?? [];
    },
    enabled: !!novelId,
  });
}

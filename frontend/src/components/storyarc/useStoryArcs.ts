import { useQuery } from "@tanstack/react-query";
import { GetStoryArcs } from "@/lib/wailsjs/go/app/App";
import { storyarcKeys } from "@/lib/queryKeys";

// useStoryArcs: 叙事弧线列表 query。
// queryFn 直接 import wailsjs GetStoryArcs（不用 useApp），不设 staleTime（继承全局 30s，跟 useNovels/useCharacters 一致）。
// enabled: !!novelId 守卫，novelId=0 时不 fetch（数据兜底空数组）。
// 消费方：ArcList / ArcListView / StoryArcGraph 共享缓存，
// CRUD 后由 mutation 的 invalidateQueries 同步（commit 2/3 抽 mutation）。
export function useStoryArcs(novelId: number) {
  return useQuery({
    queryKey: storyarcKeys.list(novelId),
    queryFn: async () => {
      const list = await GetStoryArcs(novelId);
      return list ?? [];
    },
    enabled: !!novelId,
  });
}

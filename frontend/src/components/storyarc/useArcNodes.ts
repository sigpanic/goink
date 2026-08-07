import { useQuery } from "@tanstack/react-query";
import { GetArcNodes } from "@/lib/wailsjs/go/app/App";
import { arcNodeKeys } from "@/lib/queryKeys";

// useArcNodes: 弧线节点全量列表 query。
// 4b: GetArcNodes 签名改为单参数（废弃 fromChapter/toChapter，后端改调 ListNodesByNovel(Size=-1) 全量）。
// enabled: !!novelId 守卫，novelId=0 时不 fetch（数据兜底空数组）。
// 消费方：ArcListView / StoryArcGraph / ArcList 共享缓存，
// CRUD 后由 mutation 的 invalidateQueries 同步（commit 2/3 抽 mutation）。
export function useArcNodes(novelId: number) {
  return useQuery({
    queryKey: arcNodeKeys.list(novelId),
    queryFn: async () => {
      const list = await GetArcNodes(novelId);
      return list ?? [];
    },
    enabled: !!novelId,
  });
}

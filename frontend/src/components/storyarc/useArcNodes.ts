import { useQuery } from "@tanstack/react-query";
import { GetArcNodes } from "@/lib/wailsjs/go/app/App";
import { arcNodeKeys } from "@/lib/queryKeys";

// useArcNodes: 弧线节点全量列表 query。
// queryFn 直接 import wailsjs GetArcNodes(novelId, 0, 0)（不用 useApp）。
// 第二三参数 fromChapter/toChapter 传 0 = 不限章节窗口（全量），与改造前 load() 行为一致。
// enabled: !!novelId 守卫，novelId=0 时不 fetch（数据兜底空数组）。
// 消费方：ArcListView / StoryArcGraph 共享缓存，
// CRUD 后由 mutation 的 invalidateQueries 同步（commit 2/3 抽 mutation）。
export function useArcNodes(novelId: number) {
  return useQuery({
    queryKey: arcNodeKeys.list(novelId),
    queryFn: async () => {
      const list = await GetArcNodes(novelId, 0, 0);
      return list ?? [];
    },
    enabled: !!novelId,
  });
}

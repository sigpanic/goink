import { useQuery } from "@tanstack/react-query";
import { GetChapters } from "@/lib/wailsjs/go/app/App";
import { chapterKeys } from "@/lib/queryKeys";

// useChapters: 章节元数据列表 query。
// queryFn 直接 import wailsjs GetChapters（不用 useApp），不设 staleTime（继承全局 30s）。
// enabled: !!novelId 守卫，novelId=0 时不 fetch（数据兜底空数组）。
// 消费方：ChapterList（侧栏章节列表）。CRUD 后由 mutation 的 invalidateQueries 同步（5.2 commit 2）。
// file:changed 事件触发章节刷新由 commit 3 改 qc.invalidateQueries 接管。
export function useChapters(novelId: number) {
  return useQuery({
    queryKey: chapterKeys.list(novelId),
    queryFn: async () => {
      const list = await GetChapters(novelId);
      return list ?? [];
    },
    enabled: !!novelId,
  });
}

import { useQuery } from "@tanstack/react-query";
import { GetMaxChapterNumber } from "@/lib/wailsjs/go/app/App";
import { maxChapterKeys } from "@/lib/queryKeys";

// useMaxChapterNumber: 小说最大章节号 query（number）。
// 用于 storyarc 章节窗口中心 windowCenter 初始化（ArcListView / StoryArcGraph 共享）。
// queryFn 直接 import wailsjs GetMaxChapterNumber（不用 useApp）。
// enabled: !!novelId 守卫，novelId=0 时不 fetch（数据兜底 0）。
// CRUD 后由 mutation 的 invalidateQueries 同步（commit 2/3 抽 mutation）。
export function useMaxChapterNumber(novelId: number) {
  return useQuery({
    queryKey: maxChapterKeys.detail(novelId),
    queryFn: async () => {
      const max = await GetMaxChapterNumber(novelId);
      return max ?? 0;
    },
    enabled: !!novelId,
  });
}

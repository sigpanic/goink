import { useQuery } from "@tanstack/react-query";
import { GetReaderPerspectives } from "@/lib/wailsjs/go/app/App";
import { readerKeys } from "@/lib/queryKeys";

// useReaderPerspectives: 读者视角条目列表 query。
// 后端 GetReaderPerspectives(novelId) 单参 API（无章节窗口参数，区别于 timeline/storyarc）。
// queryKey 用 ["reader", novelId] 全量缓存，invalidate 一次刷全部。
// ReaderView / ReaderList 共享缓存（同 character/location List+View 模式）。
export function useReaderPerspectives(novelId: number) {
  return useQuery({
    queryKey: readerKeys.list(novelId),
    queryFn: async () => {
      const list = await GetReaderPerspectives(novelId);
      return list ?? [];
    },
    enabled: !!novelId,
  });
}

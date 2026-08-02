import { useQuery } from "@tanstack/react-query";
import { GetNovels } from "@/lib/wailsjs/go/app/App";
import { novelKeys } from "@/lib/queryKeys";

// useNovels: 全局小说列表 query。
// queryFn 直接 import wailsjs GetNovels（不用 useApp），30s staleTime 防 fetch 抖动。
// 返回值兜底空数组，消费方用 const { data: novels = [] } = useNovels()。
export function useNovels() {
  return useQuery({
    queryKey: novelKeys.all,
    queryFn: async () => {
      const list = await GetNovels();
      return list ?? [];
    },
  });
}

import { useQuery } from "@tanstack/react-query";
import { GetSessions } from "@/lib/wailsjs/go/app/App";
import { sessionKeys } from "@/lib/queryKeys";

// useSessions: 会话列表单页 query（ChatPanel 最近会话 page=1 size=5）。
// queryKey 含 page/size/search，与 SessionHistory 的 useInfiniteSessions 区分缓存。
// SessionHistory 的无限滚动走 useInfiniteSessions（page 由 pageParam 管理）。
// enabled: !!novelId 守卫，novelId=0 时不 fetch。
export function useSessions(input: {
  novelId: number;
  page: number;
  size: number;
  search: string;
}) {
  return useQuery({
    queryKey: sessionKeys.list(
      input.novelId,
      input.page,
      input.size,
      input.search,
    ),
    queryFn: async () => {
      const r = await GetSessions({
        novel_id: input.novelId,
        page: input.page,
        size: input.size,
        search: input.search,
      });
      return r;
    },
    enabled: !!input.novelId,
  });
}

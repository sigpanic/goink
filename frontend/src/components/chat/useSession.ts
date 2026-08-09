import { useQuery } from "@tanstack/react-query";
import { GetSession } from "@/lib/wailsjs/go/app/App";
import { sessionKeys } from "@/lib/queryKeys";

// useSession: 单个会话详情 query（含 usage 字段，ChatPanel 据此恢复 lastUsage）。
// enabled: !!sid 守卫，sid 为空（新对话 / 未选中）时不 fetch。
// session_id 是 string（queryKeys.sessionKeys.detail 已修类型）。
export function useSession(sid: string) {
  return useQuery({
    queryKey: sessionKeys.detail(sid),
    queryFn: async () => {
      const detail = await GetSession(sid);
      return detail;
    },
    enabled: !!sid,
  });
}

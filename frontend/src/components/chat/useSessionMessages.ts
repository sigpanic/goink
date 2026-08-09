import { useQuery } from "@tanstack/react-query";
import { GetSessionMessages } from "@/lib/wailsjs/go/app/App";
import { sessionMessagesKeys } from "@/lib/queryKeys";

// useSessionMessages: 会话历史消息 query（ChatPanel 据此 rebuildTurns 重建 turns）。
// enabled: !!sid 守卫，sid 为空（新对话 / 未选中）时不 fetch。
// 注意：本 query 只负责「首次加载 / 切会话恢复历史」；流式过程中的 turns 增量
// 仍由本地 state 维护（agent 事件订阅），不走 query 缓存（5.1 特殊点 1）。
export function useSessionMessages(sid: string) {
  return useQuery({
    queryKey: sessionMessagesKeys.detail(sid),
    queryFn: async () => {
      const msgs = await GetSessionMessages(sid);
      return msgs ?? [];
    },
    enabled: !!sid,
  });
}

import { useInfiniteQuery, type InfiniteData } from "@tanstack/react-query";
import { GetSessionMessagesPage } from "@/lib/wailsjs/go/app/App";
import type { session } from "@/lib/wailsjs/go/models";
import { sessionMessagesKeys } from "@/lib/queryKeys";

const PAGE_SIZE = 20;

function chronologicalMessages(data: InfiniteData<session.MessagePage>) {
  return data.pages.toReversed().flatMap((page) => page.items);
}

// useSessionMessages: 会话历史消息 query（ChatPanel 据此 rebuildTurns 重建 turns）。
// enabled: !!sid 守卫，sid 为空（新对话 / 未选中）时不 fetch。
// 注意：本 query 只负责「首次加载 / 切会话恢复历史」；流式过程中的 turns 增量
// 仍由本地 state 维护（agent 事件订阅），不走 query 缓存（5.1 特殊点 1）。
export function useSessionMessages(sid: string) {
  return useInfiniteQuery({
    queryKey: sessionMessagesKeys.infiniteList(sid, PAGE_SIZE),
    queryFn: ({ pageParam }) =>
      GetSessionMessagesPage({
        session_id: sid,
        before_turn_id: pageParam,
        size: PAGE_SIZE,
      }),
    initialPageParam: 0,
    getNextPageParam: (lastPage) =>
      lastPage.has_more ? lastPage.next_before_turn_id : undefined,
    select: chronologicalMessages,
    enabled: !!sid,
  });
}

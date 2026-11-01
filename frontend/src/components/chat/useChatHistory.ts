import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type Dispatch,
  type SetStateAction,
} from "react";
import { useInView } from "react-intersection-observer";
import { rebuildTurns, type Turn } from "./types";
import { useSessionMessages } from "./useSessionMessages";

interface Options {
  sessionId: string;
  turns: Turn[];
  setTurns: Dispatch<SetStateAction<Turn[]>>;
}

// 管理历史分页、与本地 turn 的合并以及聊天列表的滚动位置。
export function useChatHistory({ sessionId, turns, setTurns }: Options) {
  const messagesQuery = useSessionMessages(sessionId);
  const { hasNextPage, isFetching, isFetchNextPageError, fetchNextPage } =
    messagesQuery;
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const scrollContainerRef = useRef<HTMLDivElement>(null);
  const isNearBottomRef = useRef(true);
  const [showScrollToBottom, setShowScrollToBottom] = useState(false);
  const localTurnIdsRef = useRef(new Set<string>());
  const oldestHistoryRef = useRef<{
    sessionId: string;
    turnId: number | undefined;
  } | null>(null);
  const historyScrollRef = useRef<{
    sessionId: string;
    anchor: HTMLElement | undefined;
    anchorTop: number;
    height: number;
  } | null>(null);
  const { ref: olderMessagesRef, inView: olderMessagesInView } = useInView({
    rootMargin: "100px",
    skip: !sessionId || !hasNextPage,
  });

  const reset = useCallback(() => {
    localTurnIdsRef.current.clear();
    oldestHistoryRef.current = null;
    historyScrollRef.current = null;
    isNearBottomRef.current = true;
    setShowScrollToBottom(false);
  }, []);

  const registerLocalTurn = useCallback((id: string) => {
    localTurnIdsRef.current.add(id);
  }, []);

  const loadOlderMessages = useCallback(() => {
    if (hasNextPage && !isFetching) {
      void fetchNextPage({ cancelRefetch: false });
    }
  }, [hasNextPage, isFetching, fetchNextPage]);

  useLayoutEffect(() => {
    if (!sessionId) {
      oldestHistoryRef.current = null;
      historyScrollRef.current = null;
      return;
    }
    if (oldestHistoryRef.current?.sessionId !== sessionId) {
      oldestHistoryRef.current = { sessionId, turnId: undefined };
      historyScrollRef.current = null;
      isNearBottomRef.current = true;
      setShowScrollToBottom(false);
    }
    const msgs = messagesQuery.data;
    if (!msgs) return;
    const history = rebuildTurns(msgs);
    const firstTurnId = history[0]?.turnId;
    const previousFirst = oldestHistoryRef.current.turnId;
    const container = scrollContainerRef.current;
    // 插入旧页前记录可见 turn，避免把底部同时增长的内容算进滚动补偿。
    if (
      container &&
      previousFirst !== undefined &&
      firstTurnId !== undefined &&
      firstTurnId < previousFirst
    ) {
      const top = container.getBoundingClientRect().top;
      const anchor = Array.from(
        container.querySelectorAll<HTMLElement>("[data-turn-id]"),
      ).find((element) => element.getBoundingClientRect().bottom > top);
      historyScrollRef.current = {
        sessionId,
        anchor,
        anchorTop: anchor?.getBoundingClientRect().top ?? 0,
        height: container.scrollHeight,
      };
    }
    oldestHistoryRef.current.turnId = firstTurnId;
    // 保留已显示 turn 的事件状态和 segment 标识，防止旧快照覆盖流式内容。
    setTurns((previous) => {
      const byTurn = new Map(history.map((turn) => [turn.turnId, turn]));
      const pending: Turn[] = [];
      for (const turn of previous) {
        if (turn.turnId === 0 && localTurnIdsRef.current.has(turn.id)) {
          pending.push(turn);
        } else {
          byTurn.set(turn.turnId, turn);
        }
      }
      return [...byTurn.values()]
        .sort((a, b) => a.turnId - b.turnId)
        .concat(pending);
    });
  }, [sessionId, messagesQuery.data, setTurns]);

  useLayoutEffect(() => {
    const snapshot = historyScrollRef.current;
    if (!snapshot) return;
    historyScrollRef.current = null;
    const container = scrollContainerRef.current;
    if (!container || snapshot.sessionId !== sessionId) return;
    const delta = snapshot.anchor?.isConnected
      ? snapshot.anchor.getBoundingClientRect().top - snapshot.anchorTop
      : container.scrollHeight - snapshot.height;
    container.scrollTop += delta;
    const nearBottom =
      container.scrollHeight - container.scrollTop - container.clientHeight <
      60;
    isNearBottomRef.current = nearBottom;
    setShowScrollToBottom(!nearBottom);
  }, [sessionId, turns]);

  useEffect(() => {
    if (
      sessionId &&
      olderMessagesInView &&
      !isFetchNextPageError &&
      (scrollContainerRef.current?.scrollTop ?? Infinity) < 120
    ) {
      loadOlderMessages();
    }
  }, [sessionId, olderMessagesInView, isFetchNextPageError, loadOlderMessages]);

  // 流式输出时自动滚到底部，但仅在用户未主动上滚时
  useEffect(() => {
    if (isNearBottomRef.current) {
      messagesEndRef.current?.scrollIntoView({ behavior: "instant" });
    }
  }, [turns]);

  const handleScroll = useCallback(() => {
    const el = scrollContainerRef.current;
    if (!el) return;
    isNearBottomRef.current =
      el.scrollHeight - el.scrollTop - el.clientHeight < 60;
    setShowScrollToBottom(!isNearBottomRef.current);
  }, []);

  const scrollToBottom = useCallback(() => {
    isNearBottomRef.current = true;
    setShowScrollToBottom(false);
    messagesEndRef.current?.scrollIntoView({ behavior: "instant" });
  }, []);

  return {
    messagesQuery,
    messagesEndRef,
    scrollContainerRef,
    olderMessagesRef,
    showScrollToBottom,
    loadOlderMessages,
    handleScroll,
    scrollToBottom,
    registerLocalTurn,
    reset,
  };
}

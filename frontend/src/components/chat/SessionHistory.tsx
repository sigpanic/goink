import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { MessageSquare, Loader2, History, Trash2 } from "lucide-react";
import { useInView } from "react-intersection-observer";
import { useTimeAgo } from "@/hooks/useTimeAgo";
import { useInfiniteSessions } from "./useInfiniteSessions";
import { useChatStore } from "./useChatStore";

interface Props {
  open: boolean;
  novelId: number;
  onClose: () => void;
  onSelectSession: (sessionId: string) => void;
}

export default function SessionHistory({
  open,
  novelId,
  onClose,
  onSelectSession,
}: Props) {
  const { t } = useTranslation();
  const [mounted, setMounted] = useState(false);
  // 相对时间：面板打开时每分钟自动刷新，关闭时停掉定时器
  const timeAgo = useTimeAgo(open);
  const [visible, setVisible] = useState(false);
  const [search, setSearch] = useState("");
  const [submittedSearch, setSubmittedSearch] = useState("");
  // 点删除只 dispatch store，ConfirmDialog + 执行由 DeleteSessionDialog 集中处理
  const setDeletingSession = useChatStore((s) => s.setDeletingSession);
  // 无限滚动：sentinel 进入视口时拉下一页（react-intersection-observer useInView）
  const { ref: sentinelRef, inView } = useInView({ rootMargin: "100px" });

  // 无限滚动 query：page 由 pageParam 管理（不进 queryKey）；
  // submittedSearch 变化 → queryKey 变化 → 自动重新从第一页 fetch。
  // enabled: open && !!novelId（面板关闭时不 fetch）。
  const sessionsQuery = useInfiniteSessions({
    novelId,
    size: 20,
    search: submittedSearch,
    enabled: open,
  });

  const sessions =
    sessionsQuery.data?.pages.flatMap((p) => p.items ?? []) ?? [];
  const total = sessionsQuery.data?.pages[0]?.total ?? 0;
  const hasMore = sessionsQuery.hasNextPage;
  const isLoading = sessionsQuery.isLoading;
  const isFetchingMore = sessionsQuery.isFetchingNextPage;

  useEffect(() => {
    if (open) {
      setMounted(true);
      requestAnimationFrame(() => setVisible(true));
    } else {
      setVisible(false);
      const timer = setTimeout(() => setMounted(false), 200);
      return () => clearTimeout(timer);
    }
  }, [open]);

  // 搜索防抖 300ms：search 输入 → submittedSearch 更新 → queryKey 变化 → refetch
  useEffect(() => {
    const timer = setTimeout(() => {
      setSubmittedSearch(search);
    }, 300);
    return () => clearTimeout(timer);
  }, [search]);

  // 无限滚动：sentinel 进入视口时拉下一页（替代原手写 scroll 事件 + 距离判断）
  useEffect(() => {
    if (inView && hasMore && !isFetchingMore) {
      sessionsQuery.fetchNextPage();
    }
  }, [inView, hasMore, isFetchingMore, sessionsQuery]);

  if (!mounted) return null;

  return (
    <div className="absolute inset-0 pointer-events-none">
      <div
        className="absolute inset-0 z-30 pointer-events-auto"
        onClick={onClose}
      />
      <div
        className={`absolute right-3 left-3 z-40 flex flex-col bg-card border rounded-xl shadow-lg pointer-events-auto transition-all duration-200 ease-out overflow-hidden ${visible ? "opacity-100 translate-y-0" : "opacity-0 -translate-y-2"}`}
        style={{ height: "40%", top: "4px" }}
      >
        <div className="flex items-center justify-between px-4 py-2 border-b shrink-0">
          <div className="flex items-center gap-3">
            <div className="flex items-center gap-2">
              <History className="w-4 h-4 text-muted-foreground" />
              <span className="text-xs font-medium">
                {t("chat.historySessions")}
              </span>
            </div>
            {total > 0 && (
              <span className="text-[10px] text-muted-foreground">
                {t("chat.totalSessions", { count: total })}
              </span>
            )}
          </div>
        </div>

        <div className="px-4 py-2 shrink-0">
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder={t("chat.searchSessions")}
            className="w-full h-7 rounded-md border bg-muted/30 px-2.5 text-xs"
          />
        </div>

        {/* 会话列表 */}
        <div className="flex-1 overflow-y-auto overscroll-contain px-3 pb-2">
          {sessions.length === 0 && isLoading ? (
            <div className="flex items-center justify-center h-full">
              <Loader2 className="w-4 h-4 animate-spin text-muted-foreground" />
            </div>
          ) : sessions.length === 0 && submittedSearch ? (
            <div className="flex items-center justify-center h-full">
              <span className="text-xs text-muted-foreground">
                {t("chat.noMatchingSessions")}
              </span>
            </div>
          ) : (
            <div className="space-y-0.5">
              {sessions.map((s) => (
                <div
                  key={s.session_id}
                  onClick={() => {
                    onSelectSession(s.session_id);
                    onClose();
                  }}
                  className="w-full flex items-center gap-2.5 px-2.5 py-2.5 rounded-lg text-left hover:bg-muted/50 transition-colors cursor-pointer select-none group"
                >
                  <MessageSquare className="w-4 h-4 shrink-0 text-muted-foreground" />
                  <div className="min-w-0 flex-1">
                    <div className="text-xs truncate">
                      {s.title || t("chat.newChat")}
                    </div>
                    <div className="text-[10px] text-muted-foreground mt-0.5">
                      {timeAgo(s.updated_at)}
                    </div>
                  </div>
                  <button
                    onClick={(e) => {
                      e.stopPropagation();
                      setDeletingSession(s);
                    }}
                    className="shrink-0 p-1 rounded text-muted-foreground hover:text-destructive opacity-0 group-hover:opacity-100 transition-opacity"
                    title={t("common.delete")}
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </button>
                </div>
              ))}
              {isFetchingMore && (
                <div className="flex justify-center py-3">
                  <Loader2 className="w-4 h-4 animate-spin text-muted-foreground" />
                </div>
              )}
              {!hasMore && sessions.length > 0 && (
                <div className="text-center text-[10px] text-muted-foreground py-2">
                  {t("chat.allSessionsShown")}
                </div>
              )}
              {/* 无限滚动 sentinel：进入视口时触发 fetchNextPage */}
              <div ref={sentinelRef} className="h-4" />
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

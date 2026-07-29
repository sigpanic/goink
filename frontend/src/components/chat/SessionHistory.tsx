import { useEffect, useRef, useState, useCallback } from "react";
import { useTranslation } from "react-i18next";
import { MessageSquare, Loader2, History, Trash2 } from "lucide-react";
import type { app } from "@/hooks/useApp";
import { useApp } from "@/hooks/useApp";
import { useDeleteSession } from "@/hooks/useDeleteSession";
import ConfirmDialog from "@/components/ui/ConfirmDialog";

interface Props {
  open: boolean;
  novelId: number;
  onClose: () => void;
  onSelectSession: (sessionId: string) => void;
  onSessionDeleted: (sessionId: string) => void;
}

export default function SessionHistory({
  open,
  novelId,
  onClose,
  onSelectSession,
  onSessionDeleted,
}: Props) {
  const { t } = useTranslation();
  const app = useApp();
  const [mounted, setMounted] = useState(false);
  const [now, setNow] = useState(() => Date.now());

  // 面板打开时每分钟刷新时间，保证 timeAgo 相对时间准确
  useEffect(() => {
    if (!open) return;
    const timer = setInterval(() => setNow(Date.now()), 60_000);
    return () => clearInterval(timer);
  }, [open]);

  function timeAgo(iso: string): string {
    const diff = now - new Date(iso).getTime();
    const min = Math.floor(diff / 60000);
    if (min < 1) return t("chat.justNow");
    if (min < 60) return t("chat.minutesAgo", { count: min });
    const hour = Math.floor(min / 60);
    if (hour < 24) return t("chat.hoursAgo", { count: hour });
    const day = Math.floor(hour / 24);
    if (day < 30) return t("chat.daysAgo", { count: day });
    return t("chat.monthsAgo", { count: Math.floor(day / 30) });
  }
  const [visible, setVisible] = useState(false);
  const [sessions, setSessions] = useState<app.SessionMeta[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [isLoading, setIsLoading] = useState(false);
  const [hasMore, setHasMore] = useState(true);
  const [search, setSearch] = useState("");
  const [submittedSearch, setSubmittedSearch] = useState("");
  const listRef = useRef<HTMLDivElement>(null);
  const loadingRef = useRef(false);
  const searchRef = useRef("");

  const loadPageRef = useRef<((p: number) => void) | null>(null);

  useEffect(() => {
    loadPageRef.current = async (p: number) => {
      if (loadingRef.current) return;
      loadingRef.current = true;
      setIsLoading(true);
      try {
        const result = await app.GetSessions({
          novel_id: novelId,
          page: p,
          size: 20,
          search: searchRef.current,
        });
        if (result?.items) {
          setSessions((prev) =>
            p === 1 ? result.items : [...prev, ...result.items],
          );
          setTotal(result.total);
          setHasMore(result.page < result.total_pages);
        }
      } catch {
        // ignore
      } finally {
        setIsLoading(false);
        loadingRef.current = false;
      }
    };
  }, [app, novelId]);

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

  // 搜索防抖 300ms
  useEffect(() => {
    const timer = setTimeout(() => {
      if (searchRef.current !== search) {
        searchRef.current = search;
        setSubmittedSearch(search);
        setSessions([]);
        setPage(1);
        setHasMore(true);
        loadPageRef.current?.(1);
      }
    }, 300);
    return () => clearTimeout(timer);
  }, [search]);

  useEffect(() => {
    if (!open) return;
    setSearch("");
    setSubmittedSearch("");
    searchRef.current = "";
    setSessions([]);
    setPage(1);
    setHasMore(true);
    loadPageRef.current?.(1);
  }, [open, novelId]);

  const handleScroll = useCallback(() => {
    if (!listRef.current || !hasMore || isLoading) return;
    const { scrollTop, scrollHeight, clientHeight } = listRef.current;
    if (scrollHeight - scrollTop - clientHeight < 80) {
      const next = page + 1;
      setPage(next);
      loadPageRef.current?.(next);
    }
  }, [hasMore, isLoading, page]);

  // 删除会话：复用 useDeleteSession hook（统一 deleteTarget/deleting/错误提示）。
  // 删除成功后既更新本地分页 state（SessionHistory 自管列表），又通过 onSessionDeleted
  // 通知父组件 ChatPanel 更新最近会话列表 / 清空当前活跃 session 视图。
  const { deleteTarget, deleting, setDeleteTarget, handleDeleteSession } =
    useDeleteSession((sid) => {
      setSessions((prev) => prev.filter((s) => s.session_id !== sid));
      setTotal((prev) => Math.max(0, prev - 1));
      onSessionDeleted(sid);
    });

  if (!mounted) return null;

  return (
    <>
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
        <div
          ref={listRef}
          onScroll={handleScroll}
          className="flex-1 overflow-y-auto overscroll-contain px-3 pb-2"
        >
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
                      setDeleteTarget(s);
                    }}
                    className="shrink-0 p-1 rounded text-muted-foreground hover:text-destructive opacity-0 group-hover:opacity-100 transition-opacity"
                    title={t("common.delete")}
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </button>
                </div>
              ))}
              {isLoading && (
                <div className="flex justify-center py-3">
                  <Loader2 className="w-4 h-4 animate-spin text-muted-foreground" />
                </div>
              )}
              {!hasMore && sessions.length > 0 && (
                <div className="text-center text-[10px] text-muted-foreground py-2">
                  {t("chat.allSessionsShown")}
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
    <ConfirmDialog
      open={deleteTarget !== null}
      title={t("chat.confirmDeleteSession")}
      message={
        deleteTarget
          ? t("chat.confirmDeleteSessionMessage", {
              title: deleteTarget.title || t("chat.newChat"),
            })
          : ""
      }
      danger
      loading={deleting}
      confirmText={t("common.delete")}
      onClose={() => setDeleteTarget(null)}
      onConfirm={handleDeleteSession}
    />
    </>
  );
}

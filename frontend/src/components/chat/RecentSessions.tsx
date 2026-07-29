import { useState } from "react";
import { MessageSquare, Trash2 } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { app } from "@/hooks/useApp";
import { useDeleteSession } from "@/hooks/useDeleteSession";
import ConfirmDialog from "@/components/ui/ConfirmDialog";

interface Props {
  sessions: app.SessionMeta[];
  total: number;
  onSelectSession: (sessionId: string) => void;
  onViewAll: () => void;
  onDeleteSession: (sessionId: string) => void;
}

export default function RecentSessions({
  sessions,
  total,
  onSelectSession,
  onViewAll,
  onDeleteSession,
}: Props) {
  const { t } = useTranslation();
  const [now] = useState(() => Date.now());

  // 删除会话：复用 useDeleteSession hook。RecentSessions 的列表数据由父组件 ChatPanel
  // 传入，删除成功后只需通过 onDeleteSession 通知父组件更新，自身无需维护列表 state。
  const { deleteTarget, deleting, setDeleteTarget, handleDeleteSession } =
    useDeleteSession(onDeleteSession);

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

  return (
    <div className="flex flex-col h-full">
      {sessions.length > 0 && (
        <div className="flex-1 overflow-y-auto px-3 pb-2">
          <div className="text-xs text-muted-foreground mb-2 px-1 select-none">
            {t("chat.recentChats")}
          </div>
          <div className="space-y-0.5">
            {sessions.map((s) => (
              <div
                key={s.session_id}
                onClick={() => onSelectSession(s.session_id)}
                className="w-full flex items-center gap-2 px-2.5 py-2 rounded-lg text-left hover:bg-muted/50 transition-colors cursor-pointer select-none group"
              >
                <MessageSquare className="w-3.5 h-3.5 shrink-0 text-muted-foreground" />
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
          </div>

          {total > sessions.length && (
            <button
              onClick={onViewAll}
              className="w-full text-center text-xs text-muted-foreground hover:text-foreground py-2 transition-colors cursor-pointer select-none"
            >
              {t("chat.viewAll", { count: total })}
            </button>
          )}
        </div>
      )}
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
    </div>
  );
}

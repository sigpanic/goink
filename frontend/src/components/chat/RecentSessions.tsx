import { MessageSquare, Trash2 } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { app } from "@/lib/wailsjs/go/models";
import { useTimeAgo } from "@/hooks/useTimeAgo";
import { useChatStore } from "./useChatStore";

interface Props {
  sessions: app.SessionMeta[];
  total: number;
  onSelectSession: (sessionId: string) => void;
  onViewAll: () => void;
}

export default function RecentSessions({
  sessions,
  total,
  onSelectSession,
  onViewAll,
}: Props) {
  const { t } = useTranslation();
  // 相对时间：组件挂载即可见，每分钟自动刷新
  const timeAgo = useTimeAgo();
  // 点删除只 dispatch store，ConfirmDialog + 执行由 DeleteSessionDialog 集中处理
  const setDeletingSession = useChatStore((s) => s.setDeletingSession);

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
                    setDeletingSession(s);
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
    </div>
  );
}

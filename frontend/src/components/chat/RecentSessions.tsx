import { useState } from "react";
import { MessageSquare } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { app } from "@/hooks/useApp";

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
  const [now] = useState(() => Date.now());

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
              <button
                key={s.session_id}
                onClick={() => onSelectSession(s.session_id)}
                className="w-full flex items-center gap-2 px-2.5 py-2 rounded-lg text-left hover:bg-muted/50 transition-colors cursor-pointer select-none"
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
              </button>
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

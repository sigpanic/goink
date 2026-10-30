import { MessageSquare, Loader2 } from "lucide-react";
import type { Ref } from "react";
import { useTranslation } from "react-i18next";
import type { Turn } from "./types";
import type { useSessions } from "./useSessions";
import type { useSessionMessages } from "./useSessionMessages";
import RecentSessions from "./RecentSessions";
import ChatTurn from "./ChatTurn";

interface Props {
  hasNovel: boolean;
  hasTurns: boolean;
  showRecent: boolean;
  isLoading: boolean;
  turns: Turn[];
  sessionsQuery: Pick<ReturnType<typeof useSessions>, "data">;
  messagesQuery: Pick<
    ReturnType<typeof useSessionMessages>,
    | "isLoading"
    | "isError"
    | "refetch"
    | "hasNextPage"
    | "isFetchingNextPage"
    | "isFetchNextPageError"
  >;
  olderMessagesRef: Ref<HTMLDivElement>;
  onLoadOlder: () => void;
  onSelectSession: (sessionId: string) => void;
  onViewAll: () => void;
  onApprove: (toolId: string, feedback: string) => Promise<void>;
  onReject: (toolId: string, feedback: string) => Promise<void>;
}

export default function ChatMessageList({
  hasNovel,
  hasTurns,
  showRecent,
  isLoading,
  turns,
  sessionsQuery,
  messagesQuery,
  olderMessagesRef,
  onLoadOlder,
  onSelectSession,
  onViewAll,
  onApprove,
  onReject,
}: Props) {
  const { t } = useTranslation();

  return !hasNovel ? (
    <div className="flex items-center justify-center h-full">
      <div className="text-center">
        <MessageSquare className="w-10 h-10 text-muted-foreground/20 mx-auto mb-3" />
        <p className="text-sm text-muted-foreground">{t("chat.selectNovel")}</p>
      </div>
    </div>
  ) : showRecent ? (
    <RecentSessions
      sessions={sessionsQuery.data?.items ?? []}
      total={sessionsQuery.data?.total ?? 0}
      onSelectSession={onSelectSession}
      onViewAll={onViewAll}
    />
  ) : messagesQuery.isLoading && !hasTurns ? (
    <div className="flex items-center justify-center h-full">
      <Loader2 className="w-5 h-5 animate-spin text-muted-foreground" />
    </div>
  ) : (
    <>
      {/* 消息列表 */}
      {messagesQuery.isError && !hasTurns ? (
        <div className="flex items-center justify-center h-full">
          <div className="text-center">
            <p className="text-sm text-red-500 mb-2">
              {t("chat.loadMessagesFailed")}
            </p>
            <button
              onClick={() => messagesQuery.refetch()}
              className="text-xs text-primary underline cursor-pointer"
            >
              {t("chat.retry")}
            </button>
          </div>
        </div>
      ) : !hasTurns && !isLoading ? (
        <div className="flex items-center justify-center h-full">
          <div className="text-center">
            <MessageSquare className="w-10 h-10 text-muted-foreground/20 mx-auto mb-3" />
            <p className="text-sm text-muted-foreground">
              {t("chat.startConversation")}
            </p>
          </div>
        </div>
      ) : (
        <div className="space-y-4">
          {messagesQuery.hasNextPage && (
            <div
              ref={olderMessagesRef}
              className="flex items-center justify-center gap-2 py-2 text-xs text-muted-foreground"
            >
              {messagesQuery.isFetchingNextPage ? (
                <Loader2
                  className="h-4 w-4 animate-spin"
                  aria-label={t("chat.loadingOlderMessages")}
                />
              ) : (
                <>
                  {messagesQuery.isFetchNextPageError && (
                    <span role="alert">
                      {t("chat.loadOlderMessagesFailed")}
                    </span>
                  )}
                  <button
                    type="button"
                    onClick={onLoadOlder}
                    className="hover:text-foreground hover:underline"
                  >
                    {t(
                      messagesQuery.isFetchNextPageError
                        ? "chat.retry"
                        : "chat.loadOlderMessages",
                    )}
                  </button>
                </>
              )}
            </div>
          )}
          {turns.map((turn) => (
            <ChatTurn
              key={turn.id}
              turn={turn}
              onApprove={onApprove}
              onReject={onReject}
            />
          ))}
        </div>
      )}
    </>
  );
}

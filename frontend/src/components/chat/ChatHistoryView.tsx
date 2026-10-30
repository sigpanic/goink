import type { ComponentProps } from "react";
import { ArrowDown } from "lucide-react";
import { useTranslation } from "react-i18next";
import ChatMessageList from "./ChatMessageList";
import type { useChatHistory } from "./useChatHistory";

type Props = Omit<
  ComponentProps<typeof ChatMessageList>,
  "messagesQuery" | "olderMessagesRef" | "onLoadOlder"
> & {
  history: ReturnType<typeof useChatHistory>;
};

export default function ChatHistoryView({ history, ...props }: Props) {
  const { t } = useTranslation();
  const {
    scrollContainerRef,
    messagesEndRef,
    messagesQuery,
    olderMessagesRef,
    handleScroll,
    loadOlderMessages,
    showScrollToBottom,
    scrollToBottom,
  } = history;

  return (
    <div className="relative flex-1 min-h-0">
      <div
        ref={scrollContainerRef}
        onScroll={handleScroll}
        className="h-full overflow-y-auto overscroll-contain px-3 py-3 relative"
      >
        <ChatMessageList
          {...props}
          messagesQuery={messagesQuery}
          olderMessagesRef={olderMessagesRef}
          onLoadOlder={loadOlderMessages}
        />
        <div ref={messagesEndRef} />
      </div>
      {showScrollToBottom && props.hasTurns && (
        <button
          type="button"
          aria-label={t("chat.scrollToBottom")}
          title={t("chat.scrollToBottom")}
          onClick={scrollToBottom}
          className="absolute bottom-3 right-4 z-10 flex h-8 w-8 items-center justify-center rounded-full border bg-background text-muted-foreground shadow-sm hover:text-foreground"
        >
          <ArrowDown className="h-4 w-4" aria-hidden="true" />
        </button>
      )}
    </div>
  );
}

import { memo } from "react";
import { useTranslation } from "react-i18next";
import Markdown from "@/components/Markdown";
import type { attachment } from "@/lib/wailsjs/go/models";
import ChatImages from "./ChatImages";

interface Props {
  role: "user" | "assistant";
  content: string;
  novelId?: number;
  images?: attachment.Ref[];
}

export default memo(function MessageBubble({
  role,
  content,
  novelId = 0,
  images = [],
}: Props) {
  const { t } = useTranslation();
  const isUser = role === "user";

  return (
    <div className={`flex ${isUser ? "justify-end" : "justify-start"}`}>
      <div
        className={`min-w-0 max-w-[85%] space-y-3 rounded-2xl border px-4 py-3 shadow-xs [overflow-wrap:anywhere] ${
          isUser
            ? "border-bubble-user-foreground/10 bg-bubble-user text-bubble-user-foreground rounded-br-md"
            : "border-border/30 bg-card text-foreground rounded-bl-md"
        }`}
      >
        {images.length > 0 && <ChatImages novelId={novelId} images={images} />}
        {content && (
          <div
            role={isUser ? "group" : undefined}
            aria-label={isUser ? t("chat.userInput") : undefined}
            tabIndex={isUser ? 0 : undefined}
            className={
              isUser
                ? "max-h-[min(320px,40vh)] overflow-y-auto overscroll-contain rounded-sm focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring/60"
                : undefined
            }
          >
            <Markdown
              content={content}
              className={`${isUser ? "markdown-user " : ""}leading-relaxed [&>:first-child]:mt-0 [&>:last-child]:mb-0`}
            />
          </div>
        )}
      </div>
    </div>
  );
});

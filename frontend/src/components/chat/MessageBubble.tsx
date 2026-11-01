import { memo, useEffect, useState } from "react";
import { Check, Copy } from "lucide-react";
import { useTranslation } from "react-i18next";
import Markdown from "@/components/Markdown";
import type { attachment } from "@/lib/wailsjs/go/models";
import { ClipboardSetText } from "@/lib/wailsjs/runtime/runtime";
import { toastError } from "@/utils/toast";
import ChatImages from "./ChatImages";

interface Props {
  role: "user" | "assistant";
  content: string;
  novelId?: number;
  images?: attachment.Ref[];
  copyable?: boolean;
  sentAt?: string;
}

export default memo(function MessageBubble({
  role,
  content,
  novelId = 0,
  images = [],
  copyable = false,
  sentAt,
}: Props) {
  const { t, i18n } = useTranslation();
  const [copying, setCopying] = useState(false);
  const [copiedText, setCopiedText] = useState<string | null>(null);
  const copied = copiedText === content;
  const isUser = role === "user";
  const canCopy = copyable && content.trim().length > 0;
  const date = sentAt ? new Date(sentAt) : null;
  const hasTime = date !== null && Number.isFinite(date.getTime());

  useEffect(() => {
    if (copiedText === null) return;
    const timer = window.setTimeout(() => setCopiedText(null), 2000);
    return () => window.clearTimeout(timer);
  }, [copiedText]);

  async function copy() {
    setCopying(true);
    try {
      if (await ClipboardSetText(content)) setCopiedText(content);
      else toastError(t("chat.copyMessageFailed"));
    } catch {
      toastError(t("chat.copyMessageFailed"));
    } finally {
      setCopying(false);
    }
  }

  return (
    <div className={`flex ${isUser ? "justify-end" : "justify-start"}`}>
      <div className="group/message min-w-0 max-w-[85%]">
        <div
          className={`space-y-3 rounded-2xl border px-4 py-3 shadow-xs [overflow-wrap:anywhere] ${
            isUser
              ? "border-bubble-user-foreground/10 bg-bubble-user text-bubble-user-foreground rounded-br-md"
              : "border-border/30 bg-card text-foreground rounded-bl-md"
          }`}
        >
          {images.length > 0 && (
            <ChatImages novelId={novelId} images={images} />
          )}
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
        {copyable && (canCopy || hasTime) && (
          <div className="mt-1 flex h-6 items-center justify-end gap-2 px-1 text-xs text-muted-foreground">
            {hasTime && (
              <time
                dateTime={sentAt}
                title={date.toLocaleString(i18n.language)}
                className="opacity-0 transition-opacity group-hover/message:opacity-100 group-focus-within/message:opacity-100"
              >
                {date.toLocaleTimeString(i18n.language, {
                  hour: "2-digit",
                  minute: "2-digit",
                  hourCycle: "h23",
                })}
              </time>
            )}
            {canCopy && (
              <button
                type="button"
                aria-label={t(
                  copied ? "chat.messageCopied" : "chat.copyMessage",
                )}
                title={t(copied ? "chat.messageCopied" : "chat.copyMessage")}
                disabled={copying}
                onClick={() => void copy()}
                className="rounded p-1 transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-2 focus-visible:outline-ring disabled:opacity-40"
              >
                {copied ? (
                  <Check className="h-3.5 w-3.5" />
                ) : (
                  <Copy className="h-3.5 w-3.5" />
                )}
              </button>
            )}
            {copied && (
              <span role="status" className="sr-only">
                {t("chat.messageCopied")}
              </span>
            )}
          </div>
        )}
      </div>
    </div>
  );
});

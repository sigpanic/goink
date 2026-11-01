import { memo } from "react";
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
  const isUser = role === "user";

  return (
    <div className={`flex ${isUser ? "justify-end" : "justify-start"}`}>
      <div
        className={`max-w-[85%] rounded-xl px-3.5 py-3 break-words ${
          isUser
            ? "bg-bubble-user text-bubble-user-foreground rounded-br-sm"
            : "bg-card border border-border/30 text-foreground rounded-bl-sm shadow-xs"
        }`}
      >
        {images.length > 0 && <ChatImages novelId={novelId} images={images} />}
        <Markdown
          content={content}
          className={isUser ? "markdown-user" : undefined}
        />
      </div>
    </div>
  );
});

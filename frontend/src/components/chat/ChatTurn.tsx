import { useId, useState } from "react";
import { ChevronRight, Loader2 } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { Turn } from "./types";
import MessageBubble from "./MessageBubble";
import TurnSegmentView from "./TurnSegmentView";
import TurnOutcome from "./TurnOutcome";

interface Props {
  turn: Turn;
  onApprove: (toolId: string, feedback: string) => Promise<void>;
  onReject: (toolId: string, feedback: string) => Promise<void>;
}

export default function ChatTurn({ turn, onApprove, onReject }: Props) {
  const { t } = useTranslation();
  const historyId = useId();
  const [historyExpanded, setHistoryExpanded] = useState(false);
  const finalIndex = turn.segments.findLastIndex(
    (seg) => seg.type === "text" && seg.content.trim() !== "",
  );
  const canCollapse =
    turn.status === "done" &&
    !turn.compressionOnly &&
    finalIndex >= 0 &&
    turn.segments.slice(finalIndex + 1).every((seg) => seg.type === "text") &&
    turn.segments.some((seg, index) =>
      seg.type === "text"
        ? !!(
            seg.thinkingContent ||
            seg.hasThinking ||
            (index !== finalIndex && seg.content)
          )
        : seg.type !== "tool" || seg.toolName !== "run_subagent",
    );

  return (
    <div className="space-y-2">
      {turn.userMessage && (
        <MessageBubble role="user" content={turn.userMessage} />
      )}

      <div className="space-y-2">
        {canCollapse && (
          <button
            type="button"
            aria-expanded={historyExpanded}
            aria-controls={historyId}
            onClick={() => setHistoryExpanded((expanded) => !expanded)}
            className="flex items-center gap-1.5 rounded-md px-1 py-1 text-xs text-muted-foreground hover:text-foreground focus-visible:outline-2 focus-visible:outline-ring"
          >
            <ChevronRight
              className={`h-3.5 w-3.5 ${historyExpanded ? "rotate-90" : ""}`}
              aria-hidden="true"
            />
            {t("chat.workHistory")}
          </button>
        )}
        <div
          id={historyId}
          hidden={canCollapse && !historyExpanded}
          className="space-y-2"
        >
          {turn.segments.map((segment, index) => (
            <TurnSegmentView
              key={segment.id}
              segment={segment}
              isTurnStreaming={turn.status === "streaming"}
              hideContent={canCollapse && index === finalIndex}
              onApprove={onApprove}
              onReject={onReject}
            />
          ))}
        </div>
      </div>

      {canCollapse && (
        <MessageBubble
          role="assistant"
          content={turn.segments[finalIndex].content}
        />
      )}

      <TurnOutcome
        status={turn.status}
        errorMessage={turn.errorMessage}
        retrying={turn.retrying}
      />
      {turn.status === "streaming" &&
        turn.segments.length === 0 &&
        !turn.retrying && (
          <div className="flex justify-start">
            <div className="bg-muted rounded-lg rounded-bl-sm px-3 py-2">
              <Loader2 className="w-4 h-4 animate-spin text-muted-foreground" />
            </div>
          </div>
        )}
    </div>
  );
}

import type { ComponentProps } from "react";
import { useTranslation } from "react-i18next";
import type { Turn, TurnSegment } from "./types";
import MessageBubble from "./MessageBubble";
import ThinkingBlock from "./ThinkingBlock";
import ToolCallCard from "./ToolCallCard";
import WebSearchCard from "./WebSearchCard";
import WebFetchCard from "./WebFetchCard";
import SubagentCard from "./SubagentCard";
import CompressionBlock from "./CompressionBlock";

interface Props extends Pick<
  ComponentProps<typeof MessageBubble>,
  "copyable" | "sentAt"
> {
  segment: TurnSegment;
  isTurnStreaming: boolean;
  turnStatus: Turn["status"];
  hideContent?: boolean;
  onApprove: (toolId: string, feedback: string) => Promise<void>;
  onReject: (toolId: string, feedback: string) => Promise<void>;
}

export default function TurnSegmentView({
  segment: seg,
  isTurnStreaming,
  turnStatus,
  hideContent = false,
  copyable,
  sentAt,
  onApprove,
  onReject,
}: Props) {
  const { t } = useTranslation();
  // 未走完的 segment 在 turn 结束时统一显示中性状态，文案区分具体结局
  const incompleteLabel =
    turnStatus === "user_stopped"
      ? t("chat.cancelled")
      : turnStatus === "system_interrupted"
        ? t("chat.interrupted")
        : turnStatus === "error"
          ? t("chat.unfinished")
          : t("chat.finished");

  if (seg.type === "subagent" && seg.agentType) {
    return (
      <SubagentCard
        agentType={seg.agentType}
        segments={seg.segments || []}
        status={seg.status || "done"}
        retrying={seg.retrying}
        errorMessage={seg.errorMessage}
        incompleteLabel={incompleteLabel}
      />
    );
  }

  if (seg.type === "tool") {
    // run_subagent 已由 subagent 段渲染，跳过纯工具卡
    if (seg.toolName === "run_subagent") return null;

    if (
      seg.toolName === "web_search" &&
      seg.toolStatus === "completed" &&
      seg.result
    ) {
      return <WebSearchCard result={seg.result} />;
    }
    if (
      seg.toolName === "web_fetch" &&
      seg.toolStatus === "completed" &&
      seg.result
    ) {
      return <WebFetchCard result={seg.result} displayText={seg.displayText} />;
    }

    return (
      <ToolCallCard
        toolName={seg.toolName}
        displayText={seg.displayText}
        status={seg.toolStatus}
        activityKind={seg.activityKind}
        error={seg.error}
        approvalType={seg.approvalType}
        approvalPayload={seg.approvalPayload}
        result={seg.result}
        incompleteLabel={incompleteLabel}
        onApprove={
          seg.toolStatus === "awaiting_approval"
            ? (feedback: string) => onApprove(seg.toolId, feedback)
            : undefined
        }
        onReject={
          seg.toolStatus === "awaiting_approval"
            ? (feedback: string) => onReject(seg.toolId, feedback)
            : undefined
        }
      />
    );
  }

  if (seg.type === "compression") {
    return <CompressionBlock phase={seg.compressionPhase || "compressing"} />;
  }

  if (
    !seg.thinkingContent &&
    !seg.hasThinking &&
    (hideContent || !seg.content)
  ) {
    return null;
  }

  return (
    <div>
      {(seg.thinkingContent || seg.hasThinking) && (
        <div className="max-w-[85%]">
          <ThinkingBlock
            content={seg.thinkingContent}
            hasThinking={seg.hasThinking}
            isStreaming={
              isTurnStreaming && !seg.thinkingDone && seg.isStreaming
            }
          />
        </div>
      )}
      {!hideContent && seg.content && (
        <MessageBubble
          role="assistant"
          content={seg.content}
          copyable={copyable}
          sentAt={sentAt}
        />
      )}
    </div>
  );
}

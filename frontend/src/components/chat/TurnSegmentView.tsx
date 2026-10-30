import type { TurnSegment } from "./types";
import MessageBubble from "./MessageBubble";
import ThinkingBlock from "./ThinkingBlock";
import ToolCallCard from "./ToolCallCard";
import WebSearchCard from "./WebSearchCard";
import WebFetchCard from "./WebFetchCard";
import SubagentCard from "./SubagentCard";
import CompressionBlock from "./CompressionBlock";

interface Props {
  segment: TurnSegment;
  isTurnStreaming: boolean;
  hideContent?: boolean;
  onApprove: (toolId: string, feedback: string) => Promise<void>;
  onReject: (toolId: string, feedback: string) => Promise<void>;
}

export default function TurnSegmentView({
  segment: seg,
  isTurnStreaming,
  hideContent = false,
  onApprove,
  onReject,
}: Props) {
  if (seg.type === "subagent" && seg.agentType) {
    return (
      <SubagentCard
        agentType={seg.agentType}
        segments={seg.segments || []}
        status={seg.status || "done"}
        retrying={seg.retrying}
        errorMessage={seg.errorMessage}
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
        <MessageBubble role="assistant" content={seg.content} />
      )}
    </div>
  );
}

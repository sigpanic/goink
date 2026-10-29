import { Loader2 } from "lucide-react";
import type { Turn } from "./types";
import MessageBubble from "./MessageBubble";
import ThinkingBlock from "./ThinkingBlock";
import ToolCallCard from "./ToolCallCard";
import WebSearchCard from "./WebSearchCard";
import WebFetchCard from "./WebFetchCard";
import SubagentCard from "./SubagentCard";
import CompressionBlock from "./CompressionBlock";
import TurnOutcome from "./TurnOutcome";

interface Props {
  turn: Turn;
  onApprove: (toolId: string, feedback: string) => Promise<void>;
  onReject: (toolId: string, feedback: string) => Promise<void>;
}

export default function ChatTurn({ turn, onApprove, onReject }: Props) {
  return (
    <div className="space-y-2">
      {turn.userMessage && (
        <MessageBubble role="user" content={turn.userMessage} />
      )}

      {turn.segments.map((seg) => {
        if (seg.type === "subagent" && seg.agentType) {
          return (
            <SubagentCard
              key={seg.id}
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
            return <WebSearchCard key={seg.id} result={seg.result} />;
          }
          if (
            seg.toolName === "web_fetch" &&
            seg.toolStatus === "completed" &&
            seg.result
          ) {
            return (
              <WebFetchCard
                key={seg.id}
                result={seg.result}
                displayText={seg.displayText}
              />
            );
          }

          return (
            <ToolCallCard
              key={seg.id}
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
          return (
            <CompressionBlock
              key={seg.id}
              phase={seg.compressionPhase || "compressing"}
            />
          );
        }

        return (
          <div key={seg.id}>
            {(seg.thinkingContent || seg.hasThinking) && (
              <div className="max-w-[85%]">
                <ThinkingBlock
                  content={seg.thinkingContent}
                  hasThinking={seg.hasThinking}
                  isStreaming={
                    turn.status === "streaming" &&
                    !seg.thinkingDone &&
                    seg.isStreaming
                  }
                />
              </div>
            )}
            {seg.content && (
              <MessageBubble role="assistant" content={seg.content} />
            )}
          </div>
        );
      })}

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

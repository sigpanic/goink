import type { Turn, TurnSegment } from "./types";

// turn 进入终态后，仍在进行中的 segment 不会再收到终态事件（例如参数流式中途停止的工具），
// 统一收敛为非错误的 incomplete，避免卡片一直停留在“进行中”。
function finalizeSegments(segments: TurnSegment[]): TurnSegment[] {
  let changed = false;
  const next = segments.map((seg) => {
    if (seg.type === "tool") {
      if (
        seg.toolStatus === "executing" ||
        seg.toolStatus === "awaiting_approval"
      ) {
        changed = true;
        return {
          ...seg,
          toolStatus: "incomplete" as const,
          approvalType: undefined,
          approvalPayload: undefined,
        };
      }
      return seg;
    }
    if (seg.type === "subagent") {
      const childSegments = seg.segments
        ? finalizeSegments(seg.segments)
        : seg.segments;
      const status =
        seg.status === "streaming" ? ("incomplete" as const) : seg.status;
      if (childSegments === seg.segments && status === seg.status) return seg;
      changed = true;
      return { ...seg, segments: childSegments, status, retrying: null };
    }
    if (seg.type === "compression" && seg.compressionPhase === "compressing") {
      changed = true;
      return { ...seg, compressionPhase: "incomplete" as const };
    }
    return seg;
  });
  return changed ? next : segments;
}

export default function finalizeTurn(turn: Turn): Turn {
  const segments = finalizeSegments(turn.segments);
  return segments === turn.segments ? turn : { ...turn, segments };
}

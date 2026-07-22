import type { session } from "@/hooks/useApp";

// 与 Go 端 internal/agent/events.go 的 AgentEventType 枚举一一对应
export enum AgentEventType {
  Thinking = 0,
  ThinkingDone = 1,
  Content = 2,
  ToolCall = 3,
  Usage = 4,
  Error = 5,
  Retrying = 6, // 可恢复错误重试中
  Compression = 7,
}

// AgentEvent 与 Go 端 AgentEvent 的 JSON 序列化一一对应
export interface AgentEvent {
  turn_id: number;
  sub_task_id?: string;
  seq?: number;
  type: AgentEventType;
  data?: string;
  tool_name?: string;
  tool_id?: string;
  phase?: string; // "selected" | "executing" | "awaiting_approval" | "completed" | "failed" | "loop_detected"
  tool_args?: Record<string, unknown>;
  success?: boolean;
  error?: string;
  display_text?: string;
  activity_kind?: string;
  metadata?: Record<string, unknown>;
  usage?: Record<string, unknown>;
  compression_phase?: string; // "compressing" | "done"
  summary?: string;
  attempt?: number; // EventRetrying 时：第几次重试（1-indexed）
  max_retries?: number; // EventRetrying 时：最大重试次数
  backoff_ms?: number; // EventRetrying 时：本次退避毫秒数
  clear_from_seq?: number; // EventRetrying 时：本轮 streamLoop 起点事件 seq（前端据此清空本轮 partial segments）
  timestamp: string;
}

// TurnSegment 是 turn 内的一个片段：文本块、工具调用、子 Agent 或压缩标记
export interface TurnSegment {
  id: string;
  type: "text" | "tool" | "subagent" | "compression";
  content: string;
  thinkingContent: string;
  thinkingDone: boolean;
  isStreaming: boolean;
  // tool
  toolName: string;
  toolId: string;
  toolStatus: "executing" | "awaiting_approval" | "completed" | "failed";
  displayText: string;
  activityKind: string;
  error: string;
  // approval
  approvalType?: string;
  approvalPayload?: Record<string, unknown>;
  // subagent
  status?: "streaming" | "done" | "failed";
  agentType?: "memory" | "review";
  taskId?: string;
  segments?: TurnSegment[];
  finalText?: string;
  // compression
  compressionPhase?: "compressing" | "done";
  // web_search / web_fetch 的富文本结果
  result?: Record<string, unknown>;
  // P2: 创建该 segment 时的事件 seq（实时流式用，>= 1）
  // rebuildTurns 创建的历史 segment 默认 0，永远不会被 EventRetrying 清空（clear_from_seq >= 1）
  firstSeq: number;
  // P1: 子 agent 重试状态（主 turn 用 Turn.retrying，子 agent segment 用此字段）
  // 仅 type='subagent' 的 segment 会用到，由 EventRetrying 携带 sub_task_id 时设置
  retrying?: {
    attempt: number;
    maxRetries: number;
    errorMessage: string;
  } | null;
  // P1: 子 agent 最终失败原因（由 EventError 携带 sub_task_id 时设置）
  // 与 retrying.errorMessage 的区别：retrying=null 后丢失，errorMessage 持久保留
  errorMessage?: string;
}

export function emptySegment(id: string): TurnSegment {
  return {
    id,
    type: "text",
    content: "",
    thinkingContent: "",
    thinkingDone: false,
    isStreaming: false,
    toolName: "",
    toolId: "",
    toolStatus: "executing",
    displayText: "",
    activityKind: "",
    error: "",
    firstSeq: 0,
  };
}

// filterSegmentsBySeq 清空本轮 streamLoop 已渲染的 partial segments。
// 规则：保留 firstSeq < clearFromSeq 的 segments（历史 + 前面轮）。
// - 历史 segments（rebuildTurns 创建）firstSeq=0，clearFromSeq >= 1，永远保留
// - 本轮 segments firstSeq >= clearFromSeq，被清空
// 后端 emit EventRetrying 时附带 clear_from_seq = streamStartSeq = *eventSeq + 1
export function filterSegmentsBySeq(
  segments: TurnSegment[],
  clearFromSeq: number,
): TurnSegment[] {
  return segments.filter((s) => s.firstSeq < clearFromSeq);
}

// Turn 是一次对话轮次：用户消息 + AI 回复的 segments
export interface Turn {
  id: string;
  turnId: number;
  userMessage: string;
  segments: TurnSegment[];
  status: "streaming" | "done" | "failed" | "interrupted" | "stopped";
  errorMessage?: string;
  compressionOnly?: boolean; // 纯压缩 turn（手动压缩），无用户消息
  retrying?: {
    // P2: 可恢复错误重试状态
    attempt: number; // 第几次重试（1-indexed）
    maxRetries: number; // 最大重试次数
    errorMessage: string; // 触发重试的错误信息
  } | null;
}

export function rebuildTurns(messages: session.Message[]): Turn[] {
  const turns: Turn[] = [];
  let current: Turn | null = null;
  let segCounter = 0;
  const subagentCache = new Map<string, TurnSegment>();

  for (const msg of messages) {
    // 中断标记：根据 event_type 区分用户停止和系统中断
    if (
      msg.event_type === "user_stopped" ||
      msg.event_type === "system_interrupted" ||
      msg.event_type === "error"
    ) {
      const target = turns.find((t) => t.turnId === msg.turn_id);
      if (target) {
        if (msg.event_type === "user_stopped") target.status = "stopped";
        else if (msg.event_type === "error") target.status = "failed";
        else target.status = "interrupted";
      }
      continue;
    }

    // 压缩边界标记：独立成一个纯压缩 turn
    if (msg.event_type === "compression") {
      if (msg.agent_type !== "main" && msg.sub_task_id) {
        const cached = subagentCache.get(msg.sub_task_id);
        if (cached && cached.segments) {
          cached.segments.push({
            ...emptySegment(`seg_${segCounter++}`),
            type: "compression",
            compressionPhase: "done",
          });
        }
        continue;
      }
      if (current && current.turnId === msg.turn_id) {
        current.segments.push({
          ...emptySegment(`seg_${segCounter++}`),
          type: "compression",
          compressionPhase: "done",
        });
        continue;
      }
      turns.push({
        id: `comp_${msg.turn_id}`,
        turnId: msg.turn_id,
        userMessage: "",
        segments: [
          {
            ...emptySegment(`comp_${msg.turn_id}`),
            type: "compression",
            compressionPhase: "done",
          },
        ],
        status: "done",
        compressionOnly: true,
      });
      continue;
    }

    if (msg.role === "user") {
      current = {
        id: `hist_${msg.turn_id}`,
        turnId: msg.turn_id,
        userMessage: msg.content,
        segments: [],
        status: "done",
      };
      turns.push(current);
    } else if (msg.role === "assistant") {
      // 子 Agent 消息：agent_type !== 'main' 且有 sub_task_id
      if (msg.agent_type !== "main" && msg.sub_task_id && current) {
        const subTaskId = msg.sub_task_id;
        const cached = subagentCache.get(subTaskId);
        const subSeg: TurnSegment =
          cached ??
          (() => {
            const seg: TurnSegment = {
              ...emptySegment(`seg_${segCounter++}`),
              type: "subagent",
              status: "done",
              agentType: (msg.agent_type as "memory" | "review") || "memory",
              taskId: subTaskId,
              segments: [],
              finalText: "",
            };
            // 不直接 push——等遇到 run_subagent tool_display 时插到正确位置
            subagentCache.set(subTaskId, seg);
            return seg;
          })();

        // 追加子 agent 的文本内容
        if ((msg.content || msg.thinking_content) && subSeg.segments) {
          subSeg.segments.push({
            ...emptySegment(`seg_${segCounter++}`),
            type: "text",
            content: msg.content || "",
            thinkingContent: msg.thinking_content || "",
            thinkingDone: true,
            isStreaming: false,
          });
          if (msg.content) {
            subSeg.finalText =
              subSeg.finalText || ""
                ? subSeg.finalText + "\n" + msg.content
                : msg.content;
          }
        }

        // 子 agent 的工具调用
        const toolDisplays = parseToolDisplays(msg.extra_metadata);
        if (toolDisplays.length > 0 && subSeg.segments) {
          for (const td of toolDisplays) {
            const phase = td.phase as
              "completed" | "failed" | "executing" | undefined;
            subSeg.segments.push({
              ...emptySegment(`seg_${segCounter++}`),
              type: "tool",
              toolName: td.tool_name,
              toolId: td.tool_id,
              toolStatus:
                phase &&
                (phase === "executing" ||
                  phase === "completed" ||
                  phase === "failed")
                  ? phase
                  : "completed",
              displayText: td.display_text,
              activityKind: td.activity_kind,
              error: td.error || "",
            });
          }
        }
        continue;
      }

      // 主 Agent 消息
      if (!current) continue;

      const thinkingContent = msg.thinking_content || "";

      // 文本内容
      if (msg.content || thinkingContent) {
        current.segments.push({
          ...emptySegment(`seg_${segCounter++}`),
          type: "text",
          content: msg.content || "",
          thinkingContent,
          thinkingDone: true,
          isStreaming: false,
        });
      }

      // 工具展示信息
      const toolDisplays = parseToolDisplays(msg.extra_metadata);
      for (const td of toolDisplays) {
        // run_subagent：在此位置插入子 Agent 卡片
        if (td.tool_name === "run_subagent") {
          const cached = subagentCache.get(td.tool_id);
          if (cached) {
            // P1: 从 tool_display 的 phase 修正 subagent segment 的 status
            // rebuildTurns 创建 subagent segment 时默认 status='done'，
            // 这里根据 run_subagent 的 phase 修正为 failed（历史回放保留失败状态）
            if (td.phase === "failed") {
              cached.status = "failed";
            }
            // 从 tool_display.error 恢复 errorMessage（历史回放）
            if (td.error) {
              cached.errorMessage = td.error;
            }
            current.segments.push(cached);
          }
          continue;
        }
        current.segments.push({
          ...emptySegment(`seg_${segCounter++}`),
          type: "tool",
          toolName: td.tool_name,
          toolId: td.tool_id,
          toolStatus:
            td.phase === "completed" ||
            td.phase === "failed" ||
            td.phase === "executing"
              ? td.phase
              : "completed",
          displayText: td.display_text || td.tool_name,
          activityKind: td.activity_kind || "",
          error: td.error || "",
          result: td.result,
        });
      }
    }
  }

  return turns;
}

interface ToolDisplay {
  tool_id: string;
  tool_name: string;
  display_text: string;
  activity_kind: string;
  phase: string;
  result?: Record<string, unknown>;
  error?: string;
}

function parseToolDisplays(extraMetadata?: string): ToolDisplay[] {
  if (!extraMetadata) return [];
  try {
    const meta = JSON.parse(extraMetadata);
    if (meta.tool_displays && Array.isArray(meta.tool_displays)) {
      return meta.tool_displays as ToolDisplay[];
    }
    return [];
  } catch {
    return [];
  }
}

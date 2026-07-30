import { useState, useCallback, useRef, useEffect } from "react";
import { useTranslation } from "react-i18next";
import { MessageSquare, Loader2, History, Plus } from "lucide-react";
import { EventsOn } from "@/lib/wailsjs/runtime/runtime";
import { useApp } from "@/hooks/useApp";
import type { llm, app } from "@/hooks/useApp";
import type { AgentEvent, Turn } from "./types";
import {
  AgentEventType,
  emptySegment,
  filterSegmentsBySeq,
  rebuildTurns,
} from "./types";
import ChatInput from "./ChatInput";
import ChatControls from "./ChatControls";
import MessageBubble from "./MessageBubble";
import ThinkingBlock from "./ThinkingBlock";
import ToolCallCard from "./ToolCallCard";
import WebSearchCard from "./WebSearchCard";
import WebFetchCard from "./WebFetchCard";
import SubagentCard from "./SubagentCard";
import CompressionBlock from "./CompressionBlock";
import type { UsageInfo } from "./ContextRing";
import SettingsDialog from "@/components/settings/SettingsDialog";
import RecentSessions from "./RecentSessions";
import SessionHistory from "./SessionHistory";
import { toastError } from "@/utils/toast";
import { toErrorMessage } from "@/utils/error";

interface Props {
  novelId: number;
  onApprove: (toolId: string, feedback: string) => Promise<void>;
  onReject: (toolId: string, feedback: string) => Promise<void>;
  onApprovalFileEdit?: (payload: {
    path: string;
    title: string;
    diff: string;
    original: string;
    modified: string;
    changeType: string;
    reason: string;
    toolId: string;
  }) => void;
  chatPanelWidth: number;
  onChatPanelResize: (w: number) => void;
}
const EVENT_REORDER_TIMEOUT = 120;

interface EventQueue {
  nextSeq: number;
  pending: Map<number, AgentEvent>;
  flushTimer: ReturnType<typeof setTimeout> | null;
}

interface ChatStartedEvent {
  session_id?: string;
  turn_id: number;
}

export default function ChatPanel({
  novelId,
  onApprove,
  onReject,
  onApprovalFileEdit,
  chatPanelWidth,
  onChatPanelResize,
}: Props) {
  const { t } = useTranslation();
  const app = useApp();
  const [isDragging, setIsDragging] = useState(false);
  const startXRef = useRef(0);
  const startWidthRef = useRef(chatPanelWidth);
  const [turns, setTurns] = useState<Turn[]>([]);
  const [sessionId, setSessionId] = useState("");
  const [isLoading, setIsLoading] = useState(false);
  const [models, setModels] = useState<llm.AvailableModel[]>([]);
  const [selectedKey, setSelectedKey] = useState("");
  const [reasoningEffort, setReasoningEffort] = useState("");
  const [approvalMode, setApprovalMode] = useState<"manual" | "auto">("manual");
  const [lastUsage, setLastUsage] = useState<UsageInfo | null>(null);
  const [isCompressing, setIsCompressing] = useState(false);
  const compressingRef = useRef(false);
  const activeCountRef = useRef(0);
  const [showSettings, setShowSettings] = useState(false);
  const [activeSessionId, setActiveSessionId] = useState<
    string | null | undefined
  >(undefined);
  const [sessions, setSessions] = useState<app.SessionMeta[]>([]);
  const [sessionsTotal, setSessionsTotal] = useState(0);
  const [showHistoryPanel, setShowHistoryPanel] = useState(false);
  const [isLoadingHistory, setIsLoadingHistory] = useState(false);
  const [initLoadError, setInitLoadError] = useState(false);
  const [initLoadRetry, setInitLoadRetry] = useState(0);
  const [historyLoadError, setHistoryLoadError] = useState(false);
  const [historyLoadRetry, setHistoryLoadRetry] = useState(0);
  const [slashCommands, setSlashCommands] = useState<app.SlashCommand[]>([]);
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const scrollContainerRef = useRef<HTMLDivElement>(null);
  const isNearBottomRef = useRef(true);
  const counterRef = useRef(0);
  const startedUnsubRef = useRef<(() => void) | null>(null);
  const agentUnsubRef = useRef<(() => void) | null>(null);
  const eventQueuesRef = useRef<Map<number, EventQueue>>(new Map());
  const onApprovalFileEditRef = useRef(onApprovalFileEdit);
  useEffect(() => {
    onApprovalFileEditRef.current = onApprovalFileEdit;
  }, [onApprovalFileEdit]);
  const lastSessionIdRef = useRef("");

  // 加载模型列表并恢复持久化设置
  useEffect(() => {
    setInitLoadError(false);
    Promise.all([app.GetModels(), app.GetSettings()])
      .then(([modelList, settings]) => {
        if (modelList && modelList.length > 0) {
          setModels(modelList);

          // 恢复模型选择（验证 key 仍存在）
          let key = settings?.selected_model_key || "";
          let model = modelList.find((m) => m.Key === key);
          if (!model) {
            model = modelList[0];
            key = model.Key;
          }
          setSelectedKey(key);

          // 恢复推理程度（验证级别仍合法）
          let effort = settings?.reasoning_effort || "";
          if (!effort || !model.ReasoningLevels?.includes(effort)) {
            effort = model.ReasoningLevels?.[0] || "";
          }
          setReasoningEffort(effort);
        }

        // 恢复审批模式
        const mode = settings?.approval_mode;
        if (mode === "manual" || mode === "auto") {
          setApprovalMode(mode);
        }

        // 暂存上次会话 ID，等 novelId 加载后恢复
        if (settings?.last_session_id) {
          lastSessionIdRef.current = settings.last_session_id;
        }
      })
      .catch((err) => {
        console.error("Load models/settings failed", err);
        setInitLoadError(true);
      });
  }, [app, initLoadRetry]);

  // 加载会话列表
  useEffect(() => {
    if (!novelId) return;
    setActiveSessionId(undefined);
    setTurns([]);
    setSessionId("");
    app
      .GetSessions({ novel_id: novelId, page: 1, size: 5, search: "" })
      .then((r) => {
        if (r) {
          setSessions(r.items);
          setSessionsTotal(r.total);
        }
      })
      .catch((err) => {
        console.error("Load sessions failed", err);
      });

    // 尝试恢复上次活跃会话（仅恢复一次，通过 ref 标记）
    const sid = lastSessionIdRef.current;
    if (sid && novelId) {
      lastSessionIdRef.current = "";
      app
        .GetSession(sid)
        .then((detail) => {
          if (detail && detail.novel_id === novelId) {
            setActiveSessionId(sid);
          }
        })
        .catch(() => {
          app.SetLastSession("").catch(() => {});
        });
    }
  }, [app, novelId]);

  // 加载历史消息
  useEffect(() => {
    if (!activeSessionId || !novelId) return;
    setSessionId(activeSessionId);
    setHistoryLoadError(false);
    setIsLoadingHistory(true);
    app
      .GetSessionMessages(activeSessionId)
      .then((msgs) => {
        if (msgs) {
          setTurns(rebuildTurns(msgs));
        }
      })
      .catch((err) => {
        console.error("Load messages failed", err);
        setHistoryLoadError(true);
      })
      .finally(() => setIsLoadingHistory(false));
  }, [app, activeSessionId, novelId, historyLoadRetry]);

  const handleMouseDown = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault();
      setIsDragging(true);
      startXRef.current = e.clientX;
      startWidthRef.current = chatPanelWidth;
    },
    [chatPanelWidth],
  );

  useEffect(() => {
    if (!isDragging) return;
    const handleMouseMove = (e: MouseEvent) => {
      const delta = e.clientX - startXRef.current;
      onChatPanelResize(startWidthRef.current - delta);
    };
    const handleMouseUp = () => setIsDragging(false);
    document.addEventListener("mousemove", handleMouseMove);
    document.addEventListener("mouseup", handleMouseUp);
    return () => {
      document.removeEventListener("mousemove", handleMouseMove);
      document.removeEventListener("mouseup", handleMouseUp);
    };
  }, [isDragging, onChatPanelResize]);

  // 清理事件监听器
  useEffect(() => {
    const eventQueues = eventQueuesRef.current;
    return () => {
      startedUnsubRef.current?.();
      agentUnsubRef.current?.();
      eventQueues.forEach((queue) => {
        if (queue.flushTimer) clearTimeout(queue.flushTimer);
      });
      eventQueues.clear();
    };
  }, []);

  // 流式输出时自动滚到底部，但仅在用户未主动上滚时
  useEffect(() => {
    if (isNearBottomRef.current) {
      messagesEndRef.current?.scrollIntoView({ behavior: "instant" });
    }
  }, [turns]);

  const handleMessagesScroll = useCallback(() => {
    const el = scrollContainerRef.current;
    if (!el) return;
    isNearBottomRef.current =
      el.scrollHeight - el.scrollTop - el.clientHeight < 60;
  }, []);

  const handleSelectSession = useCallback(
    (sid: string) => {
      setActiveSessionId(sid);
      app.SetLastSession(sid).catch(() => {});
      app
        .GetSession(sid)
        .then((detail) => {
          if (detail?.usage) {
            setLastUsage(detail.usage as unknown as UsageInfo);
          } else {
            setLastUsage(null);
          }
        })
        .catch(() => setLastUsage(null));
    },
    [app],
  );

  const handleNewChat = useCallback(() => {
    setActiveSessionId(null);
    setTurns([]);
    setSessionId("");
    setLastUsage(null);
    app
      .GetSessions({ novel_id: novelId, page: 1, size: 5, search: "" })
      .then((r) => {
        if (r) {
          setSessions(r.items);
          setSessionsTotal(r.total);
        }
      })
      .catch((err) => {
        console.error("Refresh sessions failed", err);
      });
  }, [novelId, app]);

  // 会话删除后同步视图：从最近会话列表移除并更新总数；
  // 若删除的正是当前正在查看的 session，清空活跃会话/消息，避免界面仍停留在已删除内容上。
  const handleSessionDeleted = useCallback(
    (sessionId: string) => {
      setSessions((prev) => prev.filter((s) => s.session_id !== sessionId));
      setSessionsTotal((prev) => Math.max(0, prev - 1));
      if (activeSessionId === sessionId) {
        setActiveSessionId(null);
        setTurns([]);
        setSessionId("");
      }
    },
    [activeSessionId],
  );

  const handleOpenHistory = useCallback(() => {
    setShowHistoryPanel(true);
  }, []);

  const handleCloseHistory = useCallback(() => {
    setShowHistoryPanel(false);
  }, []);

  const loadSlash = useCallback(async () => {
    if (!novelId) {
      setSlashCommands([]);
      return;
    }
    try {
      const list = await app.ListSlashCommands({ novel_id: novelId });
      setSlashCommands(list ?? []);
    } catch (err) {
      console.error("Load slash commands failed", err);
    }
  }, [app, novelId]);

  useEffect(() => {
    loadSlash();
  }, [loadSlash]);

  const applyAgentEvent = useCallback(
    (turnId: number, event: AgentEvent) => {
      // P2: 流恢复事件清空 retrying 状态（agent 重试 LLM 调用成功）
      if (
        event.type === AgentEventType.Thinking ||
        event.type === AgentEventType.ThinkingDone ||
        event.type === AgentEventType.Content ||
        event.type === AgentEventType.ToolCall
      ) {
        setTurns((prev) =>
          prev.map((turn) => {
            if (turn.turnId !== turnId) return turn;
            // P1: 子 agent 重试后流恢复，清空对应 subagent segment 的 retrying
            if (event.sub_task_id) {
              const subIdx = turn.segments.findIndex(
                (s) => s.type === "subagent" && s.taskId === event.sub_task_id,
              );
              if (subIdx >= 0 && turn.segments[subIdx].retrying) {
                const newSegs = [...turn.segments];
                newSegs[subIdx] = { ...turn.segments[subIdx], retrying: null };
                return { ...turn, segments: newSegs };
              }
              return turn;
            }
            // 主 turn 重试后流恢复
            return turn.retrying ? { ...turn, retrying: null } : turn;
          }),
        );
      }
      switch (event.type) {
        case AgentEventType.Usage: {
          if (event.usage) {
            setLastUsage(event.usage as unknown as UsageInfo);
          }
          return;
        }
        case AgentEventType.Error: {
          // P1: 子 agent 失败时按 sub_task_id 路由到对应 subagent segment
          // 后端 RunSubAgent 复用父 turn 的 TurnID+EventSeq，子 agent 失败触发的 EventError
          // 会冒泡到主 turn。前端必须按 sub_task_id 区分，否则主 turn 误设为 failed。
          if (event.sub_task_id) {
            setTurns((prev) =>
              prev.map((turn) => {
                if (turn.turnId !== turnId) return turn;
                const subIdx = turn.segments.findIndex(
                  (s) =>
                    s.type === "subagent" && s.taskId === event.sub_task_id,
                );
                if (subIdx < 0) return turn;
                const subSeg = { ...turn.segments[subIdx] };
                subSeg.status = "failed";
                subSeg.errorMessage = event.error || t("chat.chatError");
                subSeg.retrying = null;
                const newSegs = [...turn.segments];
                newSegs[subIdx] = subSeg;
                return { ...turn, segments: newSegs };
              }),
            );
            return;
          }
          // 主 turn 失败
          setTurns((prev) =>
            prev.map((turn) =>
              turn.turnId === turnId
                ? {
                    ...turn,
                    status: "failed" as const,
                    errorMessage: event.error || t("chat.chatError"),
                    retrying: null,
                  }
                : turn,
            ),
          );
          return;
        }
        case AgentEventType.Retrying: {
          // P2: agent 层正在重试 LLM 调用，显示"重试 x/y"
          // 同步清空本轮 streamLoop 已渲染的 partial segments（保留历史 segments firstSeq=0）
          const clearFromSeq = event.clear_from_seq ?? 0;
          // P1: 子 agent 重试时按 sub_task_id 路由到对应 subagent segment
          // 后端 RunSubAgent 复用父 turn 的 TurnID+EventSeq，子 agent 重试触发的 EventRetrying
          // 会冒泡到主 turn。前端必须按 sub_task_id 区分，否则主 turn 误显示 banner，
          // 且子 agent 内 partial 不会被清空（bug 复现）。
          if (event.sub_task_id) {
            setTurns((prev) =>
              prev.map((turn) => {
                if (turn.turnId !== turnId) return turn;
                const subIdx = turn.segments.findIndex(
                  (s) =>
                    s.type === "subagent" && s.taskId === event.sub_task_id,
                );
                if (subIdx < 0) return turn;
                const subSeg = { ...turn.segments[subIdx] };
                subSeg.segments = filterSegmentsBySeq(
                  subSeg.segments || [],
                  clearFromSeq,
                );
                subSeg.retrying = {
                  attempt: event.attempt || 0,
                  maxRetries: event.max_retries || 0,
                  errorMessage: event.error || "",
                };
                const newSegs = [...turn.segments];
                newSegs[subIdx] = subSeg;
                return { ...turn, segments: newSegs };
              }),
            );
            return;
          }
          setTurns((prev) =>
            prev.map((turn) =>
              turn.turnId === turnId
                ? {
                    ...turn,
                    status: "streaming" as const,
                    segments: filterSegmentsBySeq(turn.segments, clearFromSeq),
                    retrying: {
                      attempt: event.attempt || 0,
                      maxRetries: event.max_retries || 0,
                      errorMessage: event.error || "",
                    },
                  }
                : turn,
            ),
          );
          return;
        }
        case AgentEventType.Compression: {
          const phase = (event.compression_phase || "started") as
            "compressing" | "done";
          if (event.sub_task_id) {
            setTurns((prev) =>
              prev.map((turn) => {
                if (turn.turnId !== turnId) return turn;
                const subIdx = turn.segments.findIndex(
                  (s) =>
                    s.type === "subagent" && s.taskId === event.sub_task_id,
                );
                if (subIdx < 0) {
                  return {
                    ...turn,
                    segments: [
                      ...turn.segments,
                      {
                        ...emptySegment(`subagent_${event.sub_task_id}`),
                        type: "subagent",
                        status: "streaming",
                        agentType: "review" as const,
                        taskId: event.sub_task_id,
                        segments: [
                          {
                            ...emptySegment(`comp_${++counterRef.current}`),
                            type: "compression",
                            compressionPhase: phase,
                          },
                        ],
                      },
                    ],
                  };
                }
                const subSeg = { ...turn.segments[subIdx] };
                if (!subSeg.segments) subSeg.segments = [];
                const subSegs = [...subSeg.segments];
                const compIdx = subSegs.findIndex(
                  (s) => s.type === "compression",
                );
                if (compIdx >= 0) {
                  subSegs[compIdx] = {
                    ...subSegs[compIdx],
                    compressionPhase: phase,
                  };
                } else {
                  subSegs.push({
                    ...emptySegment(`comp_${++counterRef.current}`),
                    type: "compression",
                    compressionPhase: phase,
                  });
                }
                subSeg.segments = subSegs;
                const newSegs = [...turn.segments];
                newSegs[subIdx] = subSeg;
                return { ...turn, segments: newSegs };
              }),
            );
            return;
          }
          setTurns((prev) =>
            prev.map((turn) => {
              if (turn.turnId !== turnId) return turn;
              const compIdx = turn.segments.findIndex(
                (s) => s.type === "compression",
              );
              if (compIdx >= 0) {
                const segs = [...turn.segments];
                segs[compIdx] = { ...segs[compIdx], compressionPhase: phase };
                return { ...turn, segments: segs };
              }
              return {
                ...turn,
                segments: [
                  ...turn.segments,
                  {
                    ...emptySegment(`comp_${++counterRef.current}`),
                    type: "compression" as const,
                    compressionPhase: phase,
                  },
                ],
              };
            }),
          );
          return;
        }
      }

      setTurns((prev) =>
        prev.map((turn) => {
          if (turn.turnId !== turnId) return turn;

          // 子 Agent 事件：按 sub_task_id 路由到对应 SubagentSegment
          if (event.sub_task_id) {
            let subIdx = turn.segments.findIndex(
              (s) => s.type === "subagent" && s.taskId === event.sub_task_id,
            );
            let updatedSegments = turn.segments;
            if (subIdx < 0) {
              // run_subagent 的 ToolCall 事件还没 apply，子 Agent 事件先到了——就地创建
              const newSeg = {
                ...emptySegment(`subagent_${event.sub_task_id}`),
                type: "subagent" as const,
                status: "streaming" as const,
                agentType: "memory" as const,
                taskId: event.sub_task_id,
                segments: [],
                finalText: "",
                toolStatus: "executing" as const,
                firstSeq: event.seq ?? 0,
              };
              updatedSegments = [...turn.segments, newSeg];
              subIdx = updatedSegments.length - 1;
            }
            const subSeg = { ...updatedSegments[subIdx] };
            if (!subSeg.segments) subSeg.segments = [];
            const subSegs = [...subSeg.segments];
            const subSegId = `subseg_${++counterRef.current}`;

            switch (event.type) {
              case AgentEventType.Thinking: {
                const chunk = event.data || "";
                const last = subSegs[subSegs.length - 1];
                if (last && last.type === "text" && last.isStreaming) {
                  subSegs[subSegs.length - 1] = {
                    ...last,
                    thinkingContent: last.thinkingContent + chunk,
                  };
                } else {
                  subSegs.push({
                    ...emptySegment(subSegId),
                    thinkingContent: chunk,
                    thinkingDone: false,
                    isStreaming: true,
                    firstSeq: event.seq ?? 0,
                  });
                }
                break;
              }
              case AgentEventType.ThinkingDone: {
                for (let i = 0; i < subSegs.length; i++) {
                  if (subSegs[i].type === "text" && !subSegs[i].thinkingDone) {
                    subSegs[i] = {
                      ...subSegs[i],
                      thinkingDone: true,
                      isStreaming: false,
                    };
                  }
                }
                break;
              }
              case AgentEventType.Content: {
                const chunk = event.data || "";
                const last = subSegs[subSegs.length - 1];
                if (last && last.type === "text" && last.isStreaming) {
                  subSegs[subSegs.length - 1] = {
                    ...last,
                    content: last.content + chunk,
                    thinkingDone: true,
                  };
                } else {
                  subSegs.push({
                    ...emptySegment(subSegId),
                    content: chunk,
                    thinkingDone: true,
                    isStreaming: true,
                    firstSeq: event.seq ?? 0,
                  });
                }
                break;
              }
              case AgentEventType.ToolCall: {
                const subToolStatus =
                  event.phase === "completed"
                    ? ("completed" as const)
                    : event.phase === "failed"
                      ? ("failed" as const)
                      : ("executing" as const);
                const stIdx = subSegs.findIndex(
                  (s) => s.type === "tool" && s.toolId === event.tool_id,
                );
                if (stIdx >= 0) {
                  subSegs[stIdx] = {
                    ...subSegs[stIdx],
                    toolStatus: subToolStatus,
                    displayText:
                      event.display_text || subSegs[stIdx].displayText,
                    activityKind: event.activity_kind || "",
                    error: event.error || "",
                  };
                } else {
                  subSegs.push({
                    ...emptySegment(subSegId),
                    type: "tool",
                    toolName: event.tool_name || "",
                    toolId: event.tool_id || "",
                    toolStatus: subToolStatus,
                    displayText: event.display_text || event.tool_name || "",
                    activityKind: event.activity_kind || "",
                    error: event.error || "",
                    firstSeq: event.seq ?? 0,
                  });
                }
                break;
              }
              default:
                break;
            }

            subSeg.segments = subSegs;
            const newSegs = [...updatedSegments];
            newSegs[subIdx] = subSeg;
            return { ...turn, segments: newSegs };
          }

          const segments = [...turn.segments];
          const segId = `seg_${++counterRef.current}`;

          switch (event.type) {
            case AgentEventType.Thinking: {
              const chunk = event.data || "";
              const lastSeg = segments[segments.length - 1];
              if (lastSeg && lastSeg.type === "text" && lastSeg.isStreaming) {
                segments[segments.length - 1] = {
                  ...lastSeg,
                  thinkingContent: lastSeg.thinkingContent + chunk,
                };
              } else {
                segments.push({
                  ...emptySegment(segId),
                  thinkingContent: chunk,
                  thinkingDone: false,
                  isStreaming: true,
                  firstSeq: event.seq ?? 0,
                });
              }
              return { ...turn, segments };
            }

            case AgentEventType.ThinkingDone: {
              return {
                ...turn,
                segments: segments.map((seg) =>
                  seg.type === "text" && !seg.thinkingDone
                    ? { ...seg, thinkingDone: true, isStreaming: false }
                    : seg,
                ),
              };
            }

            case AgentEventType.Content: {
              const chunk = event.data || "";
              const lastSeg = segments[segments.length - 1];
              if (lastSeg && lastSeg.type === "text" && lastSeg.isStreaming) {
                segments[segments.length - 1] = {
                  ...lastSeg,
                  content: lastSeg.content + chunk,
                  thinkingDone: true,
                };
              } else {
                segments.push({
                  ...emptySegment(segId),
                  content: chunk,
                  thinkingDone: true,
                  isStreaming: true,
                  firstSeq: event.seq ?? 0,
                });
              }
              return { ...turn, segments };
            }

            case AgentEventType.ToolCall: {
              const isSubagent = event.tool_name === "run_subagent";
              const toolStatus =
                event.phase === "awaiting_approval"
                  ? ("awaiting_approval" as const)
                  : event.phase === "completed"
                    ? ("completed" as const)
                    : event.phase === "failed"
                      ? ("failed" as const)
                      : ("executing" as const);

              // run_subagent：维护对应的 subagent segment
              if (isSubagent) {
                const agentType =
                  (event.metadata?.agent_type as "memory" | "review") ||
                  "memory";
                const toolId = event.tool_id || "";
                const subIdx = segments.findIndex(
                  (seg) => seg.type === "subagent" && seg.taskId === toolId,
                );
                if (subIdx >= 0) {
                  segments[subIdx] = {
                    ...segments[subIdx],
                    agentType,
                    status:
                      toolStatus === "executing"
                        ? "streaming"
                        : toolStatus === "failed"
                          ? "failed"
                          : "done",
                    toolStatus,
                    errorMessage:
                      toolStatus === "failed"
                        ? event.error || ""
                        : segments[subIdx].errorMessage,
                  };
                } else {
                  segments.push({
                    ...emptySegment(`subagent_${toolId || segId}`),
                    type: "subagent",
                    status: "streaming",
                    agentType,
                    taskId: toolId,
                    segments: [],
                    finalText: "",
                    toolStatus: "executing",
                    firstSeq: event.seq ?? 0,
                  });
                }
                // 移除同 toolId 的 tool segment（可能由空 toolName 的早期事件误创建）
                const cleanSegs = toolId
                  ? segments.filter(
                      (seg) => !(seg.type === "tool" && seg.toolId === toolId),
                    )
                  : segments;
                return { ...turn, segments: cleanSegs };
              }

              const idx = segments.findIndex(
                (seg) =>
                  seg.type === "tool" &&
                  event.tool_id &&
                  seg.toolId === event.tool_id,
              );

              const approvalType =
                toolStatus === "awaiting_approval"
                  ? (event.metadata?.approval_type as string | undefined)
                  : undefined;
              const approvalPayload =
                toolStatus === "awaiting_approval"
                  ? (event.metadata?.payload as
                      Record<string, unknown> | undefined)
                  : undefined;

              if (idx >= 0) {
                segments[idx] = {
                  ...segments[idx],
                  toolName: event.tool_name || segments[idx].toolName,
                  toolId: event.tool_id || segments[idx].toolId,
                  toolStatus,
                  displayText: event.display_text || segments[idx].displayText,
                  activityKind:
                    event.activity_kind || segments[idx].activityKind || "",
                  error: event.error || "",
                  approvalType: approvalType ?? segments[idx].approvalType,
                  approvalPayload:
                    approvalPayload ?? segments[idx].approvalPayload,
                  result:
                    toolStatus === "completed"
                      ? event.metadata || segments[idx].result
                      : segments[idx].result,
                };
              } else {
                segments.push({
                  ...emptySegment(segId),
                  type: "tool",
                  toolName: event.tool_name || "",
                  toolId: event.tool_id || "",
                  toolStatus,
                  displayText: event.display_text || event.tool_name || "",
                  activityKind: event.activity_kind || "",
                  error: event.error || "",
                  approvalType,
                  approvalPayload,
                  result:
                    toolStatus === "completed" ? event.metadata : undefined,
                  firstSeq: event.seq ?? 0,
                });
              }

              // 文件编辑审批 → 通知 ContentPanel 打开 diff 标签页
              if (
                toolStatus === "awaiting_approval" &&
                approvalType === "file_edit" &&
                approvalPayload
              ) {
                const p = approvalPayload;
                const path = (p.path as string) || "";
                let title = `diff: ${path}`;
                if (path.startsWith("chapters/")) {
                  const num = path.replace("chapters/", "").replace(".md", "");
                  title = `diff: ${t("chat.diffChapter", { n: parseInt(num) })}`;
                } else if (path === "goink.md") {
                  title = `diff: ${t("chat.diffStoryStatus")}`;
                } else if (path.startsWith("outlines/")) {
                  const num = path.replace("outlines/", "").replace(".md", "");
                  title = `diff: ${t("chat.diffChapterOutline", { n: parseInt(num) })}`;
                }
                onApprovalFileEditRef.current?.({
                  path,
                  title,
                  diff: "",
                  original: (p.original as string) || "",
                  modified: (p.modified as string) || "",
                  changeType: (p.change_type as string) || "",
                  reason: (p.reason as string) || "",
                  toolId: (event.tool_id as string) || "",
                });
              }

              return { ...turn, segments };
            }

            default:
              return turn;
          }
        }),
      );
    },
    [t],
  );

  const flushEventQueue = useCallback(
    (turnId: number, force = false) => {
      const queue = eventQueuesRef.current.get(turnId);
      if (!queue) return;

      let event = queue.pending.get(queue.nextSeq);
      while (event) {
        queue.pending.delete(queue.nextSeq);
        queue.nextSeq += 1;
        applyAgentEvent(turnId, event);
        event = queue.pending.get(queue.nextSeq);
      }

      if (force && queue.pending.size > 0) {
        const orderedEvents = [...queue.pending.entries()].sort(
          ([a], [b]) => a - b,
        );
        queue.pending.clear();

        for (const [seq, queuedEvent] of orderedEvents) {
          if (seq >= queue.nextSeq) {
            queue.nextSeq = seq + 1;
            applyAgentEvent(turnId, queuedEvent);
          }
        }
      }

      if (queue.pending.size === 0 && queue.flushTimer) {
        clearTimeout(queue.flushTimer);
        queue.flushTimer = null;
      }
    },
    [applyAgentEvent],
  );

  const handleAgentEvent = useCallback(
    (turnId: number) => (event: AgentEvent) => {
      if (!event.seq) {
        applyAgentEvent(turnId, event);
        return;
      }

      let queue = eventQueuesRef.current.get(turnId);
      if (!queue) {
        queue = {
          nextSeq: 1,
          pending: new Map<number, AgentEvent>(),
          flushTimer: null,
        };
        eventQueuesRef.current.set(turnId, queue);
      }

      if (event.seq < queue.nextSeq) return;

      queue.pending.set(event.seq, event);
      flushEventQueue(turnId);

      if (queue.pending.size > 0 && !queue.flushTimer) {
        queue.flushTimer = setTimeout(() => {
          queue.flushTimer = null;
          flushEventQueue(turnId, true);
        }, EVENT_REORDER_TIMEOUT);
      }
    },
    [applyAgentEvent, flushEventQueue],
  );

  const handleConfigModel = useCallback(() => setShowSettings(true), []);

  const refreshModels = useCallback(() => {
    app
      .GetModels()
      .then((list) => {
        if (list && list.length > 0) setModels(list);
      })
      .catch(() => {});
  }, [app]);

  const handleSelectModel = useCallback(
    (key: string) => {
      setSelectedKey(key);
      const m = models.find((x) => x.Key === key);
      let effort = "";
      if (m?.ReasoningLevels?.length) {
        effort = m.ReasoningLevels[0];
        setReasoningEffort(effort);
      }
      app.SetSelectedModel(key, effort).catch(() => {});
    },
    [models, app],
  );

  const handleSelectEffort = useCallback(
    (effort: string) => {
      setReasoningEffort(effort);
      app.SetReasoningEffort(effort).catch(() => {});
    },
    [app],
  );

  const handleToggleApproval = useCallback(() => {
    const next = approvalMode === "manual" ? "auto" : "manual";
    setApprovalMode(next);
    app.SetApprovalMode(next).catch(() => {});
  }, [approvalMode, app]);

  const handleCompress = useCallback(async () => {
    if (!sessionId || !selectedKey || compressingRef.current) return;
    const [providerName, modelID] = selectedKey.split("/");
    if (!providerName || !modelID) return;

    compressingRef.current = true;
    setIsCompressing(true);
    // 创建压缩中 turn（用于动画展示）
    const compTurnId = `comp_${++counterRef.current}`;
    const compressingTurn: Turn = {
      id: compTurnId,
      turnId: 0,
      userMessage: "",
      segments: [
        {
          ...emptySegment(compTurnId),
          type: "compression" as const,
          compressionPhase: "compressing" as const,
        },
      ],
      status: "done" as const,
      compressionOnly: true,
    };
    setTurns((prev) => [...prev, compressingTurn]);

    try {
      const result = await app.CompressContext({
        session_id: sessionId,
        provider_name: providerName,
        model_id: modelID,
      });
      // 更新：回填真实 turnId + 完成状态
      setTurns((prev) =>
        prev.map((t) => {
          if (t.id === compTurnId) {
            return {
              ...t,
              turnId: result.turn_id,
              segments: t.segments.map((s) =>
                s.type === "compression"
                  ? { ...s, compressionPhase: "done" as const }
                  : s,
              ),
            };
          }
          return t;
        }),
      );
    } catch (err) {
      // 压缩失败，移除 compressing turn
      setTurns((prev) => prev.filter((t) => t.id !== compTurnId));
      toastError(toErrorMessage(err, t("chat.compressFailed")));
    } finally {
      setIsCompressing(false);
      compressingRef.current = false;
    }
  }, [sessionId, selectedKey, app, t]);

  const handleSend = useCallback(
    async (content: string) => {
      if (!selectedKey) return;
      const [p, m] = selectedKey.split("/");
      activeCountRef.current++;
      if (activeCountRef.current > 1) {
        app.CancelChat(sessionId);
      }
      setIsLoading(true);

      const turnId = `turn_${++counterRef.current}`;
      const newTurn: Turn = {
        id: turnId,
        turnId: 0,
        userMessage: content,
        segments: [],
        status: "streaming",
      };

      // 如果是新对话，清除历史标记
      if (activeSessionId === null || activeSessionId === undefined) {
        setActiveSessionId(null);
      }

      setTurns((prev) => [...prev, newTurn]);

      // 监听 chat:started，拿到 turnId 后订阅 agent 事件流
      startedUnsubRef.current?.();
      const startedCleanup = EventsOn(
        "chat:started",
        (data: ChatStartedEvent) => {
          if (data.session_id) {
            setSessionId(data.session_id);
            setActiveSessionId(data.session_id);
            app.SetLastSession(data.session_id).catch(() => {});
          }

          // 更新 turn 的 turnId 为后端分配的真实值
          setTurns((prev) =>
            prev.map((t) =>
              t.id === turnId ? { ...t, turnId: data.turn_id } : t,
            ),
          );

          agentUnsubRef.current?.();
          const agentCleanup = EventsOn(
            `agent:${data.turn_id}`,
            handleAgentEvent(data.turn_id),
          );
          agentUnsubRef.current = agentCleanup;
        },
      );
      startedUnsubRef.current = startedCleanup;

      try {
        await app.Chat({
          session_id: sessionId,
          novel_id: novelId,
          message: content,
          provider_name: p,
          model_id: m,
          reasoning_effort: reasoningEffort,
        });
        // 刷新会话列表
        app
          .GetSessions({ novel_id: novelId, page: 1, size: 5, search: "" })
          .then((r) => {
            if (r) {
              setSessions(r.items);
              setSessionsTotal(r.total);
            }
          })
          .catch((err) => {
            console.error("Post-send refresh sessions failed", err);
          });
      } catch (err) {
        setTurns((prev) =>
          prev.map((t) => {
            if (t.id !== turnId) return t;
            if (t.status === "stopped" || t.status === "failed") return t;
            return {
              ...t,
              status: "interrupted" as const,
              errorMessage: String(err),
            };
          }),
        );
      } finally {
        eventQueuesRef.current.forEach((queue, queuedTurnId) => {
          if (queue.flushTimer) clearTimeout(queue.flushTimer);
          const orderedEvents = [...queue.pending.entries()].sort(
            ([a], [b]) => a - b,
          );
          queue.pending.clear();
          for (const [seq, queuedEvent] of orderedEvents) {
            if (seq >= queue.nextSeq) {
              queue.nextSeq = seq + 1;
              applyAgentEvent(queuedTurnId, queuedEvent);
            }
          }
        });
        eventQueuesRef.current.clear();
        setTurns((prev) =>
          prev.map((t) =>
            t.id === turnId && t.status === "streaming"
              ? {
                  ...t,
                  status: "done" as const,
                  segments: t.segments.map((seg) =>
                    seg.type === "text" ? { ...seg, isStreaming: false } : seg,
                  ),
                }
              : t,
          ),
        );
        activeCountRef.current--;
        if (activeCountRef.current === 0) {
          setIsLoading(false);
        }
        startedUnsubRef.current?.();
        startedUnsubRef.current = null;
        agentUnsubRef.current?.();
        agentUnsubRef.current = null;
      }
    },
    [
      sessionId,
      novelId,
      selectedKey,
      reasoningEffort,
      app,
      handleAgentEvent,
      applyAgentEvent,
      activeSessionId,
    ],
  );

  const hasNovel = novelId > 0;
  const hasTurns = turns.length > 0;
  const hasActiveSession =
    activeSessionId !== undefined && activeSessionId !== null;
  const showRecent = !hasActiveSession && !hasTurns && !isLoading;

  const inputPlaceholder = !hasNovel
    ? t("chat.selectNovelFirst")
    : !selectedKey
      ? t("chat.configureModelFirst")
      : t("chat.inputPlaceholder");

  return (
    <aside
      className="shrink-0 flex flex-col bg-sidebar border-l relative overflow-hidden"
      style={{ width: chatPanelWidth }}
    >
      <div
        className="absolute left-0 top-0 bottom-0 w-1 cursor-col-resize hover:bg-primary/30 transition-colors z-10 select-none"
        style={{ marginLeft: -2 }}
        onMouseDown={handleMouseDown}
      />

      <div className="px-4 py-2.5 border-b shrink-0 flex items-center justify-between select-none">
        <span className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
          {t("chat.aiChat")}
        </span>
        <div className="flex items-center gap-2">
          <button
            onClick={handleOpenHistory}
            className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
          >
            <History className="w-3.5 h-3.5" /> {t("chat.history")}
          </button>
          <button
            onClick={handleNewChat}
            className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
          >
            <Plus className="w-3.5 h-3.5" /> {t("chat.newChat")}
          </button>
        </div>
      </div>

      {initLoadError && (
        <div className="px-4 py-2 bg-danger-bg border-b border-danger-border text-xs text-red-600 flex items-center justify-between shrink-0">
          <span>{t("chat.loadSettingsFailed")}</span>
          <button
            onClick={() => setInitLoadRetry((n) => n + 1)}
            className="underline hover:text-destructive cursor-pointer"
          >
            {t("chat.retry")}
          </button>
        </div>
      )}

      <div className="absolute left-0 right-0 top-[41px] bottom-0 pointer-events-none z-30">
        <SessionHistory
          open={showHistoryPanel}
          novelId={novelId}
          onClose={handleCloseHistory}
          onSelectSession={handleSelectSession}
          onSessionDeleted={handleSessionDeleted}
        />
      </div>

      <div
        ref={scrollContainerRef}
        onScroll={handleMessagesScroll}
        className="flex-1 overflow-y-auto overscroll-contain px-3 py-3 relative"
      >
        {!hasNovel ? (
          <div className="flex items-center justify-center h-full">
            <div className="text-center">
              <MessageSquare className="w-10 h-10 text-muted-foreground/20 mx-auto mb-3" />
              <p className="text-sm text-muted-foreground">
                {t("chat.selectNovel")}
              </p>
            </div>
          </div>
        ) : showRecent ? (
          <RecentSessions
            sessions={sessions}
            total={sessionsTotal}
            onSelectSession={handleSelectSession}
            onViewAll={handleOpenHistory}
            onDeleteSession={handleSessionDeleted}
          />
        ) : isLoadingHistory ? (
          <div className="flex items-center justify-center h-full">
            <Loader2 className="w-5 h-5 animate-spin text-muted-foreground" />
          </div>
        ) : (
          <>
            {/* 消息列表 */}
            {historyLoadError ? (
              <div className="flex items-center justify-center h-full">
                <div className="text-center">
                  <p className="text-sm text-red-500 mb-2">
                    {t("chat.loadMessagesFailed")}
                  </p>
                  <button
                    onClick={() => setHistoryLoadRetry((n) => n + 1)}
                    className="text-xs text-primary underline cursor-pointer"
                  >
                    {t("chat.retry")}
                  </button>
                </div>
              </div>
            ) : !hasTurns && !isLoading ? (
              <div className="flex items-center justify-center h-full">
                <div className="text-center">
                  <MessageSquare className="w-10 h-10 text-muted-foreground/20 mx-auto mb-3" />
                  <p className="text-sm text-muted-foreground">
                    {t("chat.startConversation")}
                  </p>
                </div>
              </div>
            ) : (
              <div className="space-y-4">
                {turns.map((turn) => (
                  <div key={turn.id} className="space-y-2">
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
                          return (
                            <WebSearchCard key={seg.id} result={seg.result} />
                          );
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
                                ? (feedback: string) =>
                                    onApprove(seg.toolId, feedback)
                                : undefined
                            }
                            onReject={
                              seg.toolStatus === "awaiting_approval"
                                ? (feedback: string) =>
                                    onReject(seg.toolId, feedback)
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
                          {seg.thinkingContent && (
                            <div className="max-w-[85%]">
                              <ThinkingBlock
                                content={seg.thinkingContent}
                                isStreaming={
                                  !seg.thinkingDone && seg.isStreaming
                                }
                              />
                            </div>
                          )}
                          {seg.content && (
                            <MessageBubble
                              role="assistant"
                              content={seg.content}
                            />
                          )}
                        </div>
                      );
                    })}

                    {turn.status === "failed" && turn.errorMessage && (
                      <div className="flex justify-start">
                        <div className="bg-danger-bg border border-danger-border rounded-lg px-3 py-2 text-xs text-red-600 max-w-[80%]">
                          {turn.errorMessage}
                        </div>
                      </div>
                    )}
                    {turn.status === "interrupted" && (
                      <div className="flex justify-center">
                        <div className="bg-danger-bg border border-danger-border rounded-lg px-3 py-2 text-xs text-red-500 max-w-[80%]">
                          {turn.errorMessage || t("chat.chatInterrupted")}
                        </div>
                      </div>
                    )}
                    {turn.status === "stopped" && (
                      <div className="flex justify-center">
                        <div className="bg-muted/50 border rounded-lg px-3 py-2 text-xs text-muted-foreground max-w-[80%]">
                          {t("chat.chatStopped")}
                        </div>
                      </div>
                    )}
                    {turn.retrying && (
                      <div className="flex justify-center">
                        <div className="bg-warning border border-warning-border rounded-lg px-3 py-2 text-xs text-warning-foreground max-w-[80%] flex items-center gap-2">
                          <Loader2 className="w-3 h-3 animate-spin" />
                          <span>
                            {t("chat.retrying", {
                              attempt: turn.retrying.attempt,
                              max: turn.retrying.maxRetries,
                            })}
                          </span>
                          {turn.retrying.errorMessage && (
                            <span className="opacity-70">
                              · {turn.retrying.errorMessage}
                            </span>
                          )}
                        </div>
                      </div>
                    )}
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
                ))}
              </div>
            )}
          </>
        )}

        <div ref={messagesEndRef} />
      </div>

      <ChatInput
        disabled={!hasNovel || !selectedKey}
        isLoading={isLoading}
        placeholder={inputPlaceholder}
        slashItems={slashCommands}
        onSend={handleSend}
        onListSlash={loadSlash}
        onStop={() => {
          setTurns((prev) =>
            prev.map((t) =>
              t.status === "streaming"
                ? { ...t, status: "stopped" as const }
                : t,
            ),
          );
          app.CancelChat(sessionId);
        }}
      />

      <div className="border-t mx-4" />

      <ChatControls
        models={models}
        selectedKey={selectedKey}
        onSelectModel={handleSelectModel}
        onRefreshModels={refreshModels}
        reasoningEffort={reasoningEffort}
        onSelectEffort={handleSelectEffort}
        approvalMode={approvalMode}
        onToggleApproval={handleToggleApproval}
        onConfigModel={handleConfigModel}
        usage={lastUsage}
        onCompress={handleCompress}
        isTurnRunning={isLoading}
        isCompressing={isCompressing}
      />

      {isDragging && (
        <div className="fixed inset-0 z-50 cursor-col-resize select-none" />
      )}

      <SettingsDialog
        open={showSettings}
        onClose={() => setShowSettings(false)}
        onSaved={() => {
          app
            .GetModels()
            .then((list) => {
              if (list && list.length > 0) {
                setModels(list);
                if (!list.find((m) => m.Key === selectedKey)) {
                  setSelectedKey(list[0].Key);
                  if (list[0].ReasoningLevels?.length) {
                    setReasoningEffort(list[0].ReasoningLevels[0]);
                  }
                }
              }
            })
            .catch(() => {});
        }}
        initialTab="model"
      />
    </aside>
  );
}

import { useCallback, useEffect, useRef, useState } from "react";
import { EventsOn } from "@/lib/wailsjs/runtime/runtime";

export type PatternProgressStage =
  | "loaded"
  | "boundaries"
  | "summaries"
  | "initial_chunks"
  | "compress_chunks"
  | "finalizing"
  | "done";

export type PatternLLMStatus = "" | "thinking" | "generating";

interface PatternProgressPayload {
  task_id?: string;
  novel_id: number;
  stage: PatternProgressStage;
  message: string;
  llm_status?: PatternLLMStatus;
  round?: number;
  batch_index?: number;
  batch_total?: number;
  tokens?: number;
  boundaries?: unknown[];
  summaries?: unknown[];
  chunks?: unknown[];
}

export interface PatternProgressState {
  taskId: string;
  novelId: number;
  stage: PatternProgressStage;
  message: string;
  llmStatus: PatternLLMStatus;
  round: number;
  batchIndex: number;
  batchTotal: number;
  tokens: number;
  boundaryCount: number;
  summaryCount: number;
  chunkCount: number;
}

export interface PatternProgressEvent {
  id: string;
  stage: PatternProgressStage;
  message: string;
  llmStatus: PatternLLMStatus;
  round: number;
  batchIndex: number;
  batchTotal: number;
  tokens: number;
  boundaryCount: number;
  summaryCount: number;
  chunkCount: number;
}

export function createPatternTaskID() {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
    return crypto.randomUUID();
  }
  return `pattern-${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

function toState(data: PatternProgressPayload): PatternProgressState {
  return {
    taskId: data.task_id ?? "",
    novelId: data.novel_id,
    stage: data.stage,
    message: data.message,
    llmStatus: data.llm_status ?? "",
    round: data.round ?? 0,
    batchIndex: data.batch_index ?? 0,
    batchTotal: data.batch_total ?? 0,
    tokens: data.tokens ?? 0,
    boundaryCount: data.boundaries?.length ?? 0,
    summaryCount: data.summaries?.length ?? 0,
    chunkCount: data.chunks?.length ?? 0,
  };
}

function toEvent(
  state: PatternProgressState,
  index: number,
): PatternProgressEvent {
  return {
    id: `${state.taskId}-${index}`,
    stage: state.stage,
    message: state.message,
    llmStatus: state.llmStatus,
    round: state.round,
    batchIndex: state.batchIndex,
    batchTotal: state.batchTotal,
    tokens: state.tokens,
    boundaryCount: state.boundaryCount,
    summaryCount: state.summaryCount,
    chunkCount: state.chunkCount,
  };
}

export function usePatternProgress(runningTaskId: string | null) {
  const [progress, setProgress] = useState<PatternProgressState | null>(null);
  const [events, setEvents] = useState<PatternProgressEvent[]>([]);
  const eventSeqRef = useRef(0);

  useEffect(() => {
    const unsubscribe = EventsOn(
      "pattern:progress",
      (data: PatternProgressPayload) => {
        if (!runningTaskId || data.task_id !== runningTaskId) return;
        const next = toState(data);
        const eventIndex = eventSeqRef.current++;
        setProgress(next);
        setEvents((prev) => [toEvent(next, eventIndex), ...prev]);
      },
    );
    return unsubscribe;
  }, [runningTaskId]);

  const reset = useCallback(() => {
    eventSeqRef.current = 0;
    setProgress(null);
    setEvents([]);
  }, []);

  return { progress, events, reset };
}

import { create } from "zustand";
import type { llm, app } from "@/lib/wailsjs/go/models";

// useChatStore: chat 领域跨组件共享的 UI 状态。
// selectedModel: 结构化选中模型（废弃拼接 key + splitModelKey），ChatPanel/ChatControls 共享。
//   handleSend/handleCompress 直接取 ProviderName + ModelID，不再 splitModelKey 拆字符串。
// reasoningEffort/approvalMode: 与 selectedModel 强相关（选模型时重置 effort），一起进 store 更内聚。
// deletingSession: 删除合并用（SessionHistory + RecentSessions 只 dispatch，DeleteSessionDialog 集中执行）。
//   存完整 SessionMeta 对象而非只 id，ConfirmDialog 需 title（分页场景下 ChatPanel 无法从 query data 查到所有 session）。
// 不进 store：turns/sessionId/isLoading（流式本地）、activeSessionId（ChatPanel 内协调）、
//   showSettings/showHistoryPanel（纯 UI 开关）、拖拽/滚动 refs（5.1 特殊点 1 + 规则 10）。
interface ChatUIState {
  selectedModel: llm.AvailableModel | null;
  setSelectedModel: (model: llm.AvailableModel | null) => void;
  reasoningEffort: string;
  setReasoningEffort: (effort: string) => void;
  approvalMode: "manual" | "auto";
  setApprovalMode: (mode: "manual" | "auto") => void;
  deletingSession: app.SessionMeta | null;
  setDeletingSession: (session: app.SessionMeta | null) => void;
}

export const useChatStore = create<ChatUIState>((set) => ({
  selectedModel: null,
  setSelectedModel: (model) => set({ selectedModel: model }),
  reasoningEffort: "",
  setReasoningEffort: (effort) => set({ reasoningEffort: effort }),
  approvalMode: "manual",
  setApprovalMode: (mode) => set({ approvalMode: mode }),
  deletingSession: null,
  setDeletingSession: (session) => set({ deletingSession: session }),
}));

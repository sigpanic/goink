import { beforeEach, expect, it, vi } from "vitest";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { session } from "@/lib/wailsjs/go/models";
import ChatPanel from "./ChatPanel";
import { AgentEventType } from "./types";

const fixture = vi.hoisted(() => ({
  Chat: vi.fn(),
  CancelChat: vi.fn(),
  GetSession: vi.fn(),
  SetLastSession: vi.fn(),
  toastError: vi.fn(),
  listeners: new Map<string, Set<(data: unknown) => void>>(),
  messages: undefined as session.Message[] | undefined,
  settings: undefined as { last_session_id: string } | undefined,
  models: [],
  selection: {
    adapter_name: "openai_chat",
    provider_name: "test",
    model_id: "model",
    reasoning_effort: "",
  },
}));

vi.mock("@/lib/wailsjs/go/app/App", () => ({
  Chat: fixture.Chat,
  CancelChat: fixture.CancelChat,
  GetSession: fixture.GetSession,
  SetLastSession: fixture.SetLastSession,
  SetApprovalMode: vi.fn(),
  CompressContext: vi.fn(),
}));
vi.mock("@/lib/wailsjs/runtime/runtime", () => ({
  EventsOn: (name: string, callback: (data: unknown) => void) => {
    const listeners = fixture.listeners.get(name) ?? new Set();
    listeners.add(callback);
    fixture.listeners.set(name, listeners);
    return () => listeners.delete(callback);
  },
}));
vi.mock("@/utils/toast", () => ({ toastError: fixture.toastError }));
vi.mock("@/components/settings/useModels", () => ({
  useModels: () => ({ data: fixture.models }),
}));
vi.mock("@/components/settings/useSettings", () => ({
  useSettings: () => ({ data: fixture.settings }),
}));
vi.mock("./useSessions", () => ({ useSessions: () => ({}) }));
vi.mock("./useSession", () => ({ useSession: () => ({}) }));
vi.mock("./useSessionMessages", () => ({
  useSessionMessages: () => ({ data: fixture.messages }),
}));
vi.mock("./useSlashCommands", () => ({ useSlashCommands: () => ({}) }));
vi.mock("./useChatModelSelection", () => ({
  useChatModelSelection: () => ({
    selectedModel: { Key: "test" },
    modelSelection: fixture.selection,
    handleSelectModel: vi.fn(),
    handleSelectEffort: vi.fn(),
  }),
}));
vi.mock("./ChatInput", () => ({
  default: ({
    disabled,
    isLoading,
    isCancelling,
    onSend,
    onStop,
  }: {
    disabled: boolean;
    isLoading: boolean;
    isCancelling: boolean;
    onSend: (message: string) => void;
    onStop: () => void;
  }) => (
    <div>
      <button disabled={disabled || isLoading} onClick={() => onSend("继续")}>
        发送
      </button>
      {isLoading && (
        <button disabled={isCancelling} onClick={onStop}>
          {isCancelling ? "取消中" : "停止"}
        </button>
      )}
    </div>
  ),
}));
vi.mock("./MessageBubble", () => ({
  default: ({ content }: { content: string }) => <div>{content}</div>,
}));
vi.mock("./ChatControls", () => ({ default: () => null }));
vi.mock("./DeleteSessionDialog", () => ({ default: () => null }));
vi.mock("./RecentSessions", () => ({ default: () => null }));
vi.mock("./SessionHistory", () => ({ default: () => null }));
vi.mock("@/components/settings/SettingsDialog", () => ({
  default: () => null,
}));

function deferred() {
  let resolve!: () => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<void>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function emit(name: string, payload: unknown) {
  fixture.listeners.get(name)?.forEach((callback) => callback(payload));
}

function renderPanel() {
  const client = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <ChatPanel
        novelId={1}
        chatPanelWidth={400}
        onChatPanelResize={vi.fn()}
        onApprove={vi.fn()}
        onReject={vi.fn()}
      />
    </QueryClientProvider>,
  );
}

async function startChat() {
  const request = deferred();
  fixture.Chat.mockReturnValueOnce(request.promise);
  renderPanel();
  fireEvent.click(screen.getByRole("button", { name: "发送" }));
  await waitFor(() => expect(fixture.Chat).toHaveBeenCalledTimes(1));
  await act(async () =>
    emit("chat:started", { session_id: "session", turn_id: 1 }),
  );
  return request;
}

beforeEach(() => {
  vi.clearAllMocks();
  fixture.listeners.clear();
  fixture.messages = undefined;
  fixture.settings = undefined;
  fixture.CancelChat.mockResolvedValue(undefined);
  fixture.SetLastSession.mockResolvedValue(undefined);
  fixture.GetSession.mockResolvedValue({ session_id: "session", novel_id: 1 });
  HTMLElement.prototype.scrollIntoView = vi.fn();
});

it.each(["error", "system_interrupted"])(
  "preserves the %s reason when the Chat promise rejects afterward",
  async (eventType) => {
    const request = await startChat();
    await act(async () =>
      emit("chat:turn_outcome", {
        turn_id: 1,
        event_type: eventType,
        reason: "具体结局原因",
      }),
    );
    expect(screen.getByText("具体结局原因")).toBeInTheDocument();
    await act(async () => request.reject("通用调用失败"));
    expect(screen.getByText("具体结局原因")).toBeInTheDocument();
    expect(screen.queryByText("通用调用失败")).not.toBeInTheDocument();
  },
);

it("shows a plain call failure and accepts a later authoritative outcome", async () => {
  const request = await startChat();
  await act(async () => request.reject(new Error("连接失败")));
  expect(screen.getByText("连接失败")).toBeInTheDocument();
  await act(async () =>
    emit("chat:turn_outcome", {
      turn_id: 1,
      event_type: "system_interrupted",
      reason: "已达最大轮数，对话结束",
    }),
  );
  expect(screen.getByText("已达最大轮数，对话结束")).toBeInTheDocument();
  expect(screen.queryByText("连接失败")).not.toBeInTheDocument();
});

it("keeps the failure visible when a retry event arrives after the outcome", async () => {
  const request = await startChat();
  await act(async () => {
    emit("chat:turn_outcome", {
      turn_id: 1,
      event_type: "error",
      reason: "最终失败原因",
    });
    emit("agent:1", {
      turn_id: 1,
      seq: 1,
      type: AgentEventType.Retrying,
      attempt: 1,
      max_retries: 8,
      error: "旧的重试原因",
    });
  });
  expect(screen.getByText("最终失败原因")).toBeInTheDocument();
  expect(screen.queryByText("旧的重试原因")).not.toBeInTheDocument();
  await act(async () => request.reject("最终失败原因"));
});

it("waits for backend confirmation after CancelChat acknowledges the request", async () => {
  const request = await startChat();
  fireEvent.click(screen.getByRole("button", { name: "停止" }));
  await waitFor(() =>
    expect(fixture.CancelChat).toHaveBeenCalledWith("session"),
  );
  expect(screen.getByRole("button", { name: "取消中" })).toBeDisabled();
  expect(screen.queryByText("chat.chatStopped")).not.toBeInTheDocument();
  await act(async () => {
    emit("chat:turn_outcome", {
      turn_id: 1,
      event_type: "user_stopped",
      reason: "",
    });
    request.reject("");
  });
  expect(screen.getByText("chat.chatStopped")).toBeInTheDocument();
  expect(screen.queryByText("chat.chatError")).not.toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: "取消中" }),
  ).not.toBeInTheDocument();
});

it("keeps the conversation running when cancellation fails", async () => {
  const request = await startChat();
  fixture.CancelChat.mockRejectedValueOnce(new Error("取消请求失败"));
  fireEvent.click(screen.getByRole("button", { name: "停止" }));
  await waitFor(() =>
    expect(fixture.toastError).toHaveBeenCalledWith("取消请求失败"),
  );
  expect(screen.getByRole("button", { name: "停止" })).toBeEnabled();
  expect(screen.queryByText("chat.chatStopped")).not.toBeInTheDocument();
  await act(async () => {
    emit("agent:1", {
      turn_id: 1,
      seq: 1,
      type: AgentEventType.Content,
      data: "仍在输出",
    });
    request.resolve();
  });
  expect(screen.getByText("仍在输出")).toBeInTheDocument();
  expect(screen.queryByText("chat.chatStopped")).not.toBeInTheDocument();
});

it("preserves normal completion when the stop request arrives too late", async () => {
  const request = await startChat();
  const cancel = deferred();
  fixture.CancelChat.mockReturnValueOnce(cancel.promise);
  fireEvent.click(screen.getByRole("button", { name: "停止" }));
  await act(async () => {
    emit("agent:1", {
      turn_id: 1,
      seq: 1,
      type: AgentEventType.Content,
      data: "已完成回复",
    });
    request.resolve();
  });
  await act(async () => cancel.resolve());
  expect(screen.getByText("已完成回复")).toBeInTheDocument();
  expect(screen.queryByText("chat.chatStopped")).not.toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: "停止" }),
  ).not.toBeInTheDocument();
});

it.each(["error", "system_interrupted"])(
  "displays the stored %s reason after reopening a session",
  async (eventType) => {
    fixture.settings = { last_session_id: "session" };
    fixture.messages = [
      { role: "user", content: "继续", turn_id: 1, agent_type: "main" },
      {
        role: "system",
        content: "历史结局原因",
        turn_id: 1,
        agent_type: "main",
        event_type: eventType,
      },
    ] as unknown as session.Message[];
    renderPanel();
    expect(await screen.findByText("历史结局原因")).toBeInTheDocument();
  },
);

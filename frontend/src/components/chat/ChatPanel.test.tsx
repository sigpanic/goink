import { beforeEach, expect, it, vi } from "vitest";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { attachment, session } from "@/lib/wailsjs/go/models";
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
  messagesBySession: {} as Record<string, session.Message[]>,
  olderInView: false,
  selectSession: undefined as ((id: string) => void) | undefined,
  historyQuery: {
    hasNextPage: false,
    isFetching: false,
    isFetchingNextPage: false,
    isFetchNextPageError: false,
    isLoading: false,
    isError: false,
    fetchNextPage: vi.fn(),
    refetch: vi.fn(),
  },
  settings: undefined as { last_session_id: string } | undefined,
  models: [],
  supportsVision: true,
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
  useSessionMessages: (id: string) => ({
    ...fixture.historyQuery,
    data: id ? (fixture.messagesBySession[id] ?? fixture.messages) : undefined,
    hasNextPage: !!id && fixture.historyQuery.hasNextPage,
  }),
}));
vi.mock("react-intersection-observer", () => ({
  useInView: () => ({ ref: () => {}, inView: fixture.olderInView }),
}));
vi.mock("./useSlashCommands", () => ({ useSlashCommands: () => ({}) }));
vi.mock("./useChatModelSelection", () => ({
  useChatModelSelection: () => ({
    selectedModel: {
      Key: "test",
      model: { supports_vision: fixture.supportsVision },
    },
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
    onSend: (message: string, images?: attachment.Ref[]) => void;
    onStop: () => void;
  }) => (
    <div>
      <button disabled={disabled || isLoading} onClick={() => onSend("继续")}>
        发送
      </button>
      <button
        disabled={disabled || isLoading}
        onClick={() =>
          onSend("", [{ attachment_id: "picture-id", variant: "standard-v1" }])
        }
      >
        发送图片
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
vi.mock("./SessionHistory", () => ({
  default: ({ onSelectSession }: { onSelectSession: (id: string) => void }) => {
    fixture.selectSession = onSelectSession;
    return null;
  },
}));
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
  const panel = () => (
    <QueryClientProvider client={client}>
      <ChatPanel
        novelId={1}
        chatPanelWidth={400}
        onChatPanelResize={vi.fn()}
        onApprove={vi.fn()}
        onReject={vi.fn()}
      />
    </QueryClientProvider>
  );
  const view = render(panel());
  return { ...view, refresh: () => view.rerender(panel()) };
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
  fixture.supportsVision = true;
  fixture.listeners.clear();
  fixture.messages = undefined;
  fixture.messagesBySession = {};
  fixture.olderInView = false;
  fixture.selectSession = undefined;
  Object.assign(fixture.historyQuery, {
    hasNextPage: false,
    isFetching: false,
    isFetchingNextPage: false,
    isFetchNextPageError: false,
    isLoading: false,
    isError: false,
  });
  fixture.settings = undefined;
  fixture.CancelChat.mockResolvedValue(undefined);
  fixture.SetLastSession.mockResolvedValue(undefined);
  fixture.GetSession.mockResolvedValue({ session_id: "session", novel_id: 1 });
  HTMLElement.prototype.scrollIntoView = vi.fn();
});

it("sends image references with an empty user message", async () => {
  fixture.Chat.mockResolvedValueOnce({ session_id: "session", turn_id: 1 });
  renderPanel();
  fireEvent.click(screen.getByRole("button", { name: "发送图片" }));
  await waitFor(() => expect(fixture.Chat).toHaveBeenCalledTimes(1));
  expect(fixture.Chat.mock.calls[0][0]).toMatchObject({
    novel_id: 1,
    message: "",
    images: [{ attachment_id: "picture-id", variant: "standard-v1" }],
  });
});

it("rejects image sends to a text-only model before calling Chat", () => {
  fixture.supportsVision = false;
  renderPanel();
  fireEvent.click(screen.getByRole("button", { name: "发送图片" }));
  expect(fixture.Chat).not.toHaveBeenCalled();
  expect(fixture.toastError).toHaveBeenCalledWith("chat.visionRequired");
});

it("retains opaque thinking after its end event and subsequent answer", async () => {
  const request = await startChat();
  await act(async () =>
    emit("agent:1", { turn_id: 1, type: AgentEventType.Thinking }),
  );
  expect(screen.getByRole("status")).toHaveTextContent("chat.thinking");
  await act(async () =>
    emit("agent:1", { turn_id: 1, type: AgentEventType.ThinkingDone }),
  );
  expect(screen.getByRole("status")).toHaveTextContent("chat.thinkingProcess");
  await act(async () =>
    emit("agent:1", {
      turn_id: 1,
      type: AgentEventType.Content,
      data: "最终回答",
    }),
  );
  expect(screen.getByText("最终回答")).toBeInTheDocument();
  expect(screen.getByRole("status")).toHaveTextContent("chat.thinkingProcess");
  await act(async () => request.resolve());
  expect(screen.getByText("最终回答")).toBeVisible();
  expect(screen.queryByRole("status")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "chat.workHistory" }));
  expect(screen.getByRole("status")).toHaveTextContent("chat.thinkingProcess");
});

it.each([true, false])(
  "respects bottom-following=%s when completed work history collapses",
  async (nearBottom) => {
    const request = await startChat();
    await act(async () => {
      emit("agent:1", { turn_id: 1, type: AgentEventType.Thinking });
      emit("agent:1", { turn_id: 1, type: AgentEventType.ThinkingDone });
      emit("agent:1", {
        turn_id: 1,
        type: AgentEventType.Content,
        data: "最终回复",
      });
    });
    const scrollContainer = screen
      .getByText("最终回复")
      .closest(".overflow-y-auto")!;
    Object.defineProperties(scrollContainer, {
      scrollHeight: { configurable: true, value: 1000 },
      clientHeight: { configurable: true, value: 400 },
      scrollTop: { configurable: true, value: nearBottom ? 600 : 100 },
    });
    fireEvent.scroll(scrollContainer);
    vi.mocked(HTMLElement.prototype.scrollIntoView).mockClear();
    await act(async () => request.resolve());
    expect(
      screen.getByRole("button", { name: "chat.workHistory" }),
    ).toHaveAttribute("aria-expanded", "false");
    expect(screen.getByText("最终回复")).toBeVisible();
    if (nearBottom) {
      expect(HTMLElement.prototype.scrollIntoView).toHaveBeenCalledWith({
        behavior: "instant",
      });
    } else {
      expect(HTMLElement.prototype.scrollIntoView).not.toHaveBeenCalled();
    }
  },
);

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

function storedTurn(turnId: number, answer: string): session.Message[] {
  return [
    {
      role: "user",
      content: `问题${turnId}`,
      turn_id: turnId,
      agent_type: "main",
    },
    { role: "assistant", content: answer, turn_id: turnId, agent_type: "main" },
  ] as session.Message[];
}

it("jumps to the latest message and resumes following subsequent output", async () => {
  const request = await startChat();
  await act(async () =>
    emit("agent:1", {
      turn_id: 1,
      type: AgentEventType.Content,
      data: "最新回复",
    }),
  );
  const container = screen.getByText("最新回复").closest(".overflow-y-auto")!;
  Object.defineProperties(container, {
    scrollHeight: { configurable: true, value: 1000 },
    clientHeight: { configurable: true, value: 400 },
    scrollTop: { configurable: true, writable: true, value: 100 },
  });
  fireEvent.scroll(container);
  vi.mocked(HTMLElement.prototype.scrollIntoView).mockClear();
  await act(async () =>
    emit("agent:1", {
      turn_id: 1,
      type: AgentEventType.Content,
      data: "继续输出",
    }),
  );
  expect(HTMLElement.prototype.scrollIntoView).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "chat.scrollToBottom" }));
  expect(HTMLElement.prototype.scrollIntoView).toHaveBeenCalledWith({
    behavior: "instant",
  });
  expect(
    screen.queryByRole("button", { name: "chat.scrollToBottom" }),
  ).not.toBeInTheDocument();
  vi.mocked(HTMLElement.prototype.scrollIntoView).mockClear();
  await act(async () => request.resolve());
  expect(HTMLElement.prototype.scrollIntoView).toHaveBeenCalledWith({
    behavior: "instant",
  });
});

it("prepends older turns without replacing live output or moving the reading position", async () => {
  fixture.settings = { last_session_id: "session" };
  fixture.messages = storedTurn(2, "历史回复");
  fixture.historyQuery.hasNextPage = true;
  const view = renderPanel();
  const answer = await screen.findByText("历史回复");
  const request = deferred();
  fixture.Chat.mockReturnValueOnce(request.promise);
  fireEvent.click(screen.getByRole("button", { name: "发送" }));
  await act(async () => {
    emit("chat:started", { session_id: "session", turn_id: 3 });
    emit("agent:3", {
      turn_id: 3,
      type: AgentEventType.Content,
      data: "实时输出",
    });
  });
  expect(screen.getByText("实时输出")).toBeInTheDocument();
  const container = answer.closest(".overflow-y-auto")!;
  Object.defineProperties(container, {
    scrollHeight: {
      configurable: true,
      get: () =>
        1000 + container.querySelectorAll("[data-turn-id]").length * 300,
    },
    clientHeight: { configurable: true, value: 400 },
    scrollTop: { configurable: true, writable: true, value: 100 },
  });
  const anchor = answer.closest<HTMLElement>("[data-turn-id]")!;
  vi.spyOn(anchor, "getBoundingClientRect").mockImplementation(
    () =>
      new DOMRect(
        0,
        20 +
          (container.querySelectorAll("[data-turn-id]").length - 2) * 300 -
          (container.scrollTop - 100),
        400,
        200,
      ),
  );
  fireEvent.scroll(container);
  vi.mocked(HTMLElement.prototype.scrollIntoView).mockClear();
  fixture.messages = [
    ...storedTurn(1, "更早的回复"),
    ...fixture.messages,
    ...storedTurn(3, "落后的数据库快照"),
  ];
  view.refresh();
  expect(await screen.findByText("更早的回复")).toBeInTheDocument();
  expect(screen.getByText("实时输出")).toBeInTheDocument();
  expect(screen.queryByText("落后的数据库快照")).not.toBeInTheDocument();
  expect(
    Array.from(container.querySelectorAll("[data-turn-id]")).map((element) =>
      element.getAttribute("data-turn-id"),
    ),
  ).toEqual(["1", "2", "3"]);
  expect(container.scrollTop).toBe(400);
  expect(anchor.getBoundingClientRect().top).toBe(20);
  expect(HTMLElement.prototype.scrollIntoView).not.toHaveBeenCalled();
  await act(async () => request.resolve());
});

it("loads older history when its sentinel is visible and offers retry without hiding existing turns", async () => {
  fixture.settings = { last_session_id: "session" };
  fixture.messages = storedTurn(2, "保留的历史");
  fixture.historyQuery.hasNextPage = true;
  fixture.olderInView = true;
  const view = renderPanel();
  await screen.findByText("保留的历史");
  expect(fixture.historyQuery.fetchNextPage).toHaveBeenCalledWith({
    cancelRefetch: false,
  });
  fixture.historyQuery.fetchNextPage.mockClear();
  fixture.historyQuery.isFetchNextPageError = true;
  fixture.historyQuery.isError = true;
  view.refresh();
  expect(screen.getByText("保留的历史")).toBeInTheDocument();
  expect(screen.getByRole("alert")).toHaveTextContent(
    "chat.loadOlderMessagesFailed",
  );
  expect(fixture.historyQuery.fetchNextPage).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "chat.retry" }));
  expect(fixture.historyQuery.fetchNextPage).toHaveBeenCalledTimes(1);
});

it("clears the previous conversation and ignores its later page updates after switching sessions", async () => {
  fixture.settings = { last_session_id: "session" };
  fixture.messagesBySession = {
    session: storedTurn(3, "会话一的回复"),
    other: storedTurn(1, "会话二的回复"),
  };
  const view = renderPanel();
  await screen.findByText("会话一的回复");
  await act(async () => fixture.selectSession?.("other"));
  expect(await screen.findByText("会话二的回复")).toBeInTheDocument();
  expect(screen.queryByText("会话一的回复")).not.toBeInTheDocument();
  fixture.messagesBySession.session = [
    ...storedTurn(2, "会话一的旧页"),
    ...fixture.messagesBySession.session,
  ];
  view.refresh();
  expect(screen.queryByText("会话一的旧页")).not.toBeInTheDocument();
  expect(screen.getByText("会话二的回复")).toBeInTheDocument();
});

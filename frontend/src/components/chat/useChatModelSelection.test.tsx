import { beforeEach, expect, it, vi } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { PropsWithChildren } from "react";
import { adapter, app, config, llm } from "@/lib/wailsjs/go/models";
import { modelKeys, sessionKeys } from "@/lib/queryKeys";
import { useModels } from "@/components/settings/useModels";
import { useSettings } from "@/components/settings/useSettings";
import { useSession } from "./useSession";
import { useChatStore } from "./useChatStore";
import { useChatModelSelection } from "./useChatModelSelection";

const api = vi.hoisted(() => ({
  GetModels: vi.fn(),
  GetSettings: vi.fn(),
  GetSession: vi.fn(),
  SetSelectedModel: vi.fn(),
}));
const toastError = vi.hoisted(() => vi.fn());
vi.mock("@/lib/wailsjs/go/app/App", () => api);
vi.mock("@/utils/toast", () => ({ toastError }));

let models: llm.AvailableModel[];
let settings: config.AppSettings;
let sessions: Record<string, app.SessionDetail>;

function choice(index: number, effort: string): adapter.ModelSelection {
  const m = models[index];
  return {
    adapter_name: m.AdapterName,
    provider_name: m.ProviderName,
    model_id: m.model.id,
    reasoning_effort: effort,
  };
}

function save(input: app.SetSelectedModelInput) {
  if (input.session_id) {
    sessions[input.session_id] = new app.SessionDetail({
      ...sessions[input.session_id],
      model_selection: input.model_selection,
    });
  } else {
    settings = new config.AppSettings({
      ...settings,
      default_model_selection: input.model_selection,
    });
  }
}

beforeEach(() => {
  vi.resetAllMocks();
  useChatStore.setState({ selectedModel: null, reasoningEffort: "" });
  models = [
    "openai_chat",
    "anthropic_messages",
    "gemini_interactions",
    "openai_responses",
  ].map(
    (protocol, i) =>
      new llm.AvailableModel({
        Key: `opaque-${i}`,
        AdapterName: protocol,
        ProviderName: "Relay",
        ProviderType: "generic",
        model: {
          id: "same-model",
          name: protocol,
          reasoning_levels:
            i === 3
              ? undefined
              : i === 0
                ? ["high", "max"]
                : ["medium", "high"],
        },
      }),
  );
  settings = new config.AppSettings({
    default_model_selection: choice(0, "max"),
  });
  sessions = {
    a: new app.SessionDetail({
      session_id: "a",
      novel_id: 1,
      model_selection: choice(1, "medium"),
    }),
    b: new app.SessionDetail({
      session_id: "b",
      novel_id: 1,
      model_selection: choice(2, "high"),
    }),
  };
  api.GetModels.mockImplementation(async () => models);
  api.GetSettings.mockImplementation(async () => settings);
  api.GetSession.mockImplementation(async (id: string) => sessions[id]);
  api.SetSelectedModel.mockImplementation(
    async (input: app.SetSelectedModelInput) => save(input),
  );
});

function setup(sessionId = "") {
  const qc = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: Infinity },
      mutations: { retry: false },
    },
  });
  const hook = renderHook(
    ({ sessionId }) => {
      const modelQuery = useModels();
      const settingsQuery = useSettings();
      const sessionQuery = useSession(sessionId);
      return useChatModelSelection({
        sessionId,
        models: modelQuery.data,
        defaultSelection: settingsQuery.data?.default_model_selection,
        session: sessionQuery.data,
        enabled: true,
      });
    },
    {
      initialProps: { sessionId },
      wrapper: ({ children }: PropsWithChildren) => (
        <QueryClientProvider client={qc}>{children}</QueryClientProvider>
      ),
    },
  );
  return { qc, ...hook };
}

it("restores per-session selections by all three identity fields and saves global defaults only outside a session", async () => {
  const { result, rerender, qc } = setup();
  await waitFor(() =>
    expect(result.current.modelSelection).toEqual(choice(0, "max")),
  );
  rerender({ sessionId: "a" });
  expect(result.current.modelSelection).toBeNull();
  await waitFor(() =>
    expect(result.current.modelSelection).toEqual(choice(1, "medium")),
  );
  act(() => result.current.handleSelectEffort("high"));
  await waitFor(() =>
    expect(sessions.a.model_selection).toEqual(choice(1, "high")),
  );
  expect(settings.default_model_selection).toEqual(choice(0, "max"));
  rerender({ sessionId: "b" });
  await waitFor(() =>
    expect(result.current.modelSelection).toEqual(choice(2, "high")),
  );
  rerender({ sessionId: "a" });
  await waitFor(() =>
    expect(result.current.modelSelection).toEqual(choice(1, "high")),
  );
  rerender({ sessionId: "" });
  await waitFor(() =>
    expect(result.current.modelSelection).toEqual(choice(0, "max")),
  );
  act(() => result.current.handleSelectModel(models[3].Key));
  expect(result.current.modelSelection).toEqual(choice(3, ""));
  expect(useChatStore.getState().reasoningEffort).toBe("");
  await waitFor(() =>
    expect(settings.default_model_selection).toEqual(choice(3, "")),
  );
  expect(api.SetSelectedModel.mock.calls.map(([input]) => input)).toEqual([
    expect.objectContaining({
      session_id: "a",
      model_selection: choice(1, "high"),
    }),
    expect.objectContaining({ session_id: "", model_selection: choice(3, "") }),
  ]);
  await waitFor(() => expect(qc.isMutating()).toBe(0));
});

it("keeps an immediate request snapshot and serializes rapid saves without mixing sessions or accepting stale refetches", async () => {
  sessions.b.model_selection = choice(0, "max");
  let finishFirst!: () => void;
  const first = new Promise<void>((resolve) => {
    finishFirst = resolve;
  });
  api.SetSelectedModel.mockImplementationOnce(
    async (input: app.SetSelectedModelInput) => {
      await first;
      save(input);
    },
  );
  const { result, rerender, qc } = setup("a");
  await waitFor(() =>
    expect(result.current.selectedModel?.Key).toBe(models[1].Key),
  );
  act(() => result.current.handleSelectModel(models[2].Key));
  const requestSnapshot = result.current.modelSelection;
  expect(requestSnapshot).toEqual(choice(2, "medium"));
  act(() => result.current.handleSelectEffort("high"));
  expect(result.current.modelSelection).toEqual(choice(2, "high"));
  await waitFor(() => expect(api.SetSelectedModel).toHaveBeenCalledTimes(1));
  await act(async () => {
    await qc.invalidateQueries({ queryKey: sessionKeys.detail("a") });
  });
  expect(result.current.modelSelection).toEqual(choice(2, "high"));
  rerender({ sessionId: "b" });
  await waitFor(() =>
    expect(result.current.modelSelection).toEqual(choice(0, "max")),
  );
  expect(api.SetSelectedModel).toHaveBeenCalledTimes(1);
  await act(async () => {
    finishFirst();
  });
  await waitFor(() =>
    expect(sessions.a.model_selection).toEqual(choice(2, "high")),
  );
  expect(requestSnapshot).toEqual(choice(2, "medium"));
  rerender({ sessionId: "a" });
  await waitFor(() =>
    expect(result.current.modelSelection).toEqual(choice(2, "high")),
  );
  expect(settings.default_model_selection).toEqual(choice(0, "max"));
  await waitFor(() => expect(qc.isMutating()).toBe(0));
});

it("retains a failed selection with an error and refreshes model capabilities without silently saving a fallback", async () => {
  api.SetSelectedModel.mockRejectedValueOnce(new Error("disk full"));
  const { result, qc } = setup("a");
  await waitFor(() =>
    expect(result.current.selectedModel?.Key).toBe(models[1].Key),
  );
  act(() => result.current.handleSelectEffort("high"));
  await waitFor(() => expect(toastError).toHaveBeenCalledWith("disk full"));
  expect(result.current.modelSelection).toEqual(choice(1, "high"));
  models = [
    new llm.AvailableModel({
      ...models[1],
      model: { ...models[1].model, reasoning_levels: undefined },
    }),
  ];
  await act(async () => {
    await qc.invalidateQueries({ queryKey: modelKeys.all });
  });
  await waitFor(() =>
    expect(result.current.modelSelection?.reasoning_effort).toBe(""),
  );
  models = [
    new llm.AvailableModel({
      ...models[0],
      Key: "replacement",
      AdapterName: "openai_chat",
      model: { id: "replacement", name: "Replacement" },
    }),
  ];
  await act(async () => {
    await qc.invalidateQueries({ queryKey: modelKeys.all });
  });
  await waitFor(() =>
    expect(result.current.modelSelection?.model_id).toBe("replacement"),
  );
  expect(api.SetSelectedModel).toHaveBeenCalledTimes(1);
});

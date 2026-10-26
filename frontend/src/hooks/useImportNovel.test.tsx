import { beforeEach, afterEach, expect, it, vi } from "vitest";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { config, llm } from "@/lib/wailsjs/go/models";
import { modelKeys, settingsKeys } from "@/lib/queryKeys";
import ImportProgressDialog from "@/components/novel/ImportProgressDialog";
import { useImportNovel } from "./useImportNovel";

const api = vi.hoisted(() => ({
  GetModels: vi.fn(),
  GetSettings: vi.fn(),
  ImportNovel: vi.fn(),
  PickAndImportNovel: vi.fn(),
  ImportWithLLM: vi.fn(),
  SetSelectedModel: vi.fn(),
}));
vi.mock("@/lib/wailsjs/go/app/App", () => api);
vi.mock("@/lib/wailsjs/runtime/runtime", () => ({
  EventsOn: vi.fn(() => vi.fn()),
}));

let models: llm.AvailableModel[];
let settings: config.AppSettings;
let qc: QueryClient;
const imported = {
  novel_id: 9,
  chapter_count: 6,
  skipped_count: 0,
  skipped_chapters: [],
};

beforeEach(() => {
  vi.resetAllMocks();
  models = [
    ["openai_chat", "Chat", "openai"],
    ["anthropic_messages", "Claude", "anthropic"],
    ["gemini_interactions", "Gemini", "google"],
  ].map(
    ([protocol, name, type], i) =>
      new llm.AvailableModel({
        Key: `opaque-${i}`,
        AdapterName: protocol,
        ProviderName: "Relay",
        ProviderType: type,
        model: {
          id: "shared-model",
          name,
          reasoning_levels: ["medium", "high"],
        },
      }),
  );
  settings = new config.AppSettings({
    default_model_selection: {
      adapter_name: "gemini_interactions",
      provider_name: "Relay",
      model_id: "shared-model",
      reasoning_effort: "high",
    },
  });
  api.GetModels.mockImplementation(async () => models);
  api.GetSettings.mockImplementation(async () => settings);
  api.PickAndImportNovel.mockResolvedValue({
    needs_llm: true,
    file_path: "/picked/novel.txt",
  });
  api.ImportNovel.mockResolvedValue({ needs_llm: true });
  api.ImportWithLLM.mockResolvedValue(imported);
});

afterEach(() => qc.clear());

function setup(filePath?: string) {
  const onImported = vi.fn().mockResolvedValue(undefined);
  function ImportFlow() {
    const flow = useImportNovel({ onImported });
    return (
      <>
        <button onClick={() => void flow.startImport(filePath)}>import</button>
        <ImportProgressDialog
          {...flow.dialogProps}
          models={flow.models}
          selectedModel={flow.selectedModel}
          onModelChange={flow.onModelChange}
          onStartLLM={flow.startLLMImport}
        />
      </>
    );
  }
  qc = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  render(
    <QueryClientProvider client={qc}>
      <ImportFlow />
    </QueryClientProvider>,
  );
  return { user: userEvent.setup(), onImported };
}

it("restores the default protocol and preserves a local model choice across query refreshes before AI import", async () => {
  const { user, onImported } = setup();
  await user.click(screen.getByRole("button", { name: "import" }));
  await user.click(await screen.findByRole("button", { name: "Gemini" }));
  expect(screen.getByText("Relay (Gemini Interactions)")).toBeInTheDocument();
  expect(screen.getByText("Relay (Anthropic Messages)")).toBeInTheDocument();
  expect(screen.queryByText("chat.reasoning")).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "Claude" }));

  act(() => {
    qc.setQueryData(
      settingsKeys.all,
      new config.AppSettings({
        ...settings,
        default_model_selection: {
          ...settings.default_model_selection,
          adapter_name: "openai_chat",
        },
      }),
    );
    qc.setQueryData(
      modelKeys.all,
      models.map(
        (m) =>
          new llm.AvailableModel({
            ...m,
            model: { ...m.model, context_window: 123_000 },
          }),
      ),
    );
  });
  expect(screen.getByRole("button", { name: "Claude" })).toBeInTheDocument();
  await user.click(
    screen.getByRole("button", { name: "novel.importAnalyzeBtn" }),
  );
  await waitFor(() =>
    expect(onImported).toHaveBeenCalledExactlyOnceWith(imported),
  );
  expect(api.ImportWithLLM).toHaveBeenCalledExactlyOnceWith({
    file_path: "/picked/novel.txt",
    adapter_name: "anthropic_messages",
    provider_name: "Relay",
    model_id: "shared-model",
  });
  expect(api.SetSelectedModel).not.toHaveBeenCalled();
});

it("falls back to an available model and retains the explicit file path when the saved route is unavailable", async () => {
  settings.default_model_selection.adapter_name = "unavailable";
  const { user, onImported } = setup("/explicit/novel.txt");
  await user.click(screen.getByRole("button", { name: "import" }));
  await screen.findByRole("button", { name: "Chat" });
  expect(api.ImportNovel).toHaveBeenCalledExactlyOnceWith({
    file_path: "/explicit/novel.txt",
  });
  expect(api.PickAndImportNovel).not.toHaveBeenCalled();
  await user.click(
    screen.getByRole("button", { name: "novel.importAnalyzeBtn" }),
  );
  await waitFor(() =>
    expect(onImported).toHaveBeenCalledExactlyOnceWith(imported),
  );
  expect(api.ImportWithLLM).toHaveBeenCalledExactlyOnceWith({
    file_path: "/explicit/novel.txt",
    adapter_name: "openai_chat",
    provider_name: "Relay",
    model_id: "shared-model",
  });
});

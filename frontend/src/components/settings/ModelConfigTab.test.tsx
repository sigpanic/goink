import { beforeEach, expect, it, vi } from "vitest";
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { adapter, llm } from "@/lib/wailsjs/go/models";
import ModelConfigTab from "./ModelConfigTab";

const api = vi.hoisted(() => ({
  GetLLMConfig: vi.fn(),
  SaveLLMConfig: vi.fn(),
  TestConnection: vi.fn(),
  DiscoverModels: vi.fn(),
}));
vi.mock("@/lib/wailsjs/go/app/App", () => api);
vi.mock("@/lib/wailsjs/runtime/runtime", () => ({ BrowserOpenURL: vi.fn() }));
vi.mock("@/utils/toast", () => ({ toastSuccess: vi.fn() }));

let stored: llm.LLMConfigView;

beforeEach(() => {
  vi.resetAllMocks();
  stored = new llm.LLMConfigView({
    providers: [
      ["openai_chat", "Chat", "openai"],
      ["anthropic_messages", "Claude", "anthropic"],
      ["openai_responses", "Responses", "openai"],
      ["gemini_interactions", "Gemini", "google"],
    ].map(([protocol, name, type]) => ({
      key: `${protocol}/${name}`,
      adapter_name: protocol,
      provider_type: type,
      name,
      chat_url: `https://relay.test/${protocol}`,
      api_key: `key-${protocol}`,
      platform_url: "",
      help_text: "",
      source: "builtin",
      builtin_models: [{ id: "model", name: "Model", context_window: 200_000 }],
      custom_models: [],
    })),
  });
  api.GetLLMConfig.mockImplementation(() => Promise.resolve(stored));
  api.SaveLLMConfig.mockResolvedValue(undefined);
  api.TestConnection.mockImplementation((input: llm.TestConnectionInput) =>
    Promise.resolve(
      new adapter.TestConnectionResult({
        URL: input.chat_url + "/resolved",
        Warning: input.adapter_name,
      }),
    ),
  );
  api.DiscoverModels.mockResolvedValue([
    new adapter.ModelInfo({
      id: "discovered",
      name: "Discovered model",
      context_window: 123_000,
      max_output_tokens: 4_000,
      supports_thinking: false,
      supports_vision: true,
    }),
  ]);
});

function setup() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <ModelConfigTab />
    </QueryClientProvider>,
  );
  return userEvent.setup();
}

it("routes probes by protocol and instance and saves resolved URLs and discovered capabilities", async () => {
  const user = setup();
  await screen.findByRole("button", { name: "Chat" });
  for (const [index, provider] of stored.providers.entries()) {
    if (index > 0) {
      await user.click(
        screen.getByRole("button", {
          name: stored.providers[index - 1].name,
        }),
      );
      await user.click(
        screen.getByRole("button", { name: new RegExp(`^${provider.name}`) }),
      );
    }
    await user.click(screen.getByRole("button", { name: "settings.test" }));
    await screen.findByText(`⚠ ${provider.adapter_name}`);
    expect(api.TestConnection).toHaveBeenLastCalledWith({
      adapter_name: provider.adapter_name,
      provider_name: provider.name,
      provider_type: provider.provider_type,
      chat_url: provider.chat_url,
      api_key: provider.api_key,
      model_id: "model",
    });
    await user.click(
      screen.getByRole("button", { name: "settings.autoDiscover" }),
    );
    await screen.findByText("settings.foundModels");
    expect(api.DiscoverModels).toHaveBeenLastCalledWith({
      adapter_name: provider.adapter_name,
      provider_name: provider.name,
      provider_type: provider.provider_type,
      chat_url: provider.chat_url + "/resolved",
      api_key: provider.api_key,
    });
  }
  await user.click(
    screen.getByRole("button", { name: "settings.importSelected" }),
  );
  expect(
    screen.getByRole("checkbox", { name: "settings.supportDeepThinking" }),
  ).not.toBeChecked();
  expect(
    screen.getByRole("checkbox", { name: "settings.vision" }),
  ).toBeChecked();
  await user.click(screen.getByRole("button", { name: "common.confirm" }));
  await user.click(screen.getByRole("button", { name: "settings.saveConfig" }));
  await waitFor(() => expect(api.SaveLLMConfig).toHaveBeenCalledTimes(1));
  const saved: llm.LLMConfigView = api.SaveLLMConfig.mock.calls[0][0];
  expect(saved.providers).toHaveLength(4);
  for (const [index, provider] of saved.providers.entries()) {
    expect(provider.adapter_name).toBe(stored.providers[index].adapter_name);
    expect(provider.chat_url).toBe(
      stored.providers[index].chat_url + "/resolved",
    );
    expect(provider.temperature).toBeUndefined();
  }
  expect(saved.providers[3].custom_models).toEqual([
    expect.objectContaining({
      id: "discovered",
      supports_thinking: false,
      supports_vision: true,
      context_window: 123_000,
      max_output_tokens: 4_000,
    }),
  ]);
  expect(api.TestConnection).toHaveBeenCalledTimes(4);
});

it("saves URL changes and an unset temperature without requiring an API key or probing", async () => {
  stored.providers = [stored.providers[0]];
  stored.providers[0].api_key = "";
  stored.providers[0].temperature = 0.7;
  const user = setup();
  await screen.findByRole("button", { name: "Chat" });
  fireEvent.change(screen.getByRole("textbox", { name: "settings.chatUrl" }), {
    target: { value: "https://changed.test/v8/chat/completions" },
  });
  await user.click(
    screen.getByRole("button", { name: "settings.resetTemperature" }),
  );
  expect(screen.getByText("settings.defaultTemperature")).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "settings.saveConfig" }));
  await waitFor(() => expect(api.SaveLLMConfig).toHaveBeenCalledTimes(1));
  expect(api.SaveLLMConfig.mock.calls[0][0].providers[0]).toMatchObject({
    adapter_name: "openai_chat",
    provider_type: "openai",
    name: "Chat",
    api_key: "",
    chat_url: "https://changed.test/v8/chat/completions",
    temperature: undefined,
  });
  expect(api.TestConnection).not.toHaveBeenCalled();
});

it("creates same-named instances under four protocols and preserves their selection after reloading", async () => {
  stored.providers.forEach((provider) => {
    provider.api_key = "";
  });
  api.SaveLLMConfig.mockImplementation((input: llm.LLMConfigView) => {
    stored = new llm.LLMConfigView(input);
    return Promise.resolve();
  });
  const user = setup();
  await screen.findByRole("button", { name: "Chat" });
  await user.click(
    screen.getByRole("button", { name: "settings.customProviders" }),
  );
  const choices = [
    ["openai_chat", "OpenAI Chat", "openai"],
    ["anthropic_messages", "Anthropic Messages", "anthropic"],
    ["openai_responses", "OpenAI Responses", "openai"],
    ["gemini_interactions", "Gemini Interactions", "google"],
  ];
  const name = "Relay/中文+官方 !";
  for (const [index, [protocol, label]] of choices.entries()) {
    await user.click(
      screen.getByRole("button", {
        name: "settings.addCustomProvider",
      }),
    );
    const form = within(
      screen.getByRole("group", { name: "settings.newCustomProvider" }),
    );
    expect(form.getAllByRole("option")).toHaveLength(4);
    await user.selectOptions(
      form.getByRole("combobox", { name: "settings.protocol" }),
      protocol,
    );
    fireEvent.change(form.getByRole("textbox", { name: "common.name" }), {
      target: { value: name },
    });
    fireEvent.change(form.getByRole("textbox", { name: "settings.chatUrl" }), {
      target: { value: `https://relay.test/${protocol}` },
    });
    await user.click(form.getByRole("button", { name: "settings.add" }));
    await screen.findByText(index === 0 ? name : `${name} (${label})`);
  }
  await user.click(screen.getByRole("button", { name: "settings.saveConfig" }));
  await waitFor(() => expect(api.GetLLMConfig).toHaveBeenCalledTimes(2));
  const saved: llm.LLMConfigView = api.SaveLLMConfig.mock.calls[0][0];
  const custom = saved.providers.filter((p) => p.source === "custom");
  expect(custom).toHaveLength(4);
  for (const [index, [protocol, , type]] of choices.entries()) {
    expect(custom[index]).toMatchObject({
      key: `${protocol}/Relay%2F%E4%B8%AD%E6%96%87+%E5%AE%98%E6%96%B9%20%21`,
      adapter_name: protocol,
      provider_type: type,
      name,
      chat_url: `https://relay.test/${protocol}`,
      api_key: "",
      temperature: undefined,
    });
  }
  const selectedName = `${name} (Gemini Interactions)`;
  await user.click(
    screen.getByRole("button", {
      name: (label) => label.includes(selectedName),
    }),
  );
  for (const [, label] of choices) {
    expect(screen.getAllByText(`${name} (${label})`).length).toBeGreaterThan(0);
  }
  await user.click(
    screen.getByRole("button", {
      name: (label) => label.includes(`${name} (OpenAI Chat)`),
    }),
  );
  expect(screen.getByRole("textbox", { name: "settings.chatUrl" })).toHaveValue(
    "https://relay.test/openai_chat",
  );
  expect(api.TestConnection).not.toHaveBeenCalled();
});

it("rejects names occupied by a builtin or custom instance only within the selected protocol", async () => {
  stored.providers = [stored.providers[0]];
  stored.providers[0].api_key = "";
  const user = setup();
  await screen.findByRole("button", { name: "Chat" });
  await user.click(
    screen.getByRole("button", { name: "settings.customProviders" }),
  );
  await user.click(
    screen.getByRole("button", { name: "settings.addCustomProvider" }),
  );
  let form = within(
    screen.getByRole("group", { name: "settings.newCustomProvider" }),
  );
  fireEvent.change(form.getByRole("textbox", { name: "common.name" }), {
    target: { value: " Chat " },
  });
  fireEvent.change(form.getByRole("textbox", { name: "settings.chatUrl" }), {
    target: { value: "https://relay.test/messages" },
  });
  await user.click(form.getByRole("button", { name: "settings.add" }));
  expect(screen.getByRole("alert")).toHaveTextContent(
    "settings.providerNameConflict",
  );
  await user.selectOptions(
    form.getByRole("combobox", { name: "settings.protocol" }),
    "anthropic_messages",
  );
  expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  await user.click(form.getByRole("button", { name: "settings.add" }));
  await user.click(
    screen.getByRole("button", { name: "settings.addCustomProvider" }),
  );
  form = within(
    screen.getByRole("group", { name: "settings.newCustomProvider" }),
  );
  await user.selectOptions(
    form.getByRole("combobox", { name: "settings.protocol" }),
    "anthropic_messages",
  );
  fireEvent.change(form.getByRole("textbox", { name: "common.name" }), {
    target: { value: "Chat" },
  });
  fireEvent.change(form.getByRole("textbox", { name: "settings.chatUrl" }), {
    target: { value: "https://relay.test/other" },
  });
  await user.click(form.getByRole("button", { name: "settings.add" }));
  expect(screen.getByRole("alert")).toHaveTextContent(
    "settings.providerNameConflict",
  );
  await user.click(form.getByRole("button", { name: "common.cancel" }));
  await user.click(screen.getByRole("button", { name: "settings.saveConfig" }));
  await waitFor(() => expect(api.SaveLLMConfig).toHaveBeenCalledTimes(1));
  const saved: llm.LLMConfigView = api.SaveLLMConfig.mock.calls[0][0];
  expect(saved.providers).toHaveLength(2);
  expect(saved.providers[0]).toMatchObject({
    adapter_name: "openai_chat",
    name: "Chat",
    source: "builtin",
  });
  expect(saved.providers[1]).toMatchObject({
    adapter_name: "anthropic_messages",
    provider_type: "anthropic",
    name: "Chat",
    source: "custom",
  });
});

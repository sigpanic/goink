import { expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { llm } from "@/lib/wailsjs/go/models";
import ModelPicker from "./ModelPicker";

function model(
  protocol: string,
  provider: string,
  id: string,
  reasoningLevels?: string[],
) {
  return new llm.AvailableModel({
    Key: JSON.stringify([protocol, provider, id]),
    AdapterName: protocol,
    ProviderName: provider,
    ProviderType: "openai",
    model: { id, name: id, reasoning_levels: reasoningLevels },
  });
}

it("groups instances by protocol and name and qualifies only cross-protocol duplicates", async () => {
  const user = userEvent.setup();
  const onSelectModel = vi.fn();
  const models = [
    model("openai_chat", "Relay", "shared"),
    model("openai_chat", "Relay", "second"),
    model("openai_chat", "Another relay", "shared"),
    model("openai_responses", "Relay", "shared"),
  ];
  render(
    <ModelPicker
      models={models}
      selectedKey=""
      onSelectModel={onSelectModel}
    />,
  );
  await user.click(
    screen.getByRole("button", { name: "chat.noModelAvailable" }),
  );
  expect(screen.getAllByText("Relay (OpenAI Chat)")).toHaveLength(1);
  expect(screen.getAllByText("Another relay")).toHaveLength(1);
  expect(screen.getAllByText("Relay (OpenAI Responses)")).toHaveLength(1);
  await user.click(screen.getAllByRole("button", { name: "shared" })[2]);
  expect(onSelectModel).toHaveBeenCalledExactlyOnceWith(models[3].Key);
  expect(
    screen.queryByText("Relay (OpenAI Responses)"),
  ).not.toBeInTheDocument();
});

it("uses the selected model's reasoning levels and preserves protocol-specific values", async () => {
  const user = userEvent.setup();
  const onSelectEffort = vi.fn();
  const models = [
    model("openai_responses", "Official", "Reasoner", [
      "none",
      "minimal",
      "low",
      "medium",
      "high",
      "xhigh",
      "max",
      "future",
    ]),
    model("openai_chat", "Relay", "Plain"),
  ];
  const props = { models, onSelectModel: vi.fn(), onSelectEffort };
  const { rerender } = render(
    <ModelPicker
      {...props}
      selectedKey={models[0].Key}
      reasoningEffort="medium"
    />,
  );
  await user.click(screen.getByRole("button", { name: "Reasoner" }));
  const labels = [
    "chat.noReasoning",
    "chat.minimalReasoning",
    "chat.lowReasoning",
    "chat.mediumReasoning",
    "chat.highReasoning",
    "chat.extraHighReasoning",
    "chat.maxReasoning",
    "future",
  ];
  for (const [index, label] of labels.entries()) {
    await user.click(screen.getByRole("button", { name: label }));
    expect(onSelectEffort).toHaveBeenLastCalledWith(
      models[0].model.reasoning_levels![index],
    );
  }
  rerender(
    <ModelPicker {...props} selectedKey={models[1].Key} reasoningEffort="" />,
  );
  expect(screen.queryByText("chat.reasoning")).not.toBeInTheDocument();
});

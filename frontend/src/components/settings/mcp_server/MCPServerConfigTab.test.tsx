import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { mcpserver, servercfg } from "@/lib/wailsjs/go/models";
import { mcpServerKeys } from "@/lib/queryKeys";
import MCPServerConfigTab from "./MCPServerConfigTab";
import SettingsDialog from "../SettingsDialog";

const api = vi.hoisted(() => ({
  GetMCPServerSettings: vi.fn(),
  SaveMCPServerSettings: vi.fn(),
  RotateMCPServerCredential: vi.fn(),
  RevokeMCPServerCredential: vi.fn(),
}));
vi.mock("@/lib/wailsjs/go/app/App", () => api);
vi.mock("../GeneralConfigTab", () => ({ default: () => null }));
vi.mock("../ModelConfigTab", () => ({ default: () => null }));
vi.mock("../AboutTab", () => ({ default: () => null }));

function makeView(
  config: Partial<servercfg.Local> = {},
  status: Partial<mcpserver.Status> = {},
  lastError = "",
) {
  return new mcpserver.LocalView({
    config: { enabled: false, port: 5108, allowed_tools: [], ...config },
    available_tools: ["edit", "get_chapter_list", "read", "delete_record"],
    token: "private-server-token",
    status: { running: false, stopping: false, url: "", ...status },
    last_error: lastError,
  });
}

let stored: mcpserver.LocalView;

function setup(element = <MCPServerConfigTab />) {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(<QueryClientProvider client={qc}>{element}</QueryClientProvider>);
  return { qc, user: userEvent.setup() };
}

async function loaded() {
  await screen.findByRole("switch", { name: "settings.mcpServer.enabled" });
}

function saveButton() {
  return screen.getByRole("button", { name: "settings.mcpServer.save" });
}

describe("local MCP server settings", () => {
  beforeEach(() => {
    vi.resetAllMocks();
    stored = makeView();
    api.GetMCPServerSettings.mockImplementation(() => Promise.resolve(stored));
    api.SaveMCPServerSettings.mockImplementation((input: servercfg.Local) => {
      stored = makeView(input, {
        running: input.enabled,
        url: input.enabled ? `http://127.0.0.1:${input.port}/mcp` : "",
      });
      return Promise.resolve(stored);
    });
  });

  it("opens the server controls from the settings navigation", async () => {
    const { user } = setup(<SettingsDialog open onClose={vi.fn()} />);
    expect(api.GetMCPServerSettings).not.toHaveBeenCalled();
    await user.click(
      screen.getByRole("button", { name: "settings.mcpServer.tab" }),
    );
    await loaded();
    expect(
      screen.getByRole("spinbutton", { name: "settings.mcpServer.port" }),
    ).toHaveValue(5108);
    expect(
      screen.getByText("tools.names.get_current_novel"),
    ).toBeInTheDocument();
    expect(screen.getByText("settings.mcpServer.builtin")).toBeInTheDocument();
    expect(
      screen.queryByRole("checkbox", { name: "tools.names.get_current_novel" }),
    ).not.toBeInTheDocument();
    expect(saveButton()).toBeDisabled();
    expect(screen.queryByText("private-server-token")).not.toBeInTheDocument();
  });

  it("saves draft port and tools when enabling and applies later edits through save", async () => {
    const { qc, user } = setup();
    await loaded();
    const port = screen.getByRole("spinbutton", {
      name: "settings.mcpServer.port",
    });
    await user.clear(port);
    await user.type(port, "5208");
    await user.click(
      screen.getByRole("checkbox", { name: "tools.names.get_chapter_list" }),
    );
    const enabled = screen.getByRole("switch", {
      name: "settings.mcpServer.enabled",
    });
    await user.click(enabled);
    await waitFor(() => expect(enabled).toBeChecked());
    expect(api.SaveMCPServerSettings).toHaveBeenLastCalledWith(
      expect.objectContaining({
        enabled: true,
        port: 5208,
        allowed_tools: ["get_chapter_list"],
      }),
    );
    await screen.findByText("settings.mcpServer.saved");
    expect(api.SaveMCPServerSettings).toHaveBeenCalledTimes(1);
    expect(saveButton()).toBeDisabled();
    expect(port).toHaveValue(5208);
    fireEvent.change(port, { target: { value: "5308" } });
    await user.click(
      screen.getByRole("checkbox", { name: "tools.names.read" }),
    );
    await waitFor(() => expect(saveButton()).toBeEnabled());
    await user.click(saveButton());
    await screen.findByText("settings.mcpServer.saved");
    expect(api.SaveMCPServerSettings).toHaveBeenLastCalledWith(
      expect.objectContaining({
        enabled: true,
        port: 5308,
        allowed_tools: ["get_chapter_list", "read"],
      }),
    );
    expect(
      qc.getQueryData<mcpserver.LocalView>(mcpServerKeys.all)?.config.port,
    ).toBe(5308);
    expect(screen.getByText("settings.mcpServer.running")).toBeInTheDocument();
    await user.click(enabled);
    await waitFor(() => expect(enabled).not.toBeChecked());
    expect(api.SaveMCPServerSettings).toHaveBeenLastCalledWith(
      expect.objectContaining({
        enabled: false,
        port: 5308,
        allowed_tools: ["get_chapter_list", "read"],
      }),
    );
    expect(screen.getByText("settings.mcpServer.stopped")).toBeInTheDocument();
  });

  it("navigates within the content while keeping drafts and a shared save action", async () => {
    const { user } = setup();
    await loaded();
    const navigation = within(
      screen.getByRole("navigation", {
        name: "settings.mcpServer.pageNavigation",
      }),
    );
    const content = screen.getByRole("region", {
      name: "settings.mcpServer.content",
    });
    const sectionOffsets = {
      connection: 0,
      credentials: 200,
      guide: 450,
      tools: 800,
    };
    vi.spyOn(content, "getBoundingClientRect").mockReturnValue({
      top: 100,
    } as DOMRect);
    for (const [id, offset] of Object.entries(sectionOffsets)) {
      const section = content.querySelector<HTMLElement>(
        `#mcp-server-section-${id}`,
      )!;
      vi.spyOn(section, "getBoundingClientRect").mockImplementation(
        () => ({ top: 100 + offset - content.scrollTop }) as DOMRect,
      );
    }
    const scrollTo = vi.fn(({ top }: ScrollToOptions) => {
      content.scrollTop = top ?? 0;
      fireEvent.scroll(content);
    });
    Object.defineProperty(content, "scrollTo", { value: scrollTo });
    expect(content).not.toContainElement(saveButton());
    expect(content).not.toContainElement(
      navigation.getByRole("button", {
        name: "settings.mcpServer.navTools",
      }),
    );

    const port = screen.getByRole("spinbutton", {
      name: "settings.mcpServer.port",
    });
    fireEvent.change(port, { target: { value: "6208" } });
    for (const [id, label] of [
      ["credentials", "navCredentials"],
      ["guide", "navGuide"],
      ["tools", "navTools"],
      ["connection", "navConnection"],
    ]) {
      const link = navigation.getByRole("button", {
        name: `settings.mcpServer.${label}`,
      });
      await user.click(link);
      expect(scrollTo).toHaveBeenLastCalledWith({
        top: sectionOffsets[id as keyof typeof sectionOffsets],
        behavior: "smooth",
      });
      expect(link).toHaveAttribute("aria-current", "location");
      expect(content.querySelector(`#mcp-server-section-${id}`)).toHaveFocus();
      expect(port).toHaveValue(6208);
    }
    expect(api.SaveMCPServerSettings).not.toHaveBeenCalled();
    expect(api.GetMCPServerSettings).toHaveBeenCalledTimes(1);
    await user.click(
      screen.getByRole("checkbox", { name: "tools.names.read" }),
    );
    content.scrollTop = sectionOffsets.credentials;
    fireEvent.scroll(content);
    expect(
      navigation.getByRole("button", {
        name: "settings.mcpServer.navCredentials",
      }),
    ).toHaveAttribute("aria-current", "location");
    expect(
      screen.getByRole("checkbox", { name: "tools.names.read" }),
    ).toBeChecked();
    await user.click(saveButton());
    await screen.findByText("settings.mcpServer.saved");
    expect(api.SaveMCPServerSettings).toHaveBeenLastCalledWith(
      expect.objectContaining({ port: 6208, allowed_tools: ["read"] }),
    );
  });

  it("marks the tools section when the content reaches the bottom", async () => {
    setup();
    await loaded();
    const content = screen.getByRole("region", {
      name: "settings.mcpServer.content",
    });
    Object.defineProperties(content, {
      scrollHeight: { value: 1200 },
      clientHeight: { value: 500 },
    });
    content.scrollTop = 700;
    fireEvent.scroll(content);
    const navigation = within(
      screen.getByRole("navigation", {
        name: "settings.mcpServer.pageNavigation",
      }),
    );
    expect(
      navigation.getByRole("button", {
        name: "settings.mcpServer.navTools",
      }),
    ).toHaveAttribute("aria-current", "location");
  });

  it.each(
    ["", "0", "1023", "32768", "65536", "5108.5"].flatMap((value) =>
      ["save", "enable"].map((action) => ({ value, action })),
    ),
  )(
    "rejects invalid port '$value' on $action without calling the backend",
    async ({ value, action }) => {
      const { user } = setup();
      await loaded();
      fireEvent.change(
        screen.getByRole("spinbutton", { name: "settings.mcpServer.port" }),
        { target: { value } },
      );
      await user.click(
        action === "save"
          ? saveButton()
          : screen.getByRole("switch", { name: "settings.mcpServer.enabled" }),
      );
      expect(screen.getByRole("alert")).toHaveTextContent(
        "settings.mcpServer.invalidPort",
      );
      expect(api.SaveMCPServerSettings).not.toHaveBeenCalled();
      expect(
        screen.getByRole("switch", { name: "settings.mcpServer.enabled" }),
      ).not.toBeChecked();
    },
  );

  it.each(["1024", "32767"])("accepts port boundary %s", async (value) => {
    const { user } = setup();
    await loaded();
    fireEvent.change(
      screen.getByRole("spinbutton", { name: "settings.mcpServer.port" }),
      { target: { value } },
    );
    await user.click(saveButton());
    await screen.findByText("settings.mcpServer.saved");
    expect(api.SaveMCPServerSettings).toHaveBeenLastCalledWith(
      expect.objectContaining({ port: Number(value) }),
    );
  });

  it("lets users explicitly expose read, edit and deletion tools", async () => {
    const { user } = setup();
    await loaded();
    for (const name of ["read", "edit", "delete_record"]) {
      const checkbox = screen.getByRole("checkbox", {
        name: `tools.names.${name}`,
      });
      expect(checkbox).not.toBeChecked();
      await user.click(checkbox);
    }
    expect(screen.queryByText("run_subagent")).not.toBeInTheDocument();
    await user.click(saveButton());
    await screen.findByText("settings.mcpServer.saved");
    expect(api.SaveMCPServerSettings).toHaveBeenLastCalledWith(
      expect.objectContaining({
        allowed_tools: ["read", "edit", "delete_record"],
      }),
    );
  });

  it("selects and clears a domain without changing other domains or adding unavailable tools", async () => {
    stored = makeView({ allowed_tools: ["read", "delete_record"] });
    const { user } = setup();
    await loaded();
    const files = within(
      screen.getByRole("group", { name: "tools.groups.files" }),
    );
    const groupCheckbox = files.getByRole("checkbox", {
      name: "settings.mcpServer.selectGroup",
    });
    expect(groupCheckbox).toBePartiallyChecked();
    expect(files.getByText("1/2")).toBeInTheDocument();
    await user.click(groupCheckbox);
    expect(groupCheckbox).toBeChecked();
    expect(files.getByText("2/2")).toBeInTheDocument();
    expect(
      screen.getByRole("checkbox", { name: "tools.names.delete_record" }),
    ).toBeChecked();
    expect(
      screen.getByRole("checkbox", { name: "tools.names.get_chapter_list" }),
    ).not.toBeChecked();
    expect(api.SaveMCPServerSettings).not.toHaveBeenCalled();
    await user.click(saveButton());
    await screen.findByText("settings.mcpServer.saved");
    expect(api.SaveMCPServerSettings).toHaveBeenLastCalledWith(
      expect.objectContaining({
        allowed_tools: ["read", "delete_record", "edit"],
      }),
    );
    await user.click(groupCheckbox);
    const confirmation = within(await screen.findByRole("alertdialog"));
    expect(groupCheckbox).toBeChecked();
    expect(api.SaveMCPServerSettings).toHaveBeenCalledTimes(1);
    await user.click(
      confirmation.getByRole("button", {
        name: "settings.mcpServer.disableCoreTools",
      }),
    );
    expect(groupCheckbox).not.toBeChecked();
    expect(files.getByText("0/2")).toBeInTheDocument();
    expect(api.SaveMCPServerSettings).toHaveBeenCalledTimes(1);
    await user.click(saveButton());
    await screen.findByText("settings.mcpServer.saved");
    expect(api.SaveMCPServerSettings).toHaveBeenLastCalledWith(
      expect.objectContaining({ allowed_tools: ["delete_record"] }),
    );
  });

  it("disables group selection while applying settings", async () => {
    let complete: (view: mcpserver.LocalView) => void;
    api.SaveMCPServerSettings.mockImplementationOnce(
      () =>
        new Promise<mcpserver.LocalView>((resolve) => {
          complete = resolve;
        }),
    );
    const { user } = setup();
    await loaded();
    await user.click(
      screen.getByRole("checkbox", { name: "tools.names.read" }),
    );
    await user.click(saveButton());
    for (const checkbox of screen.getAllByRole("checkbox")) {
      expect(checkbox).toBeDisabled();
    }
    stored = makeView({ allowed_tools: ["read"] });
    complete!(stored);
    await screen.findByText("settings.mcpServer.saved");
    expect(
      screen.getByRole("checkbox", { name: "tools.names.read" }),
    ).toBeEnabled();
  });

  it("restores the switch after a start failure and allows enabling again without losing drafts", async () => {
    stored = makeView({ allowed_tools: ["read"] });
    const { qc, user } = setup();
    await loaded();
    const port = screen.getByRole("spinbutton", {
      name: "settings.mcpServer.port",
    });
    fireEvent.change(port, { target: { value: "5308" } });
    await user.click(
      screen.getByRole("checkbox", { name: "tools.names.get_chapter_list" }),
    );
    api.SaveMCPServerSettings.mockImplementationOnce(
      (input: servercfg.Local) => {
        stored = makeView(input, {}, "port occupied");
        return Promise.reject("port occupied");
      },
    );
    await user.click(
      screen.getByRole("switch", { name: "settings.mcpServer.enabled" }),
    );
    await waitFor(() =>
      expect(screen.getByRole("alert")).toHaveTextContent("port occupied"),
    );
    const enabled = screen.getByRole("switch", {
      name: "settings.mcpServer.enabled",
    });
    await waitFor(() => {
      expect(enabled).not.toBeChecked();
      expect(enabled).toBeEnabled();
    });
    expect(api.SaveMCPServerSettings).toHaveBeenNthCalledWith(
      2,
      expect.objectContaining({
        enabled: false,
        port: 5308,
        allowed_tools: ["read", "get_chapter_list"],
      }),
    );
    expect(api.SaveMCPServerSettings).toHaveBeenCalledTimes(2);
    expect(screen.getByRole("alert")).toHaveTextContent("port occupied");
    expect(screen.getByText("settings.mcpServer.stopped")).toBeInTheDocument();
    expect(
      screen.queryByText("settings.mcpServer.enabledNotRunning"),
    ).not.toBeInTheDocument();
    expect(
      qc.getQueryData<mcpserver.LocalView>(mcpServerKeys.all)?.config.enabled,
    ).toBe(false);
    expect(port).toHaveValue(5308);
    expect(
      screen.getByRole("checkbox", { name: "tools.names.get_chapter_list" }),
    ).toBeChecked();
    expect(
      screen.queryByText("settings.mcpServer.saved"),
    ).not.toBeInTheDocument();
    await user.click(enabled);
    await waitFor(() => expect(enabled).toBeChecked());
    expect(api.SaveMCPServerSettings).toHaveBeenLastCalledWith(
      expect.objectContaining({
        enabled: true,
        port: 5308,
        allowed_tools: ["read", "get_chapter_list"],
      }),
    );
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(screen.getByText("settings.mcpServer.running")).toBeInTheDocument();
  });

  it("retries a failed start using the edited port without clicking save", async () => {
    api.SaveMCPServerSettings.mockImplementationOnce(
      (input: servercfg.Local) => {
        stored = makeView(input, {}, "port occupied");
        return Promise.reject("port occupied");
      },
    );
    const { qc, user } = setup();
    await loaded();
    const enabled = screen.getByRole("switch", {
      name: "settings.mcpServer.enabled",
    });
    await user.click(enabled);
    await waitFor(() => {
      expect(api.SaveMCPServerSettings).toHaveBeenCalledTimes(2);
      expect(enabled).not.toBeChecked();
      expect(enabled).toBeEnabled();
    });
    expect(api.SaveMCPServerSettings).toHaveBeenNthCalledWith(
      1,
      expect.objectContaining({ enabled: true, port: 5108 }),
    );
    fireEvent.change(
      screen.getByRole("spinbutton", { name: "settings.mcpServer.port" }),
      { target: { value: "5109" } },
    );
    await user.click(enabled);
    await screen.findByText("settings.mcpServer.saved");
    expect(api.SaveMCPServerSettings).toHaveBeenCalledTimes(3);
    expect(api.SaveMCPServerSettings).toHaveBeenLastCalledWith(
      expect.objectContaining({ enabled: true, port: 5109 }),
    );
    expect(
      qc.getQueryData<mcpserver.LocalView>(mcpServerKeys.all)?.config,
    ).toEqual(expect.objectContaining({ enabled: true, port: 5109 }));
    expect(enabled).toBeChecked();
    expect(saveButton()).toBeDisabled();
    expect(screen.getByText("settings.mcpServer.running")).toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("stops using saved settings even when drafts are invalid or unsubmitted", async () => {
    stored = makeView(
      { enabled: true, allowed_tools: ["read"] },
      { running: true },
    );
    const { user } = setup();
    await loaded();
    const port = screen.getByRole("spinbutton", {
      name: "settings.mcpServer.port",
    });
    fireEvent.change(port, { target: { value: "0" } });
    await user.click(
      screen.getByRole("checkbox", { name: "tools.names.get_chapter_list" }),
    );
    const enabled = screen.getByRole("switch", {
      name: "settings.mcpServer.enabled",
    });
    await user.click(enabled);
    await waitFor(() => expect(enabled).not.toBeChecked());
    expect(api.SaveMCPServerSettings).toHaveBeenCalledTimes(1);
    expect(api.SaveMCPServerSettings).toHaveBeenLastCalledWith(
      expect.objectContaining({
        enabled: false,
        port: 5108,
        allowed_tools: ["read"],
      }),
    );
    expect(port).toHaveValue(0);
    expect(
      screen.getByRole("checkbox", { name: "tools.names.get_chapter_list" }),
    ).toBeChecked();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(screen.getByText("settings.mcpServer.stopped")).toBeInTheDocument();
  });

  it("keeps controls disabled until restoring the switch finishes", async () => {
    api.SaveMCPServerSettings.mockImplementationOnce(
      (input: servercfg.Local) => {
        stored = makeView(input, {}, "port occupied");
        return Promise.reject("port occupied");
      },
    );
    let complete!: (view: mcpserver.LocalView) => void;
    api.SaveMCPServerSettings.mockImplementationOnce(
      () =>
        new Promise<mcpserver.LocalView>((resolve) => {
          complete = resolve;
        }),
    );
    const { user } = setup();
    await loaded();
    const enabled = screen.getByRole("switch", {
      name: "settings.mcpServer.enabled",
    });
    await user.click(enabled);
    await waitFor(() =>
      expect(api.SaveMCPServerSettings).toHaveBeenCalledTimes(2),
    );
    expect(enabled).toBeDisabled();
    expect(
      screen.getByRole("spinbutton", { name: "settings.mcpServer.port" }),
    ).toBeDisabled();
    expect(
      screen.getByRole("button", { name: "settings.mcpServer.refresh" }),
    ).toBeDisabled();
    await user.click(enabled);
    expect(api.SaveMCPServerSettings).toHaveBeenCalledTimes(2);
    stored = makeView();
    complete(stored);
    await waitFor(() => {
      expect(enabled).not.toBeChecked();
      expect(enabled).toBeEnabled();
    });
    expect(screen.getByRole("alert")).toHaveTextContent("port occupied");
  });

  it.each([false, true])(
    "reports restore failures and shows the persisted enabled state (%s)",
    async (persistedEnabled) => {
      api.SaveMCPServerSettings.mockImplementationOnce(
        (input: servercfg.Local) => {
          stored = makeView(input, {}, "port occupied");
          return Promise.reject("port occupied");
        },
      );
      api.SaveMCPServerSettings.mockImplementationOnce(() => {
        stored = makeView({ enabled: persistedEnabled }, {}, "restore failed");
        return Promise.reject("restore failed");
      });
      const { qc, user } = setup();
      await loaded();
      const enabled = screen.getByRole("switch", {
        name: "settings.mcpServer.enabled",
      });
      await user.click(enabled);
      await waitFor(() => {
        expect(screen.getByRole("alert")).toHaveTextContent("port occupied");
        expect(screen.getByRole("alert")).toHaveTextContent("restore failed");
        expect(enabled).toBeEnabled();
      });
      expect(enabled).toHaveProperty("checked", persistedEnabled);
      expect(
        qc.getQueryData<mcpserver.LocalView>(mcpServerKeys.all)?.config.enabled,
      ).toBe(persistedEnabled);
      expect(api.SaveMCPServerSettings).toHaveBeenCalledTimes(2);
    },
  );

  it("does not retry a failed stop automatically", async () => {
    stored = makeView({ enabled: true }, { running: true });
    api.SaveMCPServerSettings.mockRejectedValueOnce("stop failed");
    const { user } = setup();
    await loaded();
    const enabled = screen.getByRole("switch", {
      name: "settings.mcpServer.enabled",
    });
    await user.click(enabled);
    await waitFor(() => {
      expect(screen.getByRole("alert")).toHaveTextContent("stop failed");
      expect(enabled).toBeEnabled();
    });
    expect(enabled).toBeChecked();
    expect(api.SaveMCPServerSettings).toHaveBeenCalledTimes(1);
  });

  it("keeps unsaved fields when the runtime status is refreshed", async () => {
    const { user } = setup();
    await loaded();
    const port = screen.getByRole("spinbutton", {
      name: "settings.mcpServer.port",
    });
    await user.clear(port);
    await user.type(port, "5308");
    await user.click(
      screen.getByRole("checkbox", { name: "tools.names.get_chapter_list" }),
    );
    stored = makeView({ port: 6108 });
    await user.click(
      screen.getByRole("button", { name: "settings.mcpServer.refresh" }),
    );
    await waitFor(() =>
      expect(api.GetMCPServerSettings).toHaveBeenCalledTimes(2),
    );
    expect(port).toHaveValue(5308);
    expect(
      screen.getByRole("checkbox", { name: "tools.names.get_chapter_list" }),
    ).toBeChecked();
    expect(api.SaveMCPServerSettings).not.toHaveBeenCalled();
  });

  it("displays configuration errors returned with a successful settings query", async () => {
    stored = makeView({}, {}, "MCP configuration decrypt failed");
    setup();
    await loaded();
    expect(screen.getByRole("alert")).toHaveTextContent(
      "MCP configuration decrypt failed",
    );
    expect(
      screen.getByRole("switch", { name: "settings.mcpServer.enabled" }),
    ).not.toBeChecked();
    expect(screen.getByText("settings.mcpServer.stopped")).toBeInTheDocument();
    expect(saveButton()).toBeEnabled();
  });

  it("recovers from a query failure using the refresh action", async () => {
    api.GetMCPServerSettings.mockRejectedValueOnce("settings unavailable");
    const { user } = setup();
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "settings unavailable",
    );
    await user.click(
      screen.getByRole("button", { name: "settings.mcpServer.refresh" }),
    );
    await loaded();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("refreshes while shutdown is draining and stops polling once it completes", async () => {
    api.GetMCPServerSettings.mockResolvedValueOnce(
      makeView({}, { stopping: true }),
    );
    setup();
    await loaded();
    expect(screen.getByText("settings.mcpServer.stopping")).toBeInTheDocument();
    await waitFor(
      () =>
        expect(
          screen.getByText("settings.mcpServer.stopped"),
        ).toBeInTheDocument(),
      { timeout: 2500 },
    );
    expect(api.GetMCPServerSettings).toHaveBeenCalledTimes(2);
    expect(api.SaveMCPServerSettings).not.toHaveBeenCalled();
  });
});

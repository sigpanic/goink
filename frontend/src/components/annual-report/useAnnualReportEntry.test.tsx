import { StrictMode, type ReactNode } from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  GetSettings,
  MarkAnnualReportPrompted,
} from "@/lib/wailsjs/go/app/App";
import { Environment } from "@/lib/wailsjs/runtime/runtime";
import { config } from "@/lib/wailsjs/go/models";
import { toastError } from "@/utils/toast";
import { useAnnualReportEntry } from "./useAnnualReportEntry";

vi.mock("@/lib/wailsjs/go/app/App", () => ({
  GetSettings: vi.fn(),
  MarkAnnualReportPrompted: vi.fn(),
}));
vi.mock("@/lib/wailsjs/runtime/runtime", () => ({ Environment: vi.fn() }));
vi.mock("@/utils/toast", () => ({ toastError: vi.fn() }));

let savedYear = 0;

function mount(blocked = false) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  return renderHook(({ blocked }) => useAnnualReportEntry(blocked), {
    initialProps: { blocked },
    wrapper: ({ children }: { children: ReactNode }) => (
      <StrictMode>
        <QueryClientProvider client={client}>{children}</QueryClientProvider>
      </StrictMode>
    ),
  });
}

beforeEach(() => {
  vi.resetAllMocks();
  vi.stubEnv("DEV", false);
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date(2026, 11, 29, 12));
  savedYear = 0;
  vi.mocked(Environment).mockResolvedValue({
    buildType: "production",
    platform: "linux",
    arch: "amd64",
  });
  vi.mocked(GetSettings).mockImplementation(
    async () =>
      new config.AppSettings({ annual_report_prompt_year: savedYear }),
  );
  vi.mocked(MarkAnnualReportPrompted).mockImplementation(async (year) => {
    savedYear = year;
  });
});

afterEach(() => {
  vi.unstubAllEnvs();
  vi.useRealTimers();
});

describe("annual report entry", () => {
  it("records the displayed prompt once in StrictMode and keeps it open after settings refresh", async () => {
    const view = mount();
    await waitFor(() => expect(view.result.current.promptOpen).toBe(true));
    await waitFor(() => expect(GetSettings).toHaveBeenCalledTimes(2));
    expect(MarkAnnualReportPrompted).toHaveBeenCalledExactlyOnceWith(2026);
    expect(view.result.current.promptOpen).toBe(true);
    act(() => view.result.current.dismissPrompt());
    expect(view.result.current.promptOpen).toBe(false);
    view.rerender({ blocked: false });
    expect(view.result.current.promptOpen).toBe(false);
  });

  it("uses the backend record after a restart, skips January and reminds for the next report", async () => {
    const first = mount();
    await waitFor(() => expect(savedYear).toBe(2026));
    first.unmount();
    localStorage.clear();
    vi.setSystemTime(new Date(2027, 0, 1));
    const january = mount();
    await waitFor(() => expect(GetSettings).toHaveBeenCalledTimes(3));
    expect(january.result.current.promptOpen).toBe(false);
    january.unmount();
    vi.setSystemTime(new Date(2027, 11, 29));
    const next = mount();
    await waitFor(() => expect(next.result.current.promptOpen).toBe(true));
    expect(MarkAnnualReportPrompted).toHaveBeenLastCalledWith(2027);
  });

  it("waits for startup dialogs and does not consume the reminder while blocked", async () => {
    const view = mount(true);
    await waitFor(() => expect(GetSettings).toHaveBeenCalledOnce());
    expect(view.result.current.promptOpen).toBe(false);
    expect(MarkAnnualReportPrompted).not.toHaveBeenCalled();
    view.rerender({ blocked: false });
    await waitFor(() => expect(view.result.current.promptOpen).toBe(true));
    view.rerender({ blocked: true });
    expect(view.result.current.promptOpen).toBe(false);
    view.rerender({ blocked: false });
    expect(view.result.current.promptOpen).toBe(true);
    expect(MarkAnnualReportPrompted).toHaveBeenCalledOnce();
  });

  it("opens the report without reopening the reminder and keeps it open when an update arrives", async () => {
    const view = mount();
    await waitFor(() => expect(view.result.current.promptOpen).toBe(true));
    act(() => view.result.current.openReport());
    expect(view.result.current.promptOpen).toBe(false);
    expect(view.result.current.reportOpen).toBe(true);
    view.rerender({ blocked: true });
    expect(view.result.current.reportOpen).toBe(true);
    act(() => view.result.current.closeReport());
    view.rerender({ blocked: false });
    expect(view.result.current.promptOpen).toBe(false);
  });

  it.each(["vite", "wails"])(
    "prompts once in %s development outside the season and persists across launches",
    async (mode) => {
      vi.setSystemTime(new Date(2026, 6, 1));
      vi.stubEnv("DEV", mode === "vite");
      vi.mocked(Environment).mockResolvedValue({
        buildType: "dev",
        platform: "linux",
        arch: "amd64",
      });
      const view = mount();
      await waitFor(() => expect(view.result.current.promptOpen).toBe(true));
      await waitFor(() => expect(GetSettings).toHaveBeenCalledTimes(2));
      expect(MarkAnnualReportPrompted).toHaveBeenCalledExactlyOnceWith(2026);
      act(() => view.result.current.openReport());
      expect(view.result.current.reportOpen).toBe(true);
      expect(view.result.current.promptOpen).toBe(false);
      act(() => view.result.current.closeReport());
      expect(view.result.current.promptOpen).toBe(false);
      view.unmount();
      const next = mount();
      await waitFor(() => expect(GetSettings).toHaveBeenCalledTimes(3));
      expect(next.result.current.promptOpen).toBe(false);
      expect(MarkAnnualReportPrompted).toHaveBeenCalledOnce();
    },
  );

  it("still allows viewing after a save failure and retries on the next launch", async () => {
    vi.mocked(MarkAnnualReportPrompted).mockRejectedValue(
      new Error("database unavailable"),
    );
    const first = mount();
    await waitFor(() => expect(toastError).toHaveBeenCalled());
    expect(first.result.current.promptOpen).toBe(true);
    act(() => first.result.current.openReport());
    expect(first.result.current.reportOpen).toBe(true);
    expect(MarkAnnualReportPrompted).toHaveBeenCalledOnce();
    first.unmount();
    const next = mount();
    await waitFor(() => expect(next.result.current.promptOpen).toBe(true));
    expect(MarkAnnualReportPrompted).toHaveBeenCalledTimes(2);
  });

  it("does not mark the reminder when settings could not be read", async () => {
    vi.mocked(GetSettings).mockRejectedValue(new Error("database unavailable"));
    const view = mount();
    await waitFor(() => expect(GetSettings).toHaveBeenCalledOnce());
    expect(view.result.current.promptOpen).toBe(false);
    expect(MarkAnnualReportPrompted).not.toHaveBeenCalled();
  });
});

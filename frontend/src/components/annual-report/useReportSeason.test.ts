import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Environment } from "@/lib/wailsjs/runtime/runtime";
import {
  defaultReportYear,
  isAnnualReportAvailable,
  useReportSeason,
} from "./useReportSeason";

vi.mock("@/lib/wailsjs/runtime/runtime", () => ({ Environment: vi.fn() }));

beforeEach(() => {
  vi.mocked(Environment).mockReset().mockResolvedValue({
    buildType: "production",
    platform: "linux",
    arch: "amd64",
  });
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllEnvs();
});

describe("annual report season", () => {
  it.each([
    [2026, 0, 1, false],
    [2026, 11, 28, false],
    [2026, 11, 29, true],
    [2026, 11, 31, true],
    [2027, 0, 1, true],
    [2027, 0, 15, true],
    [2027, 0, 16, false],
    [2027, 6, 1, false],
    [2027, 11, 28, false],
    [2027, 11, 29, true],
    [2028, 0, 15, true],
    [2028, 0, 16, false],
  ])(
    "production availability on %i/%i/%i is %s",
    (year, month, day, expected) => {
      expect(isAnnualReportAvailable(new Date(year, month, day), false)).toBe(
        expected,
      );
      expect(
        isAnnualReportAvailable(new Date(year, month, day, 23, 59, 59), false),
      ).toBe(expected);
    },
  );

  it("makes development previews available outside the production season", () => {
    vi.stubEnv("DEV", true);
    expect(isAnnualReportAvailable(new Date(2026, 6, 1))).toBe(true);
    vi.stubEnv("DEV", false);
    expect(isAnnualReportAvailable(new Date(2026, 6, 1))).toBe(false);
  });

  it("selects the same report on either side of New Year", () => {
    expect(defaultReportYear(new Date(2026, 11, 29))).toBe(2026);
    expect(defaultReportYear(new Date(2027, 0, 15))).toBe(2026);
    expect(defaultReportYear(new Date(2027, 11, 29))).toBe(2027);
  });

  it("opens outside the season when Wails serves a production frontend in dev mode", async () => {
    vi.stubEnv("DEV", false);
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date(2026, 6, 1));
    vi.mocked(Environment).mockResolvedValue({
      buildType: "dev",
      platform: "linux",
      arch: "amd64",
    });
    const view = renderHook(() => useReportSeason());
    await waitFor(() => expect(view.result.current.available).toBe(true));
    expect(Environment).toHaveBeenCalledOnce();
  });

  it.each(["production", "debug"])(
    "keeps the season restriction for Wails %s builds",
    async (buildType) => {
      vi.stubEnv("DEV", false);
      vi.useFakeTimers({ toFake: ["Date"] });
      vi.setSystemTime(new Date(2026, 6, 1));
      vi.mocked(Environment).mockResolvedValue({
        buildType,
        platform: "linux",
        arch: "amd64",
      });
      const view = renderHook(() => useReportSeason());
      await act(async () => {
        await Promise.resolve();
      });
      expect(Environment).toHaveBeenCalledOnce();
      expect(view.result.current.available).toBe(false);
    },
  );

  it("uses the date restriction when runtime detection is unavailable", async () => {
    vi.stubEnv("DEV", false);
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date(2026, 6, 1));
    vi.mocked(Environment).mockRejectedValue(new Error("runtime unavailable"));
    const view = renderHook(() => useReportSeason());
    await act(async () => {
      await Promise.resolve();
    });
    expect(view.result.current.available).toBe(false);
  });

  it("updates an open app when the local opening and closing dates arrive", () => {
    vi.stubEnv("DEV", false);
    vi.useFakeTimers();
    vi.setSystemTime(new Date(2026, 11, 28, 23, 59, 59));
    const opening = renderHook(() => useReportSeason());
    expect(opening.result.current.available).toBe(false);
    act(() => vi.advanceTimersByTime(1000));
    expect(opening.result.current.available).toBe(true);
    opening.unmount();

    vi.setSystemTime(new Date(2027, 0, 15, 23, 59, 59));
    const closing = renderHook(() => useReportSeason());
    expect(closing.result.current).toEqual({
      available: true,
      reportYear: 2026,
    });
    act(() => vi.advanceTimersByTime(1000));
    expect(closing.result.current.available).toBe(false);
    closing.unmount();
    expect(vi.getTimerCount()).toBe(0);
  });
});

import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { useTimeAgo } from "./useTimeAgo";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, options?: { count: number }) =>
      options ? `${key}:${options.count}` : key,
    i18n: { language: "zh-CN" },
  }),
}));

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date(2026, 10, 1, 12));
});

afterEach(() => {
  vi.useRealTimers();
});

it.each([
  [new Date(2026, 10, 1, 11, 59, 45), "time.justNow"],
  [new Date(2026, 10, 1, 11, 55), "time.minutesAgo:5"],
  [new Date(2026, 10, 1, 10), "time.hoursAgo:2"],
  [new Date(2026, 9, 31, 23), "time.hoursAgo:13"],
  [new Date(2026, 9, 30, 12), "time.daysAgo:2"],
  [new Date(2026, 8, 2, 12), "time.monthsAgo:2"],
])("keeps default relative formatting for %s", (date, expected) => {
  const { result } = renderHook(() => useTimeAgo());
  expect(result.current(date.toISOString())).toBe(expected);
});

it("shows a 24-hour clock for messages on the current local calendar day", () => {
  const { result } = renderHook(() => useTimeAgo(true, "message"));
  expect(result.current(new Date(2026, 10, 1, 0, 5).toISOString())).toBe(
    "00:05",
  );
  expect(result.current(new Date(2026, 10, 1, 11, 55).toISOString())).toBe(
    "11:55",
  );
});

it.each([
  [new Date(2026, 9, 31, 23), "time.hoursAgo:13"],
  [new Date(2026, 9, 30, 12), "time.daysAgo:2"],
  [new Date(2026, 8, 2, 12), "time.monthsAgo:2"],
  [new Date(2025, 10, 1, 12), "time.monthsAgo:12"],
])("uses relative formatting for older messages from %s", (date, expected) => {
  const { result } = renderHook(() => useTimeAgo(true, "message"));
  expect(result.current(date.toISOString())).toBe(expected);
});

it("switches messages to relative time after local midnight", () => {
  vi.setSystemTime(new Date(2026, 10, 1, 23, 59, 30));
  const sentAt = new Date(2026, 10, 1, 22).toISOString();
  const { result } = renderHook(() => useTimeAgo(true, "message"));
  expect(result.current(sentAt)).toBe("22:00");

  act(() => vi.advanceTimersByTime(60_000));
  expect(result.current(sentAt)).toBe("time.hoursAgo:2");
});

it("continues refreshing default relative times every minute", () => {
  const sentAt = new Date(2026, 10, 1, 11, 59, 45).toISOString();
  const { result } = renderHook(() => useTimeAgo());
  expect(result.current(sentAt)).toBe("time.justNow");

  act(() => vi.advanceTimersByTime(60_000));
  expect(result.current(sentAt)).toBe("time.minutesAgo:1");
});

it("stops refreshing while disabled and cleans up on unmount", () => {
  const sentAt = new Date(2026, 10, 1, 11, 55).toISOString();
  const { result, rerender, unmount } = renderHook(
    ({ enabled }) => useTimeAgo(enabled),
    { initialProps: { enabled: false } },
  );
  expect(vi.getTimerCount()).toBe(0);
  act(() => vi.advanceTimersByTime(60_000));
  expect(result.current(sentAt)).toBe("time.minutesAgo:5");

  rerender({ enabled: true });
  act(() => vi.advanceTimersByTime(60_000));
  expect(result.current(sentAt)).toBe("time.minutesAgo:7");

  rerender({ enabled: false });
  expect(vi.getTimerCount()).toBe(0);
  rerender({ enabled: true });
  unmount();
  expect(vi.getTimerCount()).toBe(0);
});

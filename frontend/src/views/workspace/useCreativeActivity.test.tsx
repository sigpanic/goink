import { StrictMode } from "react";
import { act, cleanup, fireEvent, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { RecordCreativeActivity } from "@/lib/wailsjs/go/app/App";
import { useCreativeActivity } from "./useCreativeActivity";

vi.mock("@/lib/wailsjs/go/app/App", () => ({
  RecordCreativeActivity: vi.fn(),
}));

const record = vi.mocked(RecordCreativeActivity);

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-10-10T12:00:00"));
  vi.spyOn(document, "hasFocus").mockReturnValue(true);
  vi.spyOn(document, "visibilityState", "get").mockReturnValue("visible");
  record.mockReset().mockResolvedValue(undefined);
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

async function advance(ms: number) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms);
  });
}

it("交互后按分钟记录，重新渲染和持续交互不会重复启动计时", async () => {
  const { rerender } = renderHook(() => useCreativeActivity());
  await advance(60_000);
  expect(record).not.toHaveBeenCalled();
  fireEvent.keyDown(document);
  await advance(60_000);
  expect(record).toHaveBeenCalledExactlyOnceWith(60);
  rerender();
  fireEvent.pointerMove(document);
  await advance(60_000);
  expect(record.mock.calls).toEqual([[60], [60]]);
});

it("闲置五分钟暂停，重新交互不会补记闲置时间", async () => {
  renderHook(() => useCreativeActivity());
  fireEvent.pointerDown(document);
  await advance(20_000);
  fireEvent.pointerMove(document);
  await advance(7 * 60_000);
  expect(record.mock.calls).toEqual([[60], [60], [60], [60], [60], [20]]);
  fireEvent.wheel(document);
  await advance(40_000);
  expect(record).toHaveBeenLastCalledWith(40);
  await advance(60_000);
  expect(record).toHaveBeenCalledTimes(8);
  expect(record).toHaveBeenLastCalledWith(60);
});

it.each(["隐藏", "失焦"])(
  "%s时保留已有尾段，恢复后继续累计且不补记暂停时间",
  async (reason) => {
    renderHook(() => useCreativeActivity());
    fireEvent.keyDown(document);
    await advance(20_000);
    if (reason === "隐藏") {
      vi.spyOn(document, "visibilityState", "get").mockReturnValue("hidden");
      fireEvent(document, new Event("visibilitychange"));
    } else {
      fireEvent(window, new Event("blur"));
    }
    fireEvent.pointerMove(document);
    await advance(3 * 60_000);
    expect(record).toHaveBeenCalledExactlyOnceWith(20);
    if (reason === "隐藏") {
      vi.spyOn(document, "visibilityState", "get").mockReturnValue("visible");
      fireEvent(document, new Event("visibilitychange"));
    } else {
      fireEvent(window, new Event("focus"));
    }
    fireEvent.keyDown(document);
    await advance(40_000);
    expect(record.mock.calls).toEqual([[20], [40]]);
  },
);

it("短片段切换先累计再取整，不丢失尾段或跨分钟的小数秒", async () => {
  renderHook(() => useCreativeActivity());
  fireEvent.keyDown(document);
  await advance(20_400);
  fireEvent(window, new Event("blur"));
  await advance(10_000);
  fireEvent(window, new Event("focus"));
  await advance(10_400);
  fireEvent(window, new Event("blur"));
  expect(record).not.toHaveBeenCalled();
  await advance(19_200);
  expect(record).toHaveBeenCalledExactlyOnceWith(30);
  fireEvent(window, new Event("focus"));
  await advance(400);
  fireEvent(window, new Event("blur"));
  await advance(59_600);
  expect(record.mock.calls).toEqual([[30], [1]]);
});

it("计时延迟最多报一分钟，长时间暂停后不补记", async () => {
  renderHook(() => useCreativeActivity());
  fireEvent.keyDown(document);
  vi.setSystemTime(Date.now() + 90_000);
  await advance(60_000);
  expect(record).toHaveBeenCalledExactlyOnceWith(60);
  vi.setSystemTime(Date.now() + 60 * 60_000);
  await advance(60_000);
  expect(record).toHaveBeenCalledTimes(1);
  fireEvent.keyDown(document);
  await advance(60_000);
  expect(record.mock.calls).toEqual([[60], [60]]);
});

it("StrictMode只保留一个计时器，失败不补发，卸载清理监听且不补记尾段", async () => {
  const removeDocumentListener = vi.spyOn(document, "removeEventListener");
  const removeWindowListener = vi.spyOn(window, "removeEventListener");
  const { unmount } = renderHook(() => useCreativeActivity(), {
    wrapper: ({ children }) => <StrictMode>{children}</StrictMode>,
  });
  expect(vi.getTimerCount()).toBe(1);
  record.mockRejectedValueOnce(new Error("统计不可用"));
  fireEvent.keyDown(document);
  await advance(60_000);
  expect(record).toHaveBeenCalledTimes(1);
  await advance(60_000);
  expect(record.mock.calls).toEqual([[60], [60]]);
  await advance(20_000);
  unmount();
  expect(vi.getTimerCount()).toBe(0);
  for (const event of ["keydown", "pointerdown", "pointermove", "wheel"]) {
    expect(removeDocumentListener).toHaveBeenCalledWith(
      event,
      expect.any(Function),
      { capture: true, passive: true },
    );
  }
  expect(removeDocumentListener).toHaveBeenCalledWith(
    "visibilitychange",
    expect.any(Function),
  );
  for (const event of ["focus", "blur"]) {
    expect(removeWindowListener).toHaveBeenCalledWith(
      event,
      expect.any(Function),
    );
  }
  fireEvent.keyDown(document);
  await advance(60_000);
  expect(record).toHaveBeenCalledTimes(2);
});

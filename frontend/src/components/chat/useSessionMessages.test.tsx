import { beforeEach, expect, it, vi } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { PropsWithChildren } from "react";
import { session } from "@/lib/wailsjs/go/models";
import { useSessionMessages } from "./useSessionMessages";

const api = vi.hoisted(() => ({ page: vi.fn() }));

vi.mock("@/lib/wailsjs/go/app/App", () => ({
  GetSessionMessagesPage: api.page,
}));

beforeEach(() => api.page.mockReset());

function page(turnIds: number[], next = 0) {
  return new session.MessagePage({
    items: turnIds.flatMap((turnId) => [
      new session.Message({ id: turnId * 2, turn_id: turnId, role: "user" }),
      new session.Message({
        id: turnId * 2 + 1,
        turn_id: turnId,
        role: "assistant",
      }),
    ]),
    has_more: next > 0,
    next_before_turn_id: next,
  });
}

function setup(sessionId = "session") {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: Infinity } },
  });
  const wrapper = ({ children }: PropsWithChildren) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  return renderHook(({ id }) => useSessionMessages(id), {
    initialProps: { id: sessionId },
    wrapper,
  });
}

it("requests whole turn pages with an exclusive cursor and combines them chronologically", async () => {
  const latest = page([3, 4], 3);
  const earlier = page([1, 2]);
  api.page.mockResolvedValueOnce(latest).mockResolvedValueOnce(earlier);
  const { result } = setup();
  await waitFor(() => expect(result.current.isSuccess).toBe(true));
  expect(api.page).toHaveBeenNthCalledWith(1, {
    session_id: "session",
    before_turn_id: 0,
    size: 20,
  });
  expect(result.current.hasNextPage).toBe(true);
  await act(async () => {
    await result.current.fetchNextPage();
  });
  await waitFor(() => expect(result.current.data).toHaveLength(8));
  expect(api.page).toHaveBeenNthCalledWith(2, {
    session_id: "session",
    before_turn_id: 3,
    size: 20,
  });
  expect(result.current.data?.map((message) => message.turn_id)).toEqual([
    1, 1, 2, 2, 3, 3, 4, 4,
  ]);
  expect(result.current.hasNextPage).toBe(false);
  expect(latest.items.map((message) => message.turn_id)).toEqual([3, 3, 4, 4]);
});

it("does not fetch for a new conversation and stops at an empty history", async () => {
  api.page.mockResolvedValue(page([]));
  const { result, rerender } = setup("");
  expect(api.page).not.toHaveBeenCalled();
  rerender({ id: "empty" });
  await waitFor(() => expect(result.current.isSuccess).toBe(true));
  expect(result.current.data).toEqual([]);
  expect(result.current.hasNextPage).toBe(false);
});

it("keeps loaded messages when an older page fails and retries the same cursor", async () => {
  api.page
    .mockResolvedValueOnce(page([2], 2))
    .mockRejectedValueOnce(new Error("offline"))
    .mockResolvedValueOnce(page([1]));
  const { result } = setup();
  await waitFor(() => expect(result.current.isSuccess).toBe(true));
  await act(async () => {
    await result.current.fetchNextPage();
  });
  await waitFor(() => expect(result.current.isFetchNextPageError).toBe(true));
  expect(result.current.data?.map((message) => message.turn_id)).toEqual([
    2, 2,
  ]);
  await act(async () => {
    await result.current.fetchNextPage();
  });
  await waitFor(() => expect(result.current.data).toHaveLength(4));
  expect(api.page.mock.calls[1]).toEqual(api.page.mock.calls[2]);
});

it("does not mix sessions when the previous session request finishes late", async () => {
  let finish!: (value: session.MessagePage) => void;
  api.page
    .mockImplementationOnce(
      () =>
        new Promise<session.MessagePage>((resolve) => {
          finish = resolve;
        }),
    )
    .mockResolvedValueOnce(page([9]));
  const { result, rerender } = setup("first");
  await waitFor(() => expect(api.page).toHaveBeenCalledTimes(1));
  rerender({ id: "second" });
  await waitFor(() => expect(result.current.data?.[0].turn_id).toBe(9));
  await act(async () => finish(page([1])));
  expect(result.current.data?.map((message) => message.turn_id)).toEqual([
    9, 9,
  ]);
  expect(api.page.mock.calls[1][0].session_id).toBe("second");
});

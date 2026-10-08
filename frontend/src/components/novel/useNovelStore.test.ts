import { beforeEach, describe, expect, it, vi } from "vitest";
import { useNovelStore } from "./useNovelStore";

const { mockSetActiveNovel } = vi.hoisted(() => ({
  mockSetActiveNovel: vi.fn(),
}));

vi.mock("@/lib/wailsjs/go/app/App", () => ({
  SetActiveNovel: mockSetActiveNovel,
}));

function deferred() {
  let resolve!: () => void;
  let reject!: (reason: Error) => void;
  const promise = new Promise<void>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

describe("useNovelStore.switchNovel", () => {
  beforeEach(() => {
    mockSetActiveNovel.mockReset();
    useNovelStore.setState({ activeNovelId: 1 });
  });

  it("后端确认前保留旧小说，成功后才更新界面状态", async () => {
    const pending = deferred();
    mockSetActiveNovel.mockReturnValue(pending.promise);

    const switching = useNovelStore.getState().switchNovel(2);
    await vi.waitFor(() => {
      expect(mockSetActiveNovel).toHaveBeenCalledWith({ novel_id: 2 });
    });
    expect(useNovelStore.getState().activeNovelId).toBe(1);

    pending.resolve();
    await switching;
    expect(useNovelStore.getState().activeNovelId).toBe(2);
  });

  it("保存失败时保留旧小说", async () => {
    mockSetActiveNovel.mockRejectedValue(new Error("保存失败"));

    await expect(useNovelStore.getState().switchNovel(2)).rejects.toThrow(
      "保存失败",
    );
    expect(useNovelStore.getState().activeNovelId).toBe(1);
  });

  it("连续切书按请求顺序执行，最终使用最后一次选择", async () => {
    const first = deferred();
    const second = deferred();
    mockSetActiveNovel
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise);

    const switchFirst = useNovelStore.getState().switchNovel(2);
    const switchSecond = useNovelStore.getState().switchNovel(3);
    await vi.waitFor(() => expect(mockSetActiveNovel).toHaveBeenCalledTimes(1));
    expect(useNovelStore.getState().activeNovelId).toBe(1);

    first.resolve();
    await switchFirst;
    await vi.waitFor(() => expect(mockSetActiveNovel).toHaveBeenCalledTimes(2));
    expect(mockSetActiveNovel).toHaveBeenNthCalledWith(2, { novel_id: 3 });
    expect(useNovelStore.getState().activeNovelId).toBe(2);

    second.resolve();
    await switchSecond;
    expect(useNovelStore.getState().activeNovelId).toBe(3);
  });

  it("前一次失败不阻塞后续切书", async () => {
    mockSetActiveNovel
      .mockRejectedValueOnce(new Error("保存失败"))
      .mockResolvedValueOnce(undefined);

    const first = useNovelStore.getState().switchNovel(2);
    const second = useNovelStore.getState().switchNovel(3);
    await expect(first).rejects.toThrow("保存失败");
    await second;

    expect(useNovelStore.getState().activeNovelId).toBe(3);
    expect(mockSetActiveNovel).toHaveBeenNthCalledWith(2, { novel_id: 3 });
  });
});

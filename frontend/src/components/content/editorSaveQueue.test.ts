import { afterEach, describe, expect, it, vi } from "vitest";
import { QueryClient } from "@tanstack/react-query";
import {
  EditorSaveQueue,
  getEditorSaveQueue,
  type EditorSaveDraft,
} from "./editorSaveQueue";

function draft(path: string, content: string): EditorSaveDraft {
  return {
    novelId: 1,
    tabId: path,
    path,
    content,
    dirtyKey: "isDirty",
    expectedContent: "",
  };
}

afterEach(() => vi.useRealTimers());

describe("EditorSaveQueue", () => {
  it("切到另一文件后，两份待保存稿件都会写入", async () => {
    vi.useFakeTimers();
    const queue = new EditorSaveQueue();
    const saveFirst = vi.fn().mockResolvedValue(undefined);
    const saveSecond = vi.fn().mockResolvedValue(undefined);
    const markFirst = vi.fn();
    const markSecond = vi.fn();

    queue.schedule(
      draft("chapters/id_1.md", "第一章"),
      saveFirst,
      vi.fn(),
      markFirst,
      vi.fn(),
    );
    queue.schedule(
      draft("chapters/id_2.md", "第二章"),
      saveSecond,
      vi.fn(),
      markSecond,
      vi.fn(),
    );
    await vi.advanceTimersByTimeAsync(500);

    expect(saveFirst).toHaveBeenCalledOnce();
    expect(saveSecond).toHaveBeenCalledOnce();
    expect(markFirst).toHaveBeenCalledOnce();
    expect(markSecond).toHaveBeenCalledOnce();
  });

  it("同一文件只允许一次写入在途，新输入在前一版成功后再保存", async () => {
    vi.useFakeTimers();
    const queue = new EditorSaveQueue();
    let resolveFirst!: () => void;
    const firstWrite = new Promise<void>((resolve) => {
      resolveFirst = resolve;
    });
    const saveFirst = vi.fn(() => firstWrite);
    const saveSecond = vi.fn().mockResolvedValue(undefined);
    const markFirst = vi.fn();
    const markSecond = vi.fn();

    queue.schedule(
      draft("goink.md", "第一版"),
      saveFirst,
      vi.fn(),
      markFirst,
      vi.fn(),
    );
    const finished = queue.flush(1, "goink.md");
    expect(saveFirst).toHaveBeenCalledOnce();

    queue.schedule(
      draft("goink.md", "第二版"),
      saveSecond,
      vi.fn(),
      markSecond,
      vi.fn(),
    );
    await vi.advanceTimersByTimeAsync(500);
    expect(saveSecond).not.toHaveBeenCalled();

    resolveFirst();
    expect(await finished).toBe(true);
    expect(saveSecond).toHaveBeenCalledOnce();
    expect(saveFirst).toHaveBeenCalledWith("");
    expect(saveSecond).toHaveBeenCalledWith("第一版");
    expect(markFirst).not.toHaveBeenCalled();
    expect(markSecond).toHaveBeenCalledOnce();
  });

  it("保存失败时不确认稿件，也不允许关闭操作继续", async () => {
    const queue = new EditorSaveQueue();
    const save = vi
      .fn()
      .mockRejectedValueOnce(new Error("disk full"))
      .mockResolvedValueOnce(undefined);
    const markSaved = vi.fn();
    const onError = vi.fn();

    queue.schedule(
      draft("chapters/id_1.md", "稿件"),
      save,
      vi.fn(),
      markSaved,
      onError,
    );
    expect(await queue.flush(1, "chapters/id_1.md")).toBe(false);
    expect(markSaved).not.toHaveBeenCalled();
    expect(onError).toHaveBeenCalledOnce();

    expect(await queue.flush(1, "chapters/id_1.md")).toBe(true);
    expect(markSaved).toHaveBeenCalledOnce();
  });

  it("编辑器卸载再挂载时仍使用同一工作区的保存队列", async () => {
    vi.useFakeTimers();
    const qc = new QueryClient();
    const queue = getEditorSaveQueue(qc);
    const save = vi.fn().mockResolvedValue(undefined);
    queue.schedule(
      draft("volumes/id_1.md", "卷纲"),
      save,
      vi.fn(),
      vi.fn(),
      vi.fn(),
    );

    expect(getEditorSaveQueue(qc)).toBe(queue);
    await vi.advanceTimersByTimeAsync(500);
    expect(save).toHaveBeenCalledOnce();
  });

  it("条件写入冲突后暂停自动保存，确认新基线后才重试", async () => {
    vi.useFakeTimers();
    const queue = new EditorSaveQueue();
    const save = vi
      .fn()
      .mockRejectedValueOnce(new Error("CONTENT_CONFLICT: changed"))
      .mockResolvedValueOnce(undefined);
    const onSaved = vi.fn();
    const onError = vi.fn();
    queue.schedule(
      { ...draft("goink.md", "本地稿"), expectedContent: "旧版" },
      save,
      vi.fn(),
      onSaved,
      onError,
    );

    expect(await queue.flush(1, "goink.md")).toBe(false);
    expect(onSaved).not.toHaveBeenCalled();
    queue.schedule(
      { ...draft("goink.md", "本地新稿"), expectedContent: "旧版" },
      save,
      vi.fn(),
      onSaved,
      onError,
    );
    await vi.advanceTimersByTimeAsync(500);
    expect(save).toHaveBeenCalledOnce();
    expect(await queue.flush(1, "goink.md")).toBe(false);

    queue.discard(1, "goink.md");
    queue.schedule(
      { ...draft("goink.md", "本地新稿"), expectedContent: "AI 新版" },
      save,
      vi.fn(),
      onSaved,
      onError,
    );
    expect(await queue.flush(1, "goink.md")).toBe(true);
    expect(save).toHaveBeenLastCalledWith("AI 新版");
    expect(onSaved).toHaveBeenCalledOnce();
  });
});

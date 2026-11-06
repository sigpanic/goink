import { StrictMode, useEffect, useRef } from "react";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { DiffEditorProps, EditorProps } from "@monaco-editor/react";
import MonacoEditor, { DiffEditor } from "./MonacoEditor";

const mocks = vi.hoisted(() => ({
  ensureMonaco: vi.fn<() => Promise<void>>(),
  mount: true,
  crash: false,
}));

vi.mock("@/lib/monaco", () => ({ ensureMonaco: mocks.ensureMonaco }));
vi.mock("@monaco-editor/react", () => ({
  default: function MockEditor(props: EditorProps) {
    const onMount = useRef(props.onMount);
    useEffect(() => {
      if (mocks.mount) onMount.current?.({} as never, {} as never);
    }, []);
    if (mocks.crash) throw new Error("editor construction failed");
    return (
      <textarea
        aria-label="editable"
        value={props.value}
        onChange={(event) => props.onChange?.(event.target.value, {} as never)}
      />
    );
  },
  DiffEditor: function MockDiffEditor(props: DiffEditorProps) {
    const onMount = useRef(props.onMount);
    useEffect(() => {
      if (mocks.mount) onMount.current?.({} as never, {} as never);
    }, []);
    return (
      <div data-testid="diff">
        {props.original} / {props.modified}
      </div>
    );
  },
}));

function deferred() {
  let resolve!: () => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<void>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

describe("Monaco editor loading", () => {
  beforeEach(() => {
    mocks.mount = true;
    mocks.crash = false;
    mocks.ensureMonaco.mockReset().mockResolvedValue(undefined);
    vi.spyOn(console, "error").mockImplementation(() => undefined);
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("waits for the local runtime before mounting and preserves editor callbacks", async () => {
    const runtime = deferred();
    mocks.ensureMonaco.mockReturnValue(runtime.promise);
    const onMount = vi.fn();
    const onChange = vi.fn();
    render(<MonacoEditor value="正文" onMount={onMount} onChange={onChange} />);
    expect(screen.getByText("editor.loading")).toBeInTheDocument();
    expect(screen.queryByLabelText("editable")).not.toBeInTheDocument();
    await act(async () => runtime.resolve());
    expect(onMount).toHaveBeenCalledOnce();
    fireEvent.change(screen.getByLabelText("editable"), {
      target: { value: "新正文" },
    });
    expect(onChange).toHaveBeenCalledWith("新正文", {});
  });

  it("shows the current draft after failure, copies details and retries without changing the draft", async () => {
    mocks.ensureMonaco.mockRejectedValueOnce(
      new Error("local module unavailable"),
    );
    const onChange = vi.fn();
    const writeText = vi.fn().mockResolvedValue(undefined);
    vi.stubGlobal("navigator", { clipboard: { writeText } });
    const view = render(
      <MonacoEditor value="未保存的草稿" onChange={onChange} />,
    );
    await screen.findByText("local module unavailable");
    expect(screen.getByLabelText("editor.readOnlyContent")).toHaveValue(
      "未保存的草稿",
    );
    expect(screen.getByLabelText("editor.readOnlyContent")).toHaveAttribute(
      "readonly",
    );
    view.rerender(<MonacoEditor value="更新的草稿" onChange={onChange} />);
    expect(screen.getByLabelText("editor.readOnlyContent")).toHaveValue(
      "更新的草稿",
    );
    fireEvent.click(screen.getByText("editor.copyError"));
    await screen.findByText("editor.copied");
    expect(writeText).toHaveBeenCalledWith("local module unavailable");
    fireEvent.click(screen.getByText("common.retry"));
    expect(await screen.findByLabelText("editable")).toHaveValue("更新的草稿");
    expect(onChange).not.toHaveBeenCalled();
  });

  it("reports clipboard failure while keeping selectable error details", async () => {
    mocks.ensureMonaco.mockRejectedValue(new Error("runtime failure"));
    vi.stubGlobal("navigator", {
      clipboard: { writeText: vi.fn().mockRejectedValue(new Error("denied")) },
    });
    render(<MonacoEditor value="正文" />);
    await screen.findByText("runtime failure");
    fireEvent.click(screen.getByText("editor.copyError"));
    await screen.findByText("editor.copyFailed");
    expect(screen.getByText("runtime failure")).toBeInTheDocument();
  });

  it("times out a stalled runtime and ignores its late completion", async () => {
    vi.useFakeTimers();
    const runtime = deferred();
    mocks.ensureMonaco.mockReturnValue(runtime.promise);
    render(<MonacoEditor value="草稿" />);
    await act(async () => vi.advanceTimersByTime(15_000));
    expect(screen.getByText("editor.loadTimeout")).toBeInTheDocument();
    await act(async () => runtime.resolve());
    expect(screen.queryByLabelText("editable")).not.toBeInTheDocument();
    expect(screen.getByLabelText("editor.readOnlyContent")).toHaveValue("草稿");
    mocks.ensureMonaco.mockResolvedValue(undefined);
    fireEvent.click(screen.getByText("common.retry"));
    await act(async () => undefined);
    expect(screen.getByLabelText("editable")).toHaveValue("草稿");
  });

  it("times out editor mounting even when the runtime loaded successfully", async () => {
    vi.useFakeTimers();
    mocks.mount = false;
    render(<MonacoEditor value="正文" />);
    await act(async () => undefined);
    await act(async () => vi.advanceTimersByTime(15_000));
    expect(screen.getByText("editor.loadTimeout")).toBeInTheDocument();
    expect(screen.getByLabelText("editor.readOnlyContent")).toHaveValue("正文");
  });

  it("catches editor construction errors and allows a fresh mount", async () => {
    mocks.crash = true;
    render(<MonacoEditor value="草稿" />);
    await screen.findByText("editor construction failed");
    expect(screen.getByLabelText("editor.readOnlyContent")).toHaveValue("草稿");
    mocks.crash = false;
    fireEvent.click(screen.getByText("common.retry"));
    expect(await screen.findByLabelText("editable")).toHaveValue("草稿");
  });

  it("keeps both diff versions available when loading fails", async () => {
    mocks.ensureMonaco.mockRejectedValueOnce(new Error("diff runtime failure"));
    const onMount = vi.fn();
    render(
      <DiffEditor original="磁盘正文" modified="本地草稿" onMount={onMount} />,
    );
    await screen.findByText("diff runtime failure");
    expect(screen.getByLabelText("editor.originalContent")).toHaveValue(
      "磁盘正文",
    );
    expect(screen.getByLabelText("editor.modifiedContent")).toHaveValue(
      "本地草稿",
    );
    fireEvent.click(screen.getByText("common.retry"));
    expect(await screen.findByTestId("diff")).toHaveTextContent(
      "磁盘正文 / 本地草稿",
    );
    expect(onMount).toHaveBeenCalledOnce();
  });

  it("survives StrictMode lifecycle replay and clears the timeout after mounting", async () => {
    vi.useFakeTimers();
    render(
      <StrictMode>
        <MonacoEditor value="正文" />
      </StrictMode>,
    );
    await act(async () => undefined);
    await act(async () => vi.advanceTimersByTime(30_000));
    expect(screen.getByLabelText("editable")).toHaveValue("正文");
    expect(screen.queryByText("editor.loadTimeout")).not.toBeInTheDocument();
  });

  it("does not mount or report errors after unmounting", async () => {
    const runtime = deferred();
    mocks.ensureMonaco.mockReturnValue(runtime.promise);
    const onMount = vi.fn();
    const view = render(<MonacoEditor value="正文" onMount={onMount} />);
    view.unmount();
    await act(async () => runtime.reject(new Error("late failure")));
    await waitFor(() => expect(onMount).not.toHaveBeenCalled());
    expect(console.error).not.toHaveBeenCalled();
  });
});

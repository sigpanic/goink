import { describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  config: vi.fn(),
  monaco: { editor: {} },
  editor: vi.fn(function () {}),
}));

vi.mock("@monaco-editor/react", () => ({ loader: { config: mocks.config } }));
vi.mock("monaco-editor/esm/vs/editor/editor.api.js", () => mocks.monaco);
vi.mock("monaco-editor/esm/vs/editor/editor.all.js", () => ({}));
vi.mock(
  "monaco-editor/esm/vs/basic-languages/markdown/markdown.contribution.js",
  () => ({}),
);
vi.mock("monaco-editor/esm/vs/editor/editor.worker?worker&inline", () => ({
  default: mocks.editor,
}));

describe("local Monaco runtime", () => {
  it("shares initialization and injects the local instance before editors can call the loader", async () => {
    const { ensureMonaco } = await import("./monaco");
    const first = ensureMonaco();
    expect(ensureMonaco()).toBe(first);
    await first;
    expect(mocks.config).toHaveBeenCalledExactlyOnceWith({
      monaco: expect.objectContaining(mocks.monaco),
    });
  });

  it("creates the bundled editor Worker for diff computation", async () => {
    const { ensureMonaco } = await import("./monaco");
    await ensureMonaco();
    const instance = self.MonacoEnvironment!.getWorker!(
      "",
      "editorWorkerService",
    );
    expect(instance).toBeInstanceOf(mocks.editor);
  });
});

import { describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  config: vi.fn(),
  monaco: { editor: {} },
  editor: vi.fn(function () {}),
  json: vi.fn(function () {}),
  css: vi.fn(function () {}),
  html: vi.fn(function () {}),
  typescript: vi.fn(function () {}),
}));

vi.mock("@monaco-editor/react", () => ({ loader: { config: mocks.config } }));
vi.mock("monaco-editor", () => mocks.monaco);
vi.mock("monaco-editor/esm/vs/editor/editor.worker?worker&inline", () => ({
  default: mocks.editor,
}));
vi.mock("monaco-editor/esm/vs/language/json/json.worker?worker&inline", () => ({
  default: mocks.json,
}));
vi.mock("monaco-editor/esm/vs/language/css/css.worker?worker&inline", () => ({
  default: mocks.css,
}));
vi.mock("monaco-editor/esm/vs/language/html/html.worker?worker&inline", () => ({
  default: mocks.html,
}));
vi.mock(
  "monaco-editor/esm/vs/language/typescript/ts.worker?worker&inline",
  () => ({ default: mocks.typescript }),
);

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

  it.each([
    ["editorWorkerService", "editor"],
    ["json", "json"],
    ["css", "css"],
    ["scss", "css"],
    ["less", "css"],
    ["html", "html"],
    ["handlebars", "html"],
    ["razor", "html"],
    ["typescript", "typescript"],
    ["javascript", "typescript"],
  ] as const)("creates the bundled Worker for %s", async (label, worker) => {
    const { ensureMonaco } = await import("./monaco");
    await ensureMonaco();
    const instance = self.MonacoEnvironment!.getWorker!("", label);
    expect(instance).toBeInstanceOf(mocks[worker]);
  });
});

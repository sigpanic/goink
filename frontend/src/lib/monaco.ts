let runtimePromise: Promise<void> | undefined;

export function ensureMonaco(): Promise<void> {
  runtimePromise ??= import("./monacoRuntime")
    .then(() => undefined)
    .catch((error: unknown) => {
      runtimePromise = undefined;
      throw error;
    });
  return runtimePromise;
}

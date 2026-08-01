import { describe, it, expect } from "vitest";

import { splitModelKey } from "./modelKey";

describe("splitModelKey", () => {
  it("splits a plain provider/model key", () => {
    expect(splitModelKey("deepseek/deepseek-v4-pro")).toEqual([
      "deepseek",
      "deepseek-v4-pro",
    ]);
  });

  it("preserves slashes inside model ID (SiliconFlow / OpenRouter / Together)", () => {
    // 硅基流动模型 ID 形如 deepseek-ai/DeepSeek-V4-Flash，本身含 /
    // key = "siliconflow" + "/" + "deepseek-ai/DeepSeek-V4-Flash" → 三段
    expect(splitModelKey("siliconflow/deepseek-ai/DeepSeek-V4-Flash")).toEqual([
      "siliconflow",
      "deepseek-ai/DeepSeek-V4-Flash",
    ]);
  });

  it("handles model ID with multiple slashes", () => {
    expect(splitModelKey("openrouter/meta-llama/llama-3.1-70b-instruct")).toEqual([
      "openrouter",
      "meta-llama/llama-3.1-70b-instruct",
    ]);
  });

  it("returns empty modelID when key has no slash", () => {
    expect(splitModelKey("deepseek")).toEqual(["deepseek", ""]);
  });

  it("handles empty key", () => {
    expect(splitModelKey("")).toEqual(["", ""]);
  });
});

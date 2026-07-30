import { describe, it, expect } from "vitest";

import { toErrorMessage } from "./error";

describe("toErrorMessage", () => {
  it("returns string errors as-is", () => {
    // Wails 后端 reject 出来的是字符串（Go err.Error()）
    expect(toErrorMessage("网络错误")).toBe("网络错误");
  });

  it("extracts message from Error instances", () => {
    expect(toErrorMessage(new Error("boom"))).toBe("boom");
  });

  it("extracts message from objects with a string message field", () => {
    expect(toErrorMessage({ message: "oops" })).toBe("oops");
  });

  it("uses fallback for null when provided", () => {
    expect(toErrorMessage(null, "fallback")).toBe("fallback");
  });

  it("falls back to String(err) when no fallback is provided", () => {
    expect(toErrorMessage(null)).toBe("null");
  });

  it("uses fallback when message field is not a string", () => {
    // {message: 123} 不能提取出字符串，应走 fallback
    expect(toErrorMessage({ message: 123 }, "fallback")).toBe("fallback");
  });
});

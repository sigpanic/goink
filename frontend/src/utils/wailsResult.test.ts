import { describe, it, expect } from "vitest";

import { unwrapResult, AppErr } from "./wailsResult";

describe("unwrapResult", () => {
  it("returns data when err_code is empty", () => {
    // 后端 CodeOK = ""（空字符串），正常成功路径 err_code 为空
    const res = { err_code: "", data: { name: "Writer" } };
    expect(unwrapResult(res)).toEqual({ name: "Writer" });
  });

  it('returns data when err_code is "ok" (defensive)', () => {
    // 兜底兼容显式 "ok" 字符串（后端用空字符串，这里防御性处理）
    const res = { err_code: "ok", data: "hello" };
    expect(unwrapResult(res)).toBe("hello");
  });

  it("throws AppErr when err_code is non-empty", () => {
    const res = { err_code: "githubapi.network", err_msg: "timeout" };
    expect(() => unwrapResult(res)).toThrow(AppErr);
  });

  it("preserves errCode and msg on thrown AppErr", () => {
    const res = { err_code: "githubapi.rate_limited", err_msg: "429" };
    try {
      unwrapResult(res);
      throw new Error("should have thrown");
    } catch (e) {
      const appErr = e as AppErr;
      expect(appErr.errCode).toBe("githubapi.rate_limited");
      expect(appErr.message).toBe("429");
    }
  });

  it("falls back to err_code as message when err_msg missing", () => {
    const res = { err_code: "llm.not_found" };
    try {
      unwrapResult(res);
      throw new Error("should have thrown");
    } catch (e) {
      const appErr = e as AppErr;
      expect(appErr.errCode).toBe("llm.not_found");
      expect(appErr.message).toBe("llm.not_found");
    }
  });
});

describe("AppErr", () => {
  it("is an Error subclass", () => {
    const err = new AppErr("internal", "boom");
    expect(err).toBeInstanceOf(Error);
    expect(err.name).toBe("AppErr");
    expect(err.errCode).toBe("internal");
    expect(err.message).toBe("boom");
  });
});

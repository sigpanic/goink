import { beforeAll, describe, it, expect } from "vitest";
import { createInstance } from "i18next";
import zhCN from "@/i18n/locales/zh-CN.json";
import en from "@/i18n/locales/en.json";

import { errorCodeMessage, toErrorMessage } from "./error";

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

describe("errorCodeMessage", () => {
  const i18n = createInstance();
  beforeAll(async () => {
    await i18n.init({
      lng: "en",
      resources: { en: { translation: en }, "zh-CN": { translation: zhCN } },
    });
  });

  it.each([
    ["report.invalid_year", "annualReport.invalidYear"],
    ["report.invalid_image", "annualReport.invalidImage"],
    ["attachment.file_too_large", "chat.imageFileTooLarge"],
    ["attachment.too_many_pixels", "chat.imageTooManyPixels"],
    ["attachment.invalid_image", "chat.invalidImageData"],
    ["attachment.invalid_novel_id", "chat.imageNovelRequired"],
    ["skill.invalid_target", "skill.marketplace.invalidTarget"],
    ["skill.novel_required", "skill.marketplace.novelRequired"],
    ["skill.invalid_name", "skill.marketplace.invalidName"],
    ["skill.invalid_path", "skill.marketplace.invalidPath"],
    [
      "skill.expected_content_required",
      "skill.marketplace.confirmationRequired",
    ],
    ["skill.install_conflict", "skill.marketplace.targetChanged"],
  ])("translates %s in both languages", (code, key) => {
    const english = errorCodeMessage(code, i18n.getFixedT("en"));
    const chinese = errorCodeMessage(code, i18n.getFixedT("zh-CN"));
    expect(english).toBe(i18n.getFixedT("en")(key));
    expect(chinese).toBe(i18n.getFixedT("zh-CN")(key));
    expect(english).not.toBe(chinese);
    expect(english).not.toBe(key);
    expect(chinese).not.toBe(key);
  });

  it.each(["internal", "report.future_error"])(
    "leaves %s available for the caller to show the original error",
    (code) => {
      expect(errorCodeMessage(code, i18n.getFixedT("en"))).toBeUndefined();
    },
  );
});

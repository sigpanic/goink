import type { TFunction } from "i18next";

const ERROR_CODE_I18N: Record<string, string> = {
  "report.invalid_year": "annualReport.invalidYear",
  "report.invalid_image": "annualReport.invalidImage",
  "attachment.file_too_large": "chat.imageFileTooLarge",
  "attachment.too_many_pixels": "chat.imageTooManyPixels",
  "attachment.invalid_image": "chat.invalidImageData",
  "attachment.invalid_novel_id": "chat.imageNovelRequired",
  "skill.invalid_target": "skill.marketplace.invalidTarget",
  "skill.novel_required": "skill.marketplace.novelRequired",
  "skill.invalid_name": "skill.marketplace.invalidName",
  "skill.invalid_path": "skill.marketplace.invalidPath",
  "skill.expected_content_required": "skill.marketplace.confirmationRequired",
  "skill.install_conflict": "skill.marketplace.targetChanged",
};

export function errorCodeMessage(
  code: string,
  t: TFunction,
): string | undefined {
  const key = ERROR_CODE_I18N[code];
  return key ? t(key) : undefined;
}

/**
 * 把任意 unknown 错误转为可读字符串。
 *
 * Wails 后端 reject 出来的是字符串（Go err.Error()），不是 JS Error 对象，
 * 所以 instanceof Error 永远 false、e?.message 永远 undefined。本函数按
 * 优先级依次尝试：string → Error → {message} → fallback → String(err)。
 *
 * @param err catch 块的 unknown 错误
 * @param fallback 兜底文案（通常是 i18n 的 t("xxx.saveFailed")）
 */
export function toErrorMessage(err: unknown, fallback?: string): string {
  if (typeof err === "string") return err;
  if (err instanceof Error) return err.message;
  if (err && typeof err === "object" && "message" in err) {
    const m = (err as { message: unknown }).message;
    if (typeof m === "string") return m;
  }
  return fallback ?? String(err);
}

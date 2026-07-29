/**
 * 根据服务商错误消息返回对应的 i18n key。
 * 组件里用 t(explainErrorKey(msg)) 显示翻译后的解释。
 *
 * 匹配规则：按常见错误关键字（状态码、message 内容）映射到 i18n key。
 * 覆盖主流服务商（OpenAI/Anthropic/Gemini/DeepSeek/Kimi/GLM/Qwen/MiMo 等）及中转站。
 *
 * i18n key 命名空间：settings.errorHints.*
 */
export function explainErrorKey(msg: string): string {
  if (!msg) {
    return "settings.errorHints.generic";
  }

  const lower = msg.toLowerCase();

  if (
    /401|invalid.*key|invalid_key|authentication.*error|api[_-]?key.*invalid/.test(
      lower,
    )
  ) {
    return "settings.errorHints.invalidKey";
  }
  if (
    /402|余额不足|quota|insufficient.*balance|insufficient_quota|credit/.test(
      lower,
    )
  ) {
    return "settings.errorHints.insufficientBalance";
  }
  if (/403|forbidden|权限|access.*denied/.test(lower)) {
    return "settings.errorHints.forbidden";
  }
  if (/404|html 错误页|not\s*found/.test(lower)) {
    return "settings.errorHints.notFound";
  }
  if (/429|rate.*limit|too.*many|限流/.test(lower)) {
    return "settings.errorHints.rateLimit";
  }
  if (
    /50[023]|internal.*server|bad.*gateway|service.*unavailable/.test(lower)
  ) {
    return "settings.errorHints.serverError";
  }
  if (/sse|流.*未找到|chunk|非标准|choices/.test(lower)) {
    return "settings.errorHints.nonStandardSSE";
  }
  if (/timeout|超时|context.*deadline|deadline.*exceeded/.test(lower)) {
    return "settings.errorHints.timeout";
  }
  if (/url.*空|url 为空/.test(lower)) {
    return "settings.errorHints.emptyUrl";
  }

  return "settings.errorHints.generic";
}

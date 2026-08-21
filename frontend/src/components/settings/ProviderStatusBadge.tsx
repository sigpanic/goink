import { CheckCircle2, XCircle } from "lucide-react";
import { useTranslation } from "react-i18next";

interface Props {
  hasKey: boolean;
  testResult?: { ok: boolean; msg?: string; warning?: string } | undefined;
}

/**
 * 服务商状态五态标签：未配置 / 未测试 / 已通过 / 通过(限流) / 失败。
 * Builtin 和 Custom 共用，替代旧的"已配置/未配置"二态（只反映 hasKey 的弱语义）。
 *
 * 状态优先级：
 * 1. 未配置（hasKey=false）→ 灰色
 * 2. 已配置但未测试（testResult=undefined）→ 灰色
 * 3. 已测试通过（ok=true, warning 空）→ 绿色
 * 4. 已测试通过带 warning（ok=true, warning 非空，如 429）→ 黄色
 * 5. 已测试失败（ok=false）→ 红色
 */
export default function ProviderStatusBadge({ hasKey, testResult }: Props) {
  const { t } = useTranslation();

  if (!hasKey) {
    return (
      <span className="flex items-center gap-1 text-xs shrink-0 text-muted-foreground">
        {t("settings.notConfigured")}
      </span>
    );
  }
  if (!testResult) {
    return (
      <span className="flex items-center gap-1 text-xs shrink-0 text-muted-foreground">
        {t("settings.notTested")}
      </span>
    );
  }
  if (testResult.ok) {
    if (testResult.warning) {
      return (
        <span className="flex items-center gap-1 text-xs shrink-0 text-warning-foreground">
          <CheckCircle2 className="w-3.5 h-3.5" />
          {t("settings.passedWithWarning")}
        </span>
      );
    }
    return (
      <span className="flex items-center gap-1 text-xs shrink-0 text-success-foreground">
        <CheckCircle2 className="w-3.5 h-3.5" />
        {t("settings.passed")}
      </span>
    );
  }
  return (
    <span className="flex items-center gap-1 text-xs shrink-0 text-destructive">
      <XCircle className="w-3.5 h-3.5" />
      {t("settings.failed")}
    </span>
  );
}

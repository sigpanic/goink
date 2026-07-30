import { HelpCircle } from "lucide-react";
import { useTranslation } from "react-i18next";
import {
  Tooltip,
  TooltipTrigger,
  TooltipContent,
} from "@/components/ui/tooltip";
import { explainErrorKey } from "@/utils/errorExplain";
import { cn } from "@/utils/cn";

interface Props {
  testResult?: { ok: boolean; msg?: string } | undefined;
  className?: string;
}

/**
 * 测试结果显示组件：错误/成功消息 + 错误时的 ? 图标 Tooltip 提示。
 * BuiltinProviderPane、CustomProviderPane、ModelDiscoveryPanel 共用，避免重复代码。
 * className 可覆盖默认的 pl-[4rem] 左 padding（如 ModelDiscoveryPanel 不需要对齐 label）。
 */
export default function TestResultWithHint({ testResult, className }: Props) {
  const { t } = useTranslation();
  if (!testResult) return null;

  return (
    <div
      className={cn(
        "text-xs pl-[4rem] flex items-start gap-1.5",
        testResult.ok ? "text-success-foreground" : "text-red-500",
        className,
      )}
    >
      {!testResult.ok && testResult.msg && (
        <Tooltip>
          <TooltipTrigger asChild>
            <span className="inline-flex items-center justify-center w-5 h-5 mt-0.5 rounded-full bg-primary/10 text-primary cursor-help hover:bg-primary/20 transition-colors shrink-0">
              <HelpCircle className="w-3 h-3" />
            </span>
          </TooltipTrigger>
          <TooltipContent side="top" className="max-w-xs">
            <p>{t(explainErrorKey(testResult.msg))}</p>
          </TooltipContent>
        </Tooltip>
      )}
      <span className="whitespace-pre-line">
        {testResult.ok
          ? t("settings.connectionSuccess")
          : `✗ ${testResult.msg || t("settings.connectionFailed")}`}
      </span>
    </div>
  );
}

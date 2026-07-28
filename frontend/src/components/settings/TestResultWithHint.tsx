import { HelpCircle } from "lucide-react";
import { useTranslation } from "react-i18next";
import {
  Tooltip,
  TooltipTrigger,
  TooltipContent,
} from "@/components/ui/tooltip";
import { explainErrorKey } from "@/lib/errorExplain";

interface Props {
  testResult?: { ok: boolean; msg?: string } | undefined;
}

/**
 * 测试结果显示组件：错误/成功消息 + 错误时的 ? 图标 Tooltip 提示。
 * BuiltinProviderPane 和 CustomProviderPane 共用，避免重复代码。
 */
export default function TestResultWithHint({ testResult }: Props) {
  const { t } = useTranslation();
  if (!testResult) return null;

  return (
    <div
      className={`text-xs pl-[4rem] flex items-start gap-1.5 ${testResult.ok ? "text-success-foreground" : "text-red-500"}`}
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

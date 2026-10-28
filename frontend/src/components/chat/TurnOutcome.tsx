import { Loader2 } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { Turn } from "./types";

type Props = Pick<Turn, "status" | "errorMessage" | "retrying">;

export default function TurnOutcome({ status, errorMessage, retrying }: Props) {
  const { t } = useTranslation();

  return (
    <>
      {status === "error" && (
        <div className="flex justify-start">
          <div className="bg-danger-bg border border-danger-border rounded-lg px-3 py-2 text-xs text-red-600 max-w-[80%]">
            {errorMessage || t("chat.chatError")}
          </div>
        </div>
      )}
      {status === "system_interrupted" && (
        <div className="flex justify-center">
          <div className="bg-danger-bg border border-danger-border rounded-lg px-3 py-2 text-xs text-red-500 max-w-[80%]">
            {errorMessage || t("chat.chatInterrupted")}
          </div>
        </div>
      )}
      {status === "user_stopped" && (
        <div className="flex justify-center">
          <div className="bg-muted/50 border rounded-lg px-3 py-2 text-xs text-muted-foreground max-w-[80%]">
            {errorMessage || t("chat.chatStopped")}
          </div>
        </div>
      )}
      {retrying && (
        <div className="flex justify-center">
          <div className="bg-warning border border-warning-border rounded-lg px-3 py-2 text-xs text-warning-foreground max-w-[80%] flex items-center gap-2">
            <Loader2 className="w-3 h-3 animate-spin" />
            <span>
              {t("chat.retrying", {
                attempt: retrying.attempt,
                max: retrying.maxRetries,
              })}
            </span>
            {retrying.errorMessage && (
              <span className="opacity-70">· {retrying.errorMessage}</span>
            )}
          </div>
        </div>
      )}
    </>
  );
}

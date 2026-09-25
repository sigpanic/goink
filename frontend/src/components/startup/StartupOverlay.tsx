import { AlertCircle, Loader2, RefreshCw } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import Logo from "@/components/Logo";

interface Props {
  phase: "initializing" | "failed";
  error?: string;
  onRetry?: () => Promise<void>;
  retrying?: boolean;
}

export default function StartupOverlay({
  phase,
  error,
  onRetry,
  retrying = false,
}: Props) {
  const { t } = useTranslation();
  const failed = phase === "failed";

  return (
    <div className="flex min-h-screen items-center justify-center bg-background px-6">
      <div className="w-full max-w-md rounded-xl border bg-card p-8 text-center shadow-sm">
        <Logo className="mx-auto mb-6 h-14 w-14" />
        {failed ? (
          <AlertCircle className="mx-auto mb-4 h-8 w-8 text-destructive" />
        ) : (
          <Loader2 className="mx-auto mb-4 h-8 w-8 animate-spin text-primary" />
        )}
        <h1 className="mb-2 text-xl font-semibold">
          {failed ? t("startup.failedTitle") : t("startup.preparingTitle")}
        </h1>
        <p className="text-sm text-muted-foreground">
          {failed ? t("startup.failedMessage") : t("startup.preparingMessage")}
        </p>
        {failed && error && (
          <p className="mt-4 max-h-32 overflow-auto rounded-md bg-destructive/10 px-3 py-2 text-left text-sm text-destructive">
            {error}
          </p>
        )}
        {failed && onRetry && (
          <Button className="mt-6 w-full" onClick={onRetry} disabled={retrying}>
            {retrying ? (
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            ) : (
              <RefreshCw className="mr-2 h-4 w-4" />
            )}
            {retrying ? t("startup.retrying") : t("startup.retry")}
          </Button>
        )}
      </div>
    </div>
  );
}

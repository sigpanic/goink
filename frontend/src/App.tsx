import { useState, useEffect } from "react";
import { useTranslation } from "react-i18next";
import { IsInitialized, GetSettings } from "@/lib/wailsjs/go/app/App";
import { toastError } from "@/utils/toast";
import { toErrorMessage } from "@/utils/error";
import { Toaster } from "sonner";
import { TooltipProvider } from "@/components/ui/tooltip";
import { QueryClientProvider } from "@tanstack/react-query";
import { queryClient } from "@/lib/queryClient";
import InitView from "@/views/InitView";
import WorkspaceView from "@/views/WorkspaceView";

type View = "loading" | "init" | "workspace";

export default function App() {
  const { t } = useTranslation();
  const [view, setView] = useState<View>("loading");
  const [initialNovelId, setInitialNovelId] = useState(0);
  const [fromInit, setFromInit] = useState(false);
  useEffect(() => {
    IsInitialized()
      .then(async (ok) => {
        if (ok) {
          const settings = await GetSettings();
          setInitialNovelId(settings?.last_novel_id ?? 0);
          setView("workspace");
        } else {
          setView("init");
        }
      })
      .catch((err) => {
        console.error("App initialization failed", err);
        setView("init");
      });
  }, []);

  if (view === "loading") {
    return (
      <div className="flex items-center justify-center min-h-screen bg-background">
        <p className="text-muted-foreground">{t("app.loading")}</p>
      </div>
    );
  }

  return (
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>
        <div className="min-h-screen bg-background text-foreground">
          <Toaster
            position="top-center"
            richColors
            toastOptions={{
              actionButtonStyle: {
                backgroundColor: "var(--primary)",
                color: "var(--primary-foreground)",
                border: "none",
                padding: "2px 10px",
                borderRadius: "4px",
                fontSize: "12px",
              },
            }}
          />
          {view === "init" && (
            <InitView
              onInitialized={async () => {
                try {
                  const settings = await GetSettings();
                  setInitialNovelId(settings?.last_novel_id ?? 0);
                } catch (err) {
                  toastError(toErrorMessage(err, t("chat.settingsLoadFailed")));
                }
                setFromInit(true);
                setView("workspace");
              }}
            />
          )}
          {view === "workspace" && (
            <WorkspaceView
              initialNovelId={initialNovelId}
              initialShowHelp={fromInit}
            />
          )}
        </div>
      </TooltipProvider>
    </QueryClientProvider>
  );
}

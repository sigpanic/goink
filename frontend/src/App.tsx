import { Toaster } from "sonner";
import { TooltipProvider } from "@/components/ui/tooltip";
import { QueryClientProvider } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { queryClient } from "@/lib/queryClient";
import InitView from "@/views/InitView";
import WorkspaceView from "@/views/WorkspaceView";
import StartupOverlay from "@/components/startup/StartupOverlay";
import LoadingScreen from "@/components/startup/LoadingScreen";
import ConfirmDialog from "@/components/ui/ConfirmDialog";
import { useStartupState } from "@/components/startup/useStartupState";
import { useWorkspaceBoot } from "@/components/startup/useWorkspaceBoot";
import { useQuitConfirm } from "@/components/startup/useQuitConfirm";

export default function App() {
  const { t } = useTranslation();
  const startup = useStartupState();
  const boot = useWorkspaceBoot(startup.phase, startup.version);
  const quitConfirm = useQuitConfirm();

  function renderStartupContent() {
    switch (startup.phase) {
      case "unconfigured":
        return <InitView error={startup.error} />;
      case "failed":
        return (
          <StartupOverlay
            phase="failed"
            error={startup.error}
            onRetry={startup.retry}
            retrying={startup.retrying}
          />
        );
      case "ready":
        return boot.ready ? (
          <WorkspaceView
            initialNovelId={boot.novelId}
            initialShowHelp={startup.cameFromInit}
          />
        ) : (
          <LoadingScreen />
        );
      case "initializing":
        return startup.showPreparing ? (
          <StartupOverlay phase="initializing" />
        ) : (
          <LoadingScreen />
        );
      default:
        return <LoadingScreen />;
    }
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
          {renderStartupContent()}
          {/* 关窗确认由后端在迁移期间发起，弹窗挂在根部以免随启动界面切换被卸载；
              「继续等待」是安全选项，因此 X / 遮罩 / Esc 都映射到它，只有红色「退出」才真的退出。 */}
          <ConfirmDialog
            open={quitConfirm.open}
            title={t("startup.quitConfirmTitle")}
            message={t("startup.quitConfirmMessage")}
            confirmText={t("startup.quitConfirmLeave")}
            cancelText={t("startup.quitConfirmWait")}
            danger
            onConfirm={quitConfirm.confirm}
            onClose={quitConfirm.cancel}
          />
        </div>
      </TooltipProvider>
    </QueryClientProvider>
  );
}

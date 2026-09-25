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
  // 顺序不能反：useQuitConfirm 必须先注册好 app:quit-confirm 监听，useStartupState 才会在
  // effect 里调用 FrontendReady() 完成握手——后端一旦标记前端就绪，关窗请求就会被拦截并
  // 发出该事件。hook 的 effect 按声明顺序执行，所以这两行谁在前是有意义的。
  const quitConfirm = useQuitConfirm();
  const startup = useStartupState();
  const boot = useWorkspaceBoot(startup.phase, startup.version);

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

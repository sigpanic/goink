import { Toaster } from "sonner";
import { TooltipProvider } from "@/components/ui/tooltip";
import { QueryClientProvider } from "@tanstack/react-query";
import { queryClient } from "@/lib/queryClient";
import InitView from "@/views/InitView";
import WorkspaceView from "@/views/WorkspaceView";
import StartupOverlay from "@/components/startup/StartupOverlay";
import LoadingScreen from "@/components/startup/LoadingScreen";
import { useStartupState } from "@/components/startup/useStartupState";
import { useWorkspaceBoot } from "@/components/startup/useWorkspaceBoot";

export default function App() {
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
        </div>
      </TooltipProvider>
    </QueryClientProvider>
  );
}

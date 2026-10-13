import { useRef, useState } from "react";
import { AlertCircle, Check, Loader2, RefreshCw, Server } from "lucide-react";
import { useTranslation } from "react-i18next";
import { servercfg } from "@/lib/wailsjs/go/models";
import { toErrorMessage } from "@/utils/error";
import { Button } from "@/components/ui/button";
import ConfirmDialog from "@/components/ui/ConfirmDialog";
import MCPServerConnection from "./MCPServerConnection";
import MCPServerCredentials from "./MCPServerCredentials";
import MCPServerSetupGuide from "./MCPServerSetupGuide";
import MCPServerTools from "./MCPServerTools";
import MCPServerPageNavigation, {
  type MCPServerSection,
} from "./MCPServerPageNavigation";
import { useMCPServerCredentialActions } from "./useMCPServerCredentialActions";
import { useMCPServerSettings } from "./useMCPServerSettings";
import { useSaveMCPServerSettings } from "./useSaveMCPServerSettings";

export default function MCPServerConfigTab() {
  const { t } = useTranslation();
  const query = useMCPServerSettings();
  const save = useSaveMCPServerSettings();
  const credentials = useMCPServerCredentialActions();
  const [portDraft, setPortDraft] = useState<string | null>(null);
  const [toolsDraft, setToolsDraft] = useState<string[] | null>(null);
  const [actionError, setActionError] = useState("");
  const [applying, setApplying] = useState(false);
  const [saved, setSaved] = useState(false);
  const [credentialNotice, setCredentialNotice] = useState("");
  const [confirmAction, setConfirmAction] = useState<
    "rotate" | "revoke" | null
  >(null);
  const scrollRef = useRef<HTMLDivElement>(null);
  const [activeSection, setActiveSection] =
    useState<MCPServerSection>("connection");
  const view = query.data;

  if (!view) {
    return (
      <div className="flex flex-col items-center justify-center flex-1 gap-3">
        {query.isPending ? (
          <>
            <Loader2 className="w-5 h-5 animate-spin text-muted-foreground" />
            <p className="text-sm text-muted-foreground" role="status">
              {t("settings.mcpServer.loading")}
            </p>
          </>
        ) : (
          <>
            <p role="alert" className="text-sm text-destructive">
              {toErrorMessage(query.error, t("settings.mcpServer.loadFailed"))}
            </p>
            <Button
              variant="outline"
              size="sm"
              onClick={() => void query.refetch()}
              disabled={query.isFetching}
            >
              {t("settings.mcpServer.refresh")}
            </Button>
          </>
        )}
      </div>
    );
  }

  const config = view.config;
  const portValue = portDraft ?? String(config.port);
  const allowedTools = toolsDraft ?? view.config.allowed_tools ?? [];
  const busy =
    applying ||
    save.isPending ||
    credentials.rotate.isPending ||
    credentials.revoke.isPending;
  const disabled = busy || query.isError;
  const url = `http://127.0.0.1:${config.port}/mcp`;
  const statusKey = view.status.stopping
    ? "stopping"
    : view.status.running
      ? "running"
      : "stopped";
  const statusColor = view.status.stopping
    ? "bg-warning text-warning-foreground"
    : view.status.running
      ? "bg-primary/10 text-primary"
      : "bg-muted text-muted-foreground";
  const dirty =
    portValue !== String(view.config.port) ||
    allowedTools.length !== (view.config.allowed_tools ?? []).length ||
    allowedTools.some((name) => !view.config.allowed_tools?.includes(name));
  const error = query.isError
    ? toErrorMessage(query.error, t("settings.mcpServer.loadFailed"))
    : actionError || view.last_error;

  async function apply(input: servercfg.Local, clearDraft: boolean) {
    setApplying(true);
    setActionError("");
    setSaved(false);
    setCredentialNotice("");
    try {
      await save.mutateAsync(input);
      if (clearDraft) {
        setPortDraft(null);
        setToolsDraft(null);
        setSaved(true);
      }
    } catch (err) {
      const message = toErrorMessage(err, t("settings.mcpServer.saveFailed"));
      setActionError(message);
      if (!clearDraft && input.enabled) {
        try {
          await save.mutateAsync(
            new servercfg.Local({ ...input, enabled: false }),
          );
        } catch (restoreError) {
          setActionError(
            `${message}\n${toErrorMessage(restoreError, t("settings.mcpServer.saveFailed"))}`,
          );
        }
      }
    } finally {
      setApplying(false);
    }
  }

  async function changeCredential(action: "rotate" | "revoke") {
    setActionError("");
    setSaved(false);
    setCredentialNotice("");
    try {
      await credentials[action].mutateAsync();
      setCredentialNotice(
        action === "rotate"
          ? "settings.mcpServer.tokenRotated"
          : "settings.mcpServer.tokenRevoked",
      );
    } catch (err) {
      setActionError(
        toErrorMessage(
          err,
          t(
            action === "rotate"
              ? "settings.mcpServer.rotateFailed"
              : "settings.mcpServer.revokeFailed",
          ),
        ),
      );
    } finally {
      setConfirmAction(null);
    }
  }

  function saveConfig() {
    const port = Number(portValue);
    if (
      !/^\d+$/.test(portValue) ||
      !Number.isInteger(port) ||
      port < 1024 ||
      port > 32767
    ) {
      setSaved(false);
      setActionError(t("settings.mcpServer.invalidPort"));
      return;
    }
    void apply(
      new servercfg.Local({
        enabled: config.enabled,
        port,
        allowed_tools: allowedTools,
      }),
      true,
    );
  }

  function navigate(section: MCPServerSection) {
    const container = scrollRef.current;
    const target = container?.querySelector<HTMLElement>(
      `#mcp-server-section-${section}`,
    );
    if (!container || !target) return;
    const top =
      target.getBoundingClientRect().top -
      container.getBoundingClientRect().top +
      container.scrollTop;
    setActiveSection(section);
    container.scrollTo({
      top,
      behavior: window.matchMedia?.("(prefers-reduced-motion: reduce)").matches
        ? "auto"
        : "smooth",
    });
    target.focus({ preventScroll: true });
  }

  function updateActiveSection() {
    const container = scrollRef.current;
    if (!container) return;
    if (
      container.scrollHeight > container.clientHeight &&
      container.scrollTop + container.clientHeight >= container.scrollHeight - 1
    ) {
      setActiveSection("tools");
      return;
    }
    const top = container.getBoundingClientRect().top;
    let current: MCPServerSection = "connection";
    for (const section of container.querySelectorAll<HTMLElement>(
      "[data-mcp-section]",
    )) {
      if (section.getBoundingClientRect().top > top + 24) break;
      current = section.dataset.mcpSection as MCPServerSection;
    }
    setActiveSection(current);
  }

  return (
    <div className="@container flex flex-col h-full min-h-0">
      <div className="flex items-start justify-between gap-3 mb-5 pr-6">
        <div>
          <h3 className="flex items-center gap-2 text-base font-semibold">
            <span className="flex h-8 w-8 items-center justify-center rounded-lg bg-primary/10 text-primary">
              <Server className="w-4 h-4" />
            </span>
            {t("settings.mcpServer.title")}
          </h3>
          <p className="mt-2 text-xs text-muted-foreground leading-relaxed">
            {t("settings.mcpServer.description")}
          </p>
        </div>
        <span
          role="status"
          className={`flex shrink-0 items-center gap-1.5 px-2.5 py-1 rounded-full text-xs ${statusColor}`}
        >
          <span
            className={`h-1.5 w-1.5 rounded-full bg-current ${view.status.stopping ? "animate-pulse" : ""}`}
          />
          {t(`settings.mcpServer.${statusKey}`)}
        </span>
      </div>

      <MCPServerPageNavigation
        activeSection={activeSection}
        onNavigate={navigate}
      />

      <div
        ref={scrollRef}
        role="region"
        aria-label={t("settings.mcpServer.content")}
        onScroll={updateActiveSection}
        className="flex-1 min-h-0 overflow-y-auto space-y-5 pr-1"
      >
        {error && (
          <div
            role="alert"
            className="flex gap-2 p-3 rounded-md border border-destructive/30 bg-destructive/5 text-xs text-destructive break-words"
          >
            <AlertCircle className="w-4 h-4 shrink-0" />
            <span className="min-w-0 whitespace-pre-wrap">{error}</span>
          </div>
        )}

        <div
          id="mcp-server-section-connection"
          data-mcp-section="connection"
          role="group"
          tabIndex={-1}
          aria-label={t("settings.mcpServer.navConnection")}
          className="outline-none"
        >
          <MCPServerConnection
            enabled={config.enabled}
            running={view.status.running}
            stopping={view.status.stopping}
            port={portValue}
            disabled={disabled}
            onEnabledChange={(enabled) =>
              void apply(new servercfg.Local({ ...config, enabled }), false)
            }
            onPortChange={(port) => {
              setPortDraft(port);
              setSaved(false);
            }}
          />
        </div>

        <div
          id="mcp-server-section-credentials"
          data-mcp-section="credentials"
          role="group"
          tabIndex={-1}
          aria-label={t("settings.mcpServer.navCredentials")}
          className="outline-none"
        >
          <MCPServerCredentials
            url={url}
            token={view.token}
            disabled={disabled}
            onRotate={() => {
              if (view.token) setConfirmAction("rotate");
              else void changeCredential("rotate");
            }}
            onRevoke={() => setConfirmAction("revoke")}
          />
        </div>

        <div
          id="mcp-server-section-guide"
          data-mcp-section="guide"
          role="group"
          tabIndex={-1}
          aria-label={t("settings.mcpServer.navGuide")}
          className="outline-none"
        >
          <MCPServerSetupGuide
            url={url}
            token={view.token}
            disabled={disabled}
          />
        </div>

        <div
          id="mcp-server-section-tools"
          data-mcp-section="tools"
          role="group"
          tabIndex={-1}
          aria-label={t("settings.mcpServer.navTools")}
          className="outline-none"
        >
          <MCPServerTools
            available={view.available_tools}
            selected={allowedTools}
            disabled={disabled}
            onChange={(names) => {
              setToolsDraft(names);
              setSaved(false);
            }}
          />
        </div>
      </div>

      <div className="flex items-center justify-between gap-3 pt-4 border-t mt-4">
        <Button
          variant="outline"
          size="sm"
          onClick={() => {
            setActionError("");
            void query.refetch();
          }}
          disabled={busy || query.isFetching}
        >
          <RefreshCw
            className={`w-3.5 h-3.5 ${query.isFetching ? "animate-spin" : ""}`}
          />
          {t("settings.mcpServer.refresh")}
        </Button>
        <div className="flex items-center gap-3">
          {(saved || credentialNotice) && (
            <span
              role="status"
              className="flex items-center gap-1 text-xs text-primary"
            >
              <Check className="h-3.5 w-3.5" />
              {t(credentialNotice || "settings.mcpServer.saved")}
            </span>
          )}
          <Button
            size="sm"
            onClick={saveConfig}
            disabled={
              disabled || (!dirty && !view.last_error && !view.status.stopping)
            }
          >
            {busy ? t("common.saving") : t("settings.mcpServer.save")}
          </Button>
        </div>
      </div>
      <ConfirmDialog
        open={confirmAction !== null}
        title={t(
          confirmAction === "revoke"
            ? "settings.mcpServer.revokeTitle"
            : "settings.mcpServer.rotateTitle",
        )}
        message={t(
          confirmAction === "revoke"
            ? "settings.mcpServer.revokeConfirm"
            : "settings.mcpServer.rotateConfirm",
        )}
        danger={confirmAction === "revoke"}
        loading={busy}
        onConfirm={() => {
          if (confirmAction && !busy) return changeCredential(confirmAction);
        }}
        onClose={() => {
          if (!busy) setConfirmAction(null);
        }}
      />
    </div>
  );
}

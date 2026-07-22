import { Download, Sparkles, ExternalLink } from "lucide-react";
import { useTranslation } from "react-i18next";
import Markdown from "@/components/Markdown";
import type { update } from "@/lib/wailsjs/go/models";
import { DismissUpdate } from "@/lib/wailsjs/go/app/App";
import { BrowserOpenURL } from "@/lib/wailsjs/runtime/runtime";

interface Props {
  open: boolean;
  result: update.CheckResult | null;
  onClose: () => void;
}

export default function UpdateDialog({ open, result, onClose }: Props) {
  const { t } = useTranslation();

  if (!open || !result) return null;

  function handleDismiss() {
    if (result?.latest?.tag_name) {
      DismissUpdate(result.latest.tag_name).catch(() => {});
    }
    onClose();
  }

  function handleDownload() {
    if (result?.latest?.html_url) {
      BrowserOpenURL(result.latest.html_url);
    }
    if (result?.latest?.tag_name) {
      DismissUpdate(result.latest.tag_name).catch(() => {});
    }
    onClose();
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div
        className="absolute inset-0 bg-foreground/40"
        onClick={handleDismiss}
      />
      <div className="relative w-[640px] max-w-[90vw] max-h-[85vh] rounded-xl border border-border bg-background shadow-2xl flex flex-col">
        {/* 头部 */}
        <div className="flex items-start gap-3 p-6 pb-4 shrink-0">
          <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
            <Sparkles className="h-5 w-5" />
          </div>
          <div className="min-w-0 flex-1">
            <h2 className="text-base font-semibold text-foreground">
              {t("update.available")}
            </h2>
            <p className="mt-1 text-sm text-muted-foreground">
              {t("update.versionLabel", { version: result.latest.tag_name })}
            </p>
          </div>
          <span className="rounded-md bg-primary/10 px-2 py-1 text-xs font-medium text-primary">
            {result.latest.tag_name}
          </span>
        </div>

        {/* Release notes */}
        {result.latest.body && (
          <div className="flex-1 overflow-y-auto px-6 pb-4 min-h-0">
            <div className="rounded-md border border-border bg-muted/30 p-4">
              <Markdown content={result.latest.body} className="text-sm" />
            </div>
          </div>
        )}

        {/* 底部按钮 */}
        <div className="flex items-center justify-end gap-2 p-6 pt-4 shrink-0 border-t border-border">
          <button
            onClick={handleDismiss}
            className="h-9 rounded-md px-4 text-sm text-muted-foreground transition-colors hover:bg-muted"
          >
            {t("update.dismiss")}
          </button>
          <button
            onClick={handleDownload}
            className="flex h-9 items-center gap-2 rounded-md bg-primary px-4 text-sm text-primary-foreground transition-opacity hover:opacity-90"
          >
            <Download className="h-4 w-4" />
            {t("update.download")}
            <ExternalLink className="h-3 w-3 opacity-60" />
          </button>
        </div>
      </div>
    </div>
  );
}

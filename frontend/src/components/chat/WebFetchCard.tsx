import { memo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Globe, ExternalLink, ChevronDown, ChevronRight } from "lucide-react";
import Markdown from "@/components/Markdown";
import ConfirmDialog from "@/components/ui/ConfirmDialog";
import "./WebFetchCard.css";

interface Props {
  result: Record<string, unknown>;
  displayText: string;
}

export default memo(function WebFetchCard({ result, displayText }: Props) {
  const { t } = useTranslation();
  const [contentOpen, setContentOpen] = useState(false);
  // 打开外部浏览器前弹确认框，避免误点外链
  const [pendingUrl, setPendingUrl] = useState<string | null>(null);
  const confirmOpenExternal = () => {
    if (!pendingUrl) return;
    window.open(pendingUrl, "_blank", "noopener,noreferrer");
    setPendingUrl(null);
  };

  const url = (result.url as string) || "";
  const title = (result.title as string) || "";
  const text = (result.text as string) || "";
  const wordCount = text.replace(/\s/g, "").length;

  return (
    <div className="fetch-card completed">
      <div className="fetch-card-row">
        <span className="fetch-card-icon">
          <Globe size={14} />
        </span>
        <span className="fetch-card-label">{displayText}</span>
        <span className="fetch-card-badge fetch-card-badge-done">
          {t("chat.done")}
        </span>
      </div>

      <div className="fetch-card-meta">
        <div className="fetch-card-title-line">
          <span className="fetch-card-title">{title || url}</span>
          <button
            className="fetch-card-ext-btn"
            onClick={() => setPendingUrl(url)}
            title={url}
          >
            <ExternalLink size={12} />
          </button>
        </div>
        {url && <span className="fetch-card-url">{url}</span>}
      </div>

      {text && (
        <div className="fetch-card-content">
          <button
            className="fetch-card-content-toggle"
            onClick={() => setContentOpen(!contentOpen)}
          >
            {contentOpen ? (
              <ChevronDown size={14} />
            ) : (
              <ChevronRight size={14} />
            )}
            {t("chat.pageContent", { count: wordCount })}
          </button>
          {contentOpen && (
            <div className="fetch-card-content-body">
              <Markdown content={text} />
            </div>
          )}
        </div>
      )}
      <ConfirmDialog
        open={pendingUrl !== null}
        title={t("chat.openInBrowser")}
        message={pendingUrl ?? ""}
        onClose={() => setPendingUrl(null)}
        onConfirm={confirmOpenExternal}
      />
    </div>
  );
});

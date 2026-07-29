import { memo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Search, ExternalLink, ChevronDown, ChevronRight } from "lucide-react";
import { BrowserOpenURL } from "@/lib/wailsjs/runtime";
import Markdown from "@/components/Markdown";
import ConfirmDialog from "@/components/ui/ConfirmDialog";
import "./WebSearchCard.css";

interface SourceItem {
  title: string;
  url: string;
}

interface Props {
  result: Record<string, unknown>;
}

export default memo(function WebSearchCard({ result }: Props) {
  const { t } = useTranslation();
  const [summaryOpen, setSummaryOpen] = useState(false);
  // 打开外部浏览器前弹确认框，避免误点外链
  const [pendingUrl, setPendingUrl] = useState<string | null>(null);
  const confirmOpenExternal = () => {
    if (!pendingUrl) return;
    BrowserOpenURL(pendingUrl);
    setPendingUrl(null);
  };

  const queries = (result.queries as string[]) || [];
  const summary = (result.summary as string) || "";
  const sources = (result.sources as SourceItem[]) || [];

  return (
    <div className="web-card completed">
      <div className="web-card-row">
        <span className="web-card-icon">
          <Search size={14} />
        </span>
        <span className="web-card-label">{t("chat.searchComplete")}</span>
        <span className="web-card-badge web-card-badge-done">
          {t("chat.done")}
        </span>
      </div>

      {queries.length > 0 && (
        <div className="web-card-queries">
          <span className="web-card-queries-label">
            {t("chat.searchQuery")}
          </span>
          {queries.map((q, i) => (
            <span key={i} className="web-card-query-tag">
              {q}
            </span>
          ))}
        </div>
      )}

      {sources.length > 0 && (
        <div className="web-card-sources">
          {sources.map((s, i) => (
            <div
              key={i}
              className="web-card-source"
              onClick={() => setPendingUrl(s.url)}
              title={s.url}
            >
              <span className="web-card-source-index">{i + 1}</span>
              <div className="web-card-source-body">
                <span className="web-card-source-title">
                  {s.title || s.url}
                </span>
                <span className="web-card-source-url">{s.url}</span>
              </div>
              <ExternalLink size={12} className="web-card-source-ext" />
            </div>
          ))}
        </div>
      )}

      {summary && (
        <div className="web-card-summary">
          <button
            className="web-card-summary-toggle"
            onClick={() => setSummaryOpen(!summaryOpen)}
          >
            {summaryOpen ? (
              <ChevronDown size={14} />
            ) : (
              <ChevronRight size={14} />
            )}
            {t("chat.searchResultSummary")}
          </button>
          {summaryOpen && (
            <div className="web-card-summary-body">
              <Markdown content={summary} />
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

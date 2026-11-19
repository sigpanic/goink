import { memo, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  Search,
  ExternalLink,
  ChevronDown,
  ChevronRight,
  CheckCircle2,
  Globe,
  CalendarDays,
} from "lucide-react";
import { BrowserOpenURL } from "@/lib/wailsjs/runtime";
import Markdown from "@/components/Markdown";
import ConfirmDialog from "@/components/ui/ConfirmDialog";
import "./WebSearchCard.css";

interface SourceItem {
  title: string;
  url: string;
  summary?: string;
  date?: string;
  truncated?: boolean;
  authority?: number;
  rerank_score?: number;
}

interface Props {
  result: Record<string, unknown>;
}

export default memo(function WebSearchCard({ result }: Props) {
  const { t } = useTranslation();
  const [summaryOpen, setSummaryOpen] = useState(false);
  const [expandedSources, setExpandedSources] = useState<
    Record<number, boolean>
  >({});
  // 打开外部浏览器前弹确认框，避免误点外链
  const [pendingUrl, setPendingUrl] = useState<string | null>(null);
  const confirmOpenExternal = () => {
    if (!pendingUrl) return;
    BrowserOpenURL(pendingUrl);
    setPendingUrl(null);
  };

  const queries =
    typeof result.query === "string"
      ? [result.query]
      : (result.queries as string[]) || [];
  const summary = (result.summary as string) || "";
  const sources = (
    Array.isArray(result.results) ? result.results : result.sources || []
  ) as SourceItem[];

  return (
    <div className="web-card completed">
      <div className="web-card-row">
        <span className="web-card-icon">
          <Search size={14} />
        </span>
        <span className="web-card-label">{t("chat.searchComplete")}</span>
        {sources.length > 0 && (
          <span className="web-card-count">
            {t("chat.searchSourceCount", { count: sources.length })}
          </span>
        )}
        <span className="web-card-badge web-card-badge-done">
          <CheckCircle2 size={11} />
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
              data-expanded={!!expandedSources[i]}
            >
              <span className="web-card-source-index">{i + 1}</span>
              <div className="web-card-source-body">
                <button
                  type="button"
                  className="web-card-source-link"
                  onClick={() => setPendingUrl(s.url)}
                  title={s.url}
                  aria-label={s.title || s.url}
                >
                  <span className="web-card-source-heading">
                    <span className="web-card-source-title">
                      {s.title || s.url}
                    </span>
                    <ExternalLink size={12} className="web-card-source-ext" />
                  </span>
                  <span className="web-card-source-url">
                    <Globe size={11} />
                    <span className="web-card-source-address">{s.url}</span>
                  </span>
                </button>
                {(s.date ||
                  typeof s.authority === "number" ||
                  typeof s.rerank_score === "number" ||
                  s.truncated) && (
                  <div className="web-card-source-meta">
                    {s.date && (
                      <time dateTime={s.date}>
                        <CalendarDays size={11} />
                        {s.date.slice(0, 10)}
                      </time>
                    )}
                    {typeof s.authority === "number" && (
                      <span
                        className="web-card-source-score"
                        title={t("chat.sourceScoreHint")}
                      >
                        {t("chat.sourceAuthority")}: {s.authority}/4
                      </span>
                    )}
                    {typeof s.rerank_score === "number" && (
                      <span
                        className="web-card-source-score"
                        title={t("chat.sourceScoreHint")}
                      >
                        {t("chat.sourceRankScore")}: {s.rerank_score.toFixed(2)}
                        /3
                      </span>
                    )}
                    {s.truncated && (
                      <span className="web-card-truncated">
                        {t("chat.summaryTruncated")}
                      </span>
                    )}
                  </div>
                )}
                {s.summary && (
                  <>
                    <button
                      type="button"
                      className="web-card-source-summary-toggle"
                      aria-expanded={!!expandedSources[i]}
                      onClick={() =>
                        setExpandedSources((current) => ({
                          ...current,
                          [i]: !current[i],
                        }))
                      }
                    >
                      {expandedSources[i] ? (
                        <ChevronDown size={14} />
                      ) : (
                        <ChevronRight size={14} />
                      )}
                      {t("chat.sourceSummary")}
                    </button>
                    {expandedSources[i] && (
                      <div className="web-card-source-summary">
                        <Markdown content={s.summary} />
                      </div>
                    )}
                  </>
                )}
              </div>
            </div>
          ))}
        </div>
      )}

      {sources.length === 0 && !summary && (
        <div className="web-card-empty">{t("chat.noSearchResults")}</div>
      )}

      {summary && (
        <div className="web-card-summary">
          <button
            type="button"
            className="web-card-summary-toggle"
            aria-expanded={summaryOpen}
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

import { memo, useState } from "react";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { Globe, ExternalLink, ChevronDown, ChevronRight } from "lucide-react";
import Markdown from "@/components/Markdown";
import "./WebFetchCard.css";

interface Props {
  result: Record<string, unknown>;
  displayText: string;
}

function openExternal(url: string, t: TFunction) {
  if (window.confirm(`${t("chat.openInBrowser")}\n${url}`)) {
    window.open(url, "_blank", "noopener,noreferrer");
  }
}

export default memo(function WebFetchCard({ result, displayText }: Props) {
  const { t } = useTranslation();
  const [contentOpen, setContentOpen] = useState(false);

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
            onClick={() => openExternal(url, t)}
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
    </div>
  );
});

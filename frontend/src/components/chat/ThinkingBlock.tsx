import { useTranslation } from "react-i18next";
import "./ThinkingBlock.css";

interface Props {
  content: string;
  isStreaming: boolean;
}

export default function ThinkingBlock({ content, isStreaming }: Props) {
  const { t } = useTranslation();
  if (!content) return null;

  return (
    <details className="thinking-block">
      <summary className="thinking-summary">
        <span className="thinking-chevron">▶</span>
        {isStreaming ? (
          <span className="thinking-shimmer">{t("chat.thinking")}</span>
        ) : (
          <span>{t("chat.thinkingProcess")}</span>
        )}
      </summary>
      <div className="thinking-content">{content}</div>
    </details>
  );
}

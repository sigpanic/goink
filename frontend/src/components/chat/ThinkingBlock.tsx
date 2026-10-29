import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { Check, ChevronDown, Loader2 } from "lucide-react";
import { useTranslation } from "react-i18next";
import "./ThinkingBlock.css";

interface Props {
  content: string;
  isStreaming: boolean;
  hasThinking?: boolean;
}

export default function ThinkingBlock({
  content,
  isStreaming,
  hasThinking,
}: Props) {
  const { t } = useTranslation();
  const previewRef = useRef<HTMLSpanElement>(null);
  const [elapsed, setElapsed] = useState<number | null>(isStreaming ? 0 : null);
  const hasText = content.trim() !== "";
  const preview = isStreaming
    ? content.replace(/\s+/g, " ").trim()
    : content
        .split(/\r?\n/)
        .filter((line) => line.trim())
        .at(-1)
        ?.trim();

  useLayoutEffect(() => {
    if (previewRef.current) {
      previewRef.current.scrollLeft = isStreaming
        ? previewRef.current.scrollWidth
        : 0;
    }
  }, [preview, isStreaming]);

  useEffect(() => {
    if (!isStreaming) return;
    const started = Date.now();
    const timer = window.setInterval(
      () => setElapsed(Math.floor((Date.now() - started) / 1000)),
      1000,
    );
    return () => window.clearInterval(timer);
  }, [isStreaming]);

  if (!hasText && !hasThinking) return null;

  const heading = (
    <>
      {isStreaming ? (
        <Loader2
          className="thinking-indicator animate-spin"
          aria-hidden="true"
        />
      ) : (
        <Check className="thinking-indicator" aria-hidden="true" />
      )}
      <span className="thinking-preview-window" ref={previewRef}>
        <span
          className={`thinking-preview${isStreaming ? " thinking-shimmer" : ""}`}
        >
          {preview || t(isStreaming ? "chat.thinking" : "chat.thinkingProcess")}
        </span>
      </span>
      {elapsed !== null && (
        <span className="thinking-elapsed" aria-hidden="true">
          {elapsed}s
        </span>
      )}
      {hasText && (
        <ChevronDown className="thinking-chevron" aria-hidden="true" />
      )}
    </>
  );

  return (
    <div className="thinking-block" data-thinking={isStreaming}>
      {hasText ? (
        <details>
          <summary className="thinking-summary" title={preview}>
            {heading}
          </summary>
          <div className="thinking-content">{content}</div>
        </details>
      ) : (
        <div className="thinking-summary" data-expandable="false" role="status">
          {heading}
        </div>
      )}
    </div>
  );
}

import { useState, useEffect } from "react";
import { useTranslation } from "react-i18next";
import "./CompressionBlock.css";

interface Props {
  phase: "compressing" | "done";
}

export default function CompressionBlock({ phase }: Props) {
  const { t } = useTranslation();
  const [seconds, setSeconds] = useState(0);

  useEffect(() => {
    if (phase !== "compressing") return;
    setSeconds(0);
    const timer = setInterval(() => setSeconds((s) => s + 1), 1000);
    return () => clearInterval(timer);
  }, [phase]);

  const elapsed =
    seconds < 60
      ? `${seconds}s`
      : `${Math.floor(seconds / 60)}m${seconds % 60}s`;

  if (phase === "compressing") {
    return (
      <div className="compression-block">
        <div className="compression-compressing">
          <span className="compression-shimmer">
            {t("chat.compressingContext", { seconds: elapsed })}
          </span>
        </div>
      </div>
    );
  }

  return (
    <div className="compression-block">
      <div className="compression-done">
        <span className="compression-line" />
        <span className="compression-label">{t("chat.compressedContext")}</span>
        <span className="compression-line" />
      </div>
    </div>
  );
}

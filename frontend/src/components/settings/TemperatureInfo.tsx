import { useState, useRef, useEffect } from "react";
import { Info } from "lucide-react";
import { createPortal } from "react-dom";
import { useTranslation } from "react-i18next";

export default function TemperatureInfo() {
  const { t } = useTranslation();
  const [show, setShow] = useState(false);
  const iconRef = useRef<HTMLSpanElement>(null);
  const [pos, setPos] = useState({ top: 0, left: 0 });

  useEffect(() => {
    if (show && iconRef.current) {
      const rect = iconRef.current.getBoundingClientRect();
      setPos({ top: rect.top - 8, left: rect.left + rect.width / 2 });
    }
  }, [show]);

  return (
    <span ref={iconRef} className="inline-flex items-center">
      <Info
        className="w-3.5 h-3.5 text-muted-foreground hover:text-foreground cursor-help transition-colors"
        onMouseEnter={() => setShow(true)}
        onMouseLeave={() => setShow(false)}
      />
      {show &&
        createPortal(
          <span
            className="fixed z-[100] w-48 text-xs leading-relaxed bg-popover text-popover-foreground border rounded-md p-2 shadow-md -translate-x-1/2 -translate-y-full"
            style={{ top: pos.top, left: pos.left }}
          >
            {t("settings.temperatureDesc")}
          </span>,
          document.body,
        )}
    </span>
  );
}

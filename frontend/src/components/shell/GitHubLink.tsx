import { useState, useRef, useCallback } from "react";
import { BrowserOpenURL } from "@/lib/wailsjs/runtime/runtime";
import { useTranslation } from "react-i18next";
import GitHubIcon from "@/components/ui/GitHubIcon";

export default function GitHubLink() {
  const { t } = useTranslation();
  const url = "https://github.com/sigpanic/goink";

  const [showPopover, setShowPopover] = useState(false);
  const hideTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Mirror ContextRing's hover pattern: delay hide by 150ms so the cursor can
  // cross the gap between the button and the popover, and cancel the timer
  // when entering either element to keep the popover visible.
  const handleEnter = useCallback(() => {
    if (hideTimerRef.current) {
      clearTimeout(hideTimerRef.current);
      hideTimerRef.current = null;
    }
    setShowPopover(true);
  }, []);

  const handleLeave = useCallback(() => {
    hideTimerRef.current = setTimeout(() => setShowPopover(false), 150);
  }, []);

  return (
    <button
      onClick={() => BrowserOpenURL(url)}
      onMouseEnter={handleEnter}
      onMouseLeave={handleLeave}
      className="relative flex items-center gap-1.5 text-muted-foreground hover:text-foreground transition-colors cursor-pointer select-none"
    >
      <GitHubIcon className="w-4 h-4" />
      <span className="text-[11px]">GitHub</span>

      {showPopover && (
        <div
          onMouseEnter={handleEnter}
          onMouseLeave={handleLeave}
          className="absolute top-full right-0 mt-1.5 w-56 bg-popover border rounded-md shadow-md p-3 z-50"
        >
          <p className="text-xs text-foreground mb-1">
            {t("shell.githubIssue")}
          </p>
          <p className="text-xs text-foreground mb-1.5">
            {t("shell.githubStar")}
          </p>
          <p className="text-[10px] text-muted-foreground font-mono">
            github.com/sigpanic/goink
          </p>
        </div>
      )}
    </button>
  );
}

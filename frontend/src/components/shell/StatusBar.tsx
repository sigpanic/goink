import { useMemo, useState, useRef } from "react";
import { useTranslation } from "react-i18next";

interface Props {
  content: string;
  isDirty?: boolean;
}

interface DetailedStats {
  wordCount: number;
  lineCount: number;
  chineseChars: number;
  englishWords: number;
  charCountSpace: number;
  charCountNoSpace: number;
  paragraphCount: number;
}

function computeStats(text: string): DetailedStats {
  if (!text) {
    return {
      wordCount: 0,
      lineCount: 0,
      chineseChars: 0,
      englishWords: 0,
      charCountSpace: 0,
      charCountNoSpace: 0,
      paragraphCount: 0,
    };
  }

  let chineseChars = 0;
  let spaces = 0;
  let paragraphCount = 0;
  let inPara = false;

  for (const ch of text) {
    const cp = ch.codePointAt(0)!;
    if (
      (cp >= 0x4e00 && cp <= 0x9fff) ||
      (cp >= 0x3400 && cp <= 0x4dbf) ||
      (cp >= 0x20000 && cp <= 0x2a6df) ||
      (cp >= 0xf900 && cp <= 0xfaff)
    ) {
      chineseChars++;
    } else if (ch === " " || ch === "\t" || ch === "\n" || ch === "\r") {
      spaces++;
    }

    if (ch === "\n") {
      if (inPara) {
        paragraphCount++;
        inPara = false;
      }
    } else if (ch !== " " && ch !== "\t" && ch !== "\r") {
      inPara = true;
    }
  }
  if (inPara) paragraphCount++;

  const englishWords = (text.match(/[a-zA-Z]+(?:'[a-zA-Z]+)?/g) || []).length;
  const lineCount = text ? text.split("\n").length : 0;

  return {
    wordCount: chineseChars + englishWords,
    lineCount,
    chineseChars,
    englishWords,
    charCountSpace: [...text].length,
    charCountNoSpace: [...text].length - spaces,
    paragraphCount,
  };
}

export default function StatusBar({ content, isDirty }: Props) {
  const { t } = useTranslation();
  const stats = useMemo(() => computeStats(content), [content]);
  const [showDetail, setShowDetail] = useState(false);
  const hoverTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  function handleMouseEnter() {
    hoverTimer.current = setTimeout(() => setShowDetail(true), 150);
  }

  function handleMouseLeave() {
    if (hoverTimer.current) clearTimeout(hoverTimer.current);
    setShowDetail(false);
  }

  return (
    <div className="relative h-7 flex items-center justify-between px-4 border-t bg-sidebar text-xs text-muted-foreground select-none">
      <div className="flex items-center gap-4">
        <span
          className="cursor-default"
          onMouseEnter={handleMouseEnter}
          onMouseLeave={handleMouseLeave}
        >
          {t("shell.wordCount")} {stats.wordCount}
        </span>
        <span>
          {t("shell.lineCount")} {stats.lineCount}
        </span>
      </div>

      {showDetail && (
        <div
          className="absolute bottom-0 left-0 mb-7 ml-4 bg-popover border rounded-lg shadow-lg p-4 text-sm space-y-1.5 z-50 min-w-[220px]"
          onMouseEnter={() => {
            if (hoverTimer.current) clearTimeout(hoverTimer.current);
            setShowDetail(true);
          }}
          onMouseLeave={handleMouseLeave}
        >
          <div className="font-medium text-foreground mb-2">
            {t("shell.wordStats")}
          </div>
          <div className="flex justify-between gap-8">
            <span>{t("shell.wordCount")}</span>
            <span className="tabular-nums">{stats.wordCount}</span>
          </div>
          <div className="flex justify-between gap-8">
            <span className="pl-3">{t("shell.chineseChars")}</span>
            <span className="tabular-nums">{stats.chineseChars}</span>
          </div>
          <div className="flex justify-between gap-8">
            <span className="pl-3">{t("shell.englishWords")}</span>
            <span className="tabular-nums">{stats.englishWords}</span>
          </div>
          <div className="border-t my-1.5" />
          <div className="flex justify-between gap-8">
            <span>{t("shell.charsNoSpace")}</span>
            <span className="tabular-nums">{stats.charCountNoSpace}</span>
          </div>
          <div className="flex justify-between gap-8">
            <span>{t("shell.charsWithSpace")}</span>
            <span className="tabular-nums">{stats.charCountSpace}</span>
          </div>
          <div className="border-t my-1.5" />
          <div className="flex justify-between gap-8">
            <span>{t("shell.lineCount")}</span>
            <span className="tabular-nums">{stats.lineCount}</span>
          </div>
          <div className="flex justify-between gap-8">
            <span>{t("shell.paragraphCount")}</span>
            <span className="tabular-nums">{stats.paragraphCount}</span>
          </div>
        </div>
      )}

      <span className="flex items-center gap-1">
        <span
          className={`w-1.5 h-1.5 rounded-full ${isDirty ? "bg-status-warning" : "bg-status-ok"}`}
        />
        {isDirty ? t("shell.unsaved") : t("shell.saved")}
      </span>
    </div>
  );
}

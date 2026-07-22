import { GitCommitHorizontal, Copy, Check } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import type { git } from "@/lib/wailsjs/go/models";

interface Props {
  commit: git.CommitInfo;
}

function formatFullDate(iso: string, locale: string): string {
  const d = new Date(iso);
  const dateStr = new Intl.DateTimeFormat(locale, {
    year: "numeric",
    month: "long",
    day: "numeric",
  }).format(d);
  const h = d.getHours().toString().padStart(2, "0");
  const min = d.getMinutes().toString().padStart(2, "0");
  const s = d.getSeconds().toString().padStart(2, "0");
  const timeStr = `${h}:${min}:${s}`;
  return `${dateStr} ${timeStr}`;
}

export default function GitCommitTooltip({ commit }: Props) {
  const { t, i18n } = useTranslation();
  const [copied, setCopied] = useState(false);

  async function handleCopy() {
    try {
      await navigator.clipboard.writeText(commit.hash);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      /* ignore */
    }
  }

  return (
    <div className="w-80 p-4 rounded-lg border bg-card text-card-foreground shadow-xl">
      {/* 完整 commit message */}
      <p className="text-sm font-medium whitespace-pre-wrap break-words mb-3">
        {commit.message || t("git.noCommitMessage")}
      </p>

      {/* 作者 + 日期 */}
      <div className="flex items-center gap-2 text-xs text-muted-foreground mb-2">
        <GitCommitHorizontal className="w-3.5 h-3.5 shrink-0" />
        <span className="font-medium text-foreground">{commit.authorName}</span>
        <span className="text-muted-foreground/60">
          &lt;{commit.authorEmail}&gt;
        </span>
      </div>
      <p className="text-xs text-muted-foreground mb-3">
        {formatFullDate(commit.time, i18n.language)}
      </p>

      {/* 变更统计 */}
      <div className="flex items-center gap-1.5 text-xs mb-3">
        <span>{t("git.filesChanged", { count: commit.filesChanged })}</span>
        <span className="text-tag-green-foreground">+{commit.insertions}</span>
        <span className="text-tag-rose-foreground">-{commit.deletions}</span>
      </div>

      {/* short hash + copy（复制完整 hash） */}
      <div className="flex items-center gap-2 p-2 rounded-md bg-muted">
        <code className="text-xs font-mono text-muted-foreground">
          {commit.shortHash}
        </code>
        <span className="text-[10px] text-muted-foreground/40">·</span>
        <button
          onClick={handleCopy}
          className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors"
          title={t("git.copyFullHash")}
        >
          {copied ? (
            <>
              <Check className="w-3 h-3 text-tag-green-foreground" />
              {t("git.copied")}
            </>
          ) : (
            <>
              <Copy className="w-3 h-3" />
              {t("git.copyFullHash")}
            </>
          )}
        </button>
      </div>
    </div>
  );
}

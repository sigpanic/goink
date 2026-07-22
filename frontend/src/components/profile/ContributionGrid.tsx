import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";

interface Props {
  data: Record<string, number>; // "YYYY-MM-DD" -> 字数
  months?: number;
}

const LEVELS = [
  { max: 0, cls: "bg-contribution-0" },
  { max: 100, cls: "bg-contribution-1" },
  { max: 500, cls: "bg-contribution-2" },
  { max: 2000, cls: "bg-contribution-3" },
  { max: Infinity, cls: "bg-contribution-4" },
];

function levelClass(words: number): string {
  for (const l of LEVELS) {
    if (words <= l.max) return l.cls;
  }
  return LEVELS[0].cls;
}

function formatDate(dateStr: string, locale: string): string {
  const d = new Date(dateStr + "T00:00:00");
  return new Intl.DateTimeFormat(locale, {
    year: "numeric",
    month: "long",
    day: "numeric",
  }).format(d);
}

export default function ContributionGrid({ data, months = 12 }: Props) {
  const { t, i18n } = useTranslation();
  const [tooltip, setTooltip] = useState<{
    date: string;
    words: number;
    x: number;
    y: number;
  } | null>(null);
  const [today] = useState(() => {
    const d = new Date();
    d.setHours(0, 0, 0, 0);
    return d;
  });

  const weeks = useMemo(() => {
    const end = today;
    const start = new Date(end);
    start.setMonth(end.getMonth() - months);
    // 对齐到周日
    start.setDate(start.getDate() - start.getDay());

    const result: { date: string; words: number }[][] = [];
    const cur = new Date(start);
    while (cur <= end) {
      const week: { date: string; words: number }[] = [];
      for (let i = 0; i < 7; i++) {
        const ds = cur.toISOString().slice(0, 10);
        week.push({ date: ds, words: data[ds] ?? 0 });
        cur.setDate(cur.getDate() + 1);
      }
      result.push(week);
    }
    return result;
  }, [data, months, today]);

  const monthLabels = useMemo(() => {
    const labels: { label: string; span: number }[] = [];
    weeks.forEach((week, i) => {
      const midDay = week[3]?.date; // 用周三判断月份
      if (!midDay) return;
      const month = midDay.slice(0, 7);
      const last = labels[labels.length - 1];
      if (!last || last.label !== month) {
        if (last)
          last.span = i - labels.slice(0, -1).reduce((s, l) => s + l.span, 0);
        labels.push({ label: month, span: 0 });
      }
    });
    if (labels.length > 0) {
      labels[labels.length - 1].span =
        weeks.length - labels.slice(0, -1).reduce((s, l) => s + l.span, 0);
    }
    return labels.map((l) => {
      const parts = l.label.split("-");
      return {
        label: new Intl.DateTimeFormat(i18n.language, {
          month: "short",
        }).format(new Date(parseInt(parts[0], 10), parseInt(parts[1], 10) - 1)),
        span: l.span,
      };
    });
  }, [weeks, i18n.language]);

  const showTooltip = (e: React.MouseEvent, date: string, words: number) => {
    const rect = (e.target as HTMLElement).getBoundingClientRect();
    setTooltip({
      date,
      words,
      x: rect.left + rect.width / 2,
      y: rect.top - 32,
    });
  };

  return (
    <div className="relative select-none">
      {/* 月份标签 */}
      <div
        className="flex text-[10px] text-muted-foreground mb-1"
        style={{ paddingLeft: 28 }}
      >
        {monthLabels.map((m, i) => (
          <span key={i} className="text-left" style={{ width: m.span * 16 }}>
            {m.label}
          </span>
        ))}
      </div>

      <div className="flex gap-[3px]">
        {/* 星期标签 */}
        <div
          className="flex flex-col gap-[3px] text-[10px] text-muted-foreground pr-2"
          style={{ width: 22 }}
        >
          <span className="h-[13px] leading-[13px]" />
          <span className="h-[13px] leading-[13px]">{t("profile.mon")}</span>
          <span className="h-[13px] leading-[13px]" />
          <span className="h-[13px] leading-[13px]">{t("profile.wed")}</span>
          <span className="h-[13px] leading-[13px]" />
          <span className="h-[13px] leading-[13px]">{t("profile.fri")}</span>
          <span className="h-[13px] leading-[13px]" />
        </div>

        {/* 格子矩阵 */}
        <div className="flex gap-[3px]">
          {weeks.map((week, wi) => (
            <div key={wi} className="flex flex-col gap-[3px]">
              {week.map((day, di) => (
                <div
                  key={di}
                  className={`w-[13px] h-[13px] rounded-[2px] ${levelClass(day.words)} cursor-pointer select-none`}
                  onMouseEnter={(e) => showTooltip(e, day.date, day.words)}
                  onMouseLeave={() => setTooltip(null)}
                />
              ))}
            </div>
          ))}
        </div>
      </div>

      {/* 图例 */}
      <div className="flex items-center gap-1 mt-2 justify-end text-[10px] text-muted-foreground">
        <span>{t("profile.less")}</span>
        {LEVELS.map((l, i) => (
          <div key={i} className={`w-[10px] h-[10px] rounded-[2px] ${l.cls}`} />
        ))}
        <span className="ml-1">{t("profile.more")}</span>
      </div>

      {/* Tooltip */}
      {tooltip && (
        <div
          className="fixed z-50 px-2 py-1 rounded text-xs bg-foreground text-background whitespace-nowrap pointer-events-none -translate-x-1/2"
          style={{ left: tooltip.x, top: tooltip.y }}
        >
          {tooltip.words > 0
            ? `${tooltip.words.toLocaleString()} ${t("profile.charUnit")}`
            : t("profile.noWriting")}{" "}
          · {formatDate(tooltip.date, i18n.language)}
        </div>
      )}
    </div>
  );
}

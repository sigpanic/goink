import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Sparkles } from "lucide-react";
import type { AnnualReport } from "./useAnnualReport";

export default function MonthlyChart({ report }: { report: AnnualReport }) {
  const { t, i18n } = useTranslation();
  const [metric, setMetric] = useState<"words" | "hours">("words");
  const [selected, setSelected] = useState<number | null>(null);
  const values = report.Monthly.map((month) =>
    metric === "words" ? month.WordsAdded : month.CreativeSeconds / 3600,
  );
  const maximum = Math.max(...values, 0);
  const peak = values.indexOf(maximum);
  const highlight = selected ?? peak;
  const formatValue = (value: number) =>
    t(
      metric === "words" ? "annualReport.wordValue" : "annualReport.hourValue",
      {
        value: new Intl.NumberFormat(i18n.language, {
          maximumFractionDigits: metric === "words" ? 0 : 1,
        }).format(value),
      },
    );
  const monthName = (month: number) =>
    new Intl.DateTimeFormat(i18n.language, { month: "short" }).format(
      new Date(report.Year, month - 1, 1),
    );
  return (
    <section
      className="report-panel report-trend"
      aria-labelledby="report-trend-title"
    >
      <div className="report-panel-heading">
        <div>
          <span className="report-kicker">{t("annualReport.trendKicker")}</span>
          <h3 id="report-trend-title">{t("annualReport.trendTitle")}</h3>
        </div>
        <div
          className="report-segmented"
          aria-label={t("annualReport.trendMetric")}
        >
          {(["words", "hours"] as const).map((value) => (
            <button
              type="button"
              key={value}
              aria-pressed={metric === value}
              onClick={() => {
                setMetric(value);
                setSelected(null);
              }}
            >
              {t(
                value === "words"
                  ? "annualReport.wordsMetric"
                  : "annualReport.hoursMetric",
              )}
            </button>
          ))}
        </div>
      </div>
      <div className="report-chart" aria-label={t("annualReport.monthlyChart")}>
        {report.Monthly.map((month, index) => {
          const start = `${report.Year}-${String(month.Month).padStart(2, "0")}-01`;
          const end = `${month.Month === 12 ? report.Year + 1 : report.Year}-${String(month.Month === 12 ? 1 : month.Month + 1).padStart(2, "0")}-01`;
          const isCovered =
            !!report.Coverage &&
            start < report.Coverage.End &&
            end > report.Coverage.Start;
          const label = `${monthName(month.Month)} · ${isCovered ? formatValue(values[index]) : t("annualReport.notCovered")}`;
          return (
            <div className="report-chart-column" key={month.Month}>
              <button
                type="button"
                className="report-chart-bar"
                style={{
                  height: `${maximum > 0 ? Math.max((values[index] / maximum) * 100, 2) : 2}%`,
                }}
                data-highlight={maximum > 0 && highlight === index}
                data-covered={isCovered}
                disabled={!isCovered}
                aria-pressed={selected === index}
                aria-label={label}
                title={label}
                onClick={() => setSelected(index)}
              />
              <span>{monthName(month.Month)}</span>
            </div>
          );
        })}
      </div>
      <div className="report-chart-insight" aria-live="polite">
        <Sparkles size={14} />
        <span>
          {maximum > 0
            ? t(
                selected === null
                  ? metric === "words"
                    ? "annualReport.wordsPeakMonth"
                    : "annualReport.hoursPeakMonth"
                  : "annualReport.selectedMonth",
                {
                  month: monthName(report.Monthly[highlight].Month),
                  value: formatValue(values[highlight]),
                },
              )
            : t("annualReport.noTrend")}
        </span>
      </div>
    </section>
  );
}

import styles from "./report.module.css";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Sparkles } from "lucide-react";
import type { AnnualReport } from "../../useAnnualReport";

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
      metric === "words"
        ? "annualReport.editions.2026.wordValue"
        : "annualReport.editions.2026.hourValue",
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
      className={[styles["report-panel"], styles["report-trend"]].join(" ")}
      aria-labelledby="report-trend-title"
    >
      <div className={styles["report-panel-heading"]}>
        <div>
          <span className={styles["report-kicker"]}>
            {t("annualReport.editions.2026.trendKicker")}
          </span>
          <h3 id="report-trend-title">
            {t("annualReport.editions.2026.trendTitle")}
          </h3>
        </div>
        <div
          className={styles["report-segmented"]}
          aria-label={t("annualReport.editions.2026.trendMetric")}
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
                  ? "annualReport.editions.2026.wordsMetric"
                  : "annualReport.editions.2026.hoursMetric",
              )}
            </button>
          ))}
        </div>
      </div>
      <div
        className={styles["report-chart"]}
        aria-label={t("annualReport.editions.2026.monthlyChart")}
      >
        {report.Monthly.map((month, index) => {
          const start = `${report.Year}-${String(month.Month).padStart(2, "0")}-01`;
          const end = `${month.Month === 12 ? report.Year + 1 : report.Year}-${String(month.Month === 12 ? 1 : month.Month + 1).padStart(2, "0")}-01`;
          const isCovered =
            !!report.Coverage &&
            start < report.Coverage.End &&
            end > report.Coverage.Start;
          const label = `${monthName(month.Month)} · ${isCovered ? formatValue(values[index]) : t("annualReport.editions.2026.notCovered")}`;
          return (
            <div className={styles["report-chart-column"]} key={month.Month}>
              <button
                type="button"
                className={styles["report-chart-bar"]}
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
      <div className={styles["report-chart-insight"]} aria-live="polite">
        <Sparkles size={14} />
        <span>
          {maximum > 0
            ? t(
                selected === null
                  ? metric === "words"
                    ? "annualReport.editions.2026.wordsPeakMonth"
                    : "annualReport.editions.2026.hoursPeakMonth"
                  : "annualReport.editions.2026.selectedMonth",
                {
                  month: monthName(report.Monthly[highlight].Month),
                  value: formatValue(values[highlight]),
                },
              )
            : t("annualReport.editions.2026.noTrend")}
        </span>
      </div>
    </section>
  );
}

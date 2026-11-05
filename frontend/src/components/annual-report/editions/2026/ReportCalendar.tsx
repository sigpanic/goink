import styles from "./report.module.css";
import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { CalendarDays } from "lucide-react";
import type { AnnualReport } from "../../useAnnualReport";

export default function ReportCalendar({ report }: { report: AnnualReport }) {
  const { t, i18n } = useTranslation();
  const [selected, setSelected] = useState<string | null>(null);
  const days = useMemo(() => {
    const first = Date.UTC(report.Year, 0, 1);
    const length = (Date.UTC(report.Year + 1, 0, 1) - first) / 86_400_000;
    return Array.from(
      { length },
      (_, index) => new Date(first + index * 86_400_000),
    );
  }, [report.Year]);
  const changes = useMemo(
    () => new Map(report.DailyWriting.map((day) => [day.date, day])),
    [report.DailyWriting],
  );
  const offset = days[0].getUTCDay();
  const weeks = Math.ceil((days.length + offset) / 7);
  const dateFormat = new Intl.DateTimeFormat(i18n.language, {
    month: "short",
    day: "numeric",
    year: "numeric",
    timeZone: "UTC",
  });
  const covered = (date: string) =>
    !!report.Coverage &&
    date >= report.Coverage.Start &&
    date < report.Coverage.End;
  function description(date: string) {
    const label = dateFormat.format(new Date(date + "T00:00:00Z"));
    if (!covered(date))
      return `${label} · ${t("annualReport.editions.2026.notCovered")}`;
    const day = changes.get(date);
    if (!day) return `${label} · ${t("annualReport.editions.2026.noEdits")}`;
    return `${label} · ${t("profile.dailyChanges", {
      added: day.words_added.toLocaleString(i18n.language),
      deleted: day.words_deleted.toLocaleString(i18n.language),
      net:
        (day.words_net > 0 ? "+" : "") +
        day.words_net.toLocaleString(i18n.language),
    })}`;
  }
  return (
    <section
      className={[styles["report-panel"], styles["report-calendar-panel"]].join(
        " ",
      )}
      aria-labelledby="report-calendar-title"
    >
      <div className={styles["report-panel-heading"]}>
        <div>
          <span className={styles["report-kicker"]}>
            {t("annualReport.editions.2026.calendarKicker")}
          </span>
          <h3 id="report-calendar-title">
            {t("annualReport.editions.2026.calendarTitle")}
          </h3>
        </div>
        <span className={styles["report-panel-note"]}>
          <CalendarDays size={13} />
          {t("annualReport.editions.2026.activeDaysNote", {
            days: report.Writing.ActiveDays,
          })}
        </span>
      </div>
      <div className={styles["report-calendar-scroll"]}>
        <div className={styles["report-calendar-inner"]}>
          <div
            className={styles["report-calendar-months"]}
            style={{ gridTemplateColumns: `repeat(${weeks}, minmax(0, 1fr))` }}
            aria-hidden="true"
          >
            {Array.from({ length: 12 }, (_, month) => {
              const day = new Date(Date.UTC(report.Year, month, 1));
              const index = Math.round(
                (day.getTime() - days[0].getTime()) / 86_400_000,
              );
              return (
                <span
                  key={month}
                  style={{ gridColumn: Math.floor((index + offset) / 7) + 1 }}
                >
                  {new Intl.DateTimeFormat(i18n.language, {
                    month: "short",
                    timeZone: "UTC",
                  }).format(day)}
                </span>
              );
            })}
          </div>
          <div className={styles["report-calendar-body"]}>
            <div
              className={styles["report-calendar-weekdays"]}
              aria-hidden="true"
            >
              <span />
              <span>{t("profile.mon")}</span>
              <span />
              <span>{t("profile.wed")}</span>
              <span />
              <span>{t("profile.fri")}</span>
              <span />
            </div>
            <div
              className={styles["report-calendar-grid"]}
              aria-label={t("annualReport.editions.2026.calendarTitle")}
            >
              {Array.from({ length: offset }, (_, i) => (
                <span key={`blank-${i}`} />
              ))}
              {days.map((day) => {
                const date = day.toISOString().slice(0, 10);
                const row = changes.get(date);
                const edits =
                  (row?.words_added ?? 0) + (row?.words_deleted ?? 0);
                const level =
                  edits === 0
                    ? 0
                    : edits <= 100
                      ? 1
                      : edits <= 500
                        ? 2
                        : edits <= 2000
                          ? 3
                          : 4;
                return (
                  <button
                    type="button"
                    key={date}
                    className={styles["report-calendar-day"]}
                    data-date={date}
                    data-level={covered(date) ? level : undefined}
                    aria-label={description(date)}
                    title={description(date)}
                    onMouseEnter={() => setSelected(date)}
                    onFocus={() => setSelected(date)}
                    onClick={() => setSelected(date)}
                  />
                );
              })}
            </div>
          </div>
        </div>
      </div>
      <div className={styles["report-calendar-footer"]}>
        <p>
          {selected
            ? description(selected)
            : t("annualReport.editions.2026.calendarHint")}
        </p>
        <div className={styles["report-calendar-legend"]} aria-hidden="true">
          <span>{t("profile.less")}</span>
          {[0, 1, 2, 3, 4].map((level) => (
            <i key={level} data-level={level} />
          ))}
          <span>{t("profile.more")}</span>
        </div>
      </div>
    </section>
  );
}

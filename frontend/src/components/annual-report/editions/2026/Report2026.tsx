import { Feather, CalendarDays } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { AnnualReportEditionProps } from "../../types";
import ReportCover from "./ReportCover";
import ReportBook from "./ReportBook";
import {
  AchievementPage,
  AIPage,
  WorldPage,
  StoryPage,
  HabitsPage,
  InspirationPage,
} from "./ReportPages";
import ReportShare from "./ReportShare";
import styles from "./report.module.css";

export default function Report2026({
  report,
  name,
  locale,
}: AnnualReportEditionProps) {
  const { t } = useTranslation();
  const date = (value: string) =>
    new Intl.DateTimeFormat(locale, {
      year: "numeric",
      month: "short",
      day: "numeric",
    }).format(new Date(value + "T00:00:00"));
  const hasData =
    report.Writing.ActiveDays > 0 ||
    report.UsageCount > 0 ||
    Object.values(report.Activity).some((value) => value > 0);
  const lastCovered = report.Coverage
    ? new Date(report.Coverage.End + "T00:00:00")
    : null;
  if (lastCovered) lastCovered.setDate(lastCovered.getDate() - 1);
  const coverage =
    report.Coverage && lastCovered
      ? t("annualReport.editions.2026.coverage", {
          start: date(report.Coverage.Start),
          end: new Intl.DateTimeFormat(locale, {
            year: "numeric",
            month: "short",
            day: "numeric",
          }).format(lastCovered),
        })
      : t("annualReport.editions.2026.uncoveredYear", {
          start: date(report.TrackingStartDate),
        });
  const ending = (
    <div className={styles["report-ending"]}>
      <span className={styles["report-ending-year"]} aria-hidden="true">
        {report.Year}
      </span>
      <Feather size={30} />
      <span className={styles["report-kicker"]}>
        {t("annualReport.editions.2026.nextChapter")}
      </span>
      <h2>
        {t(
          hasData
            ? "annualReport.editions.2026.closingTitle"
            : report.Coverage
              ? "annualReport.editions.2026.emptyTitle"
              : "annualReport.editions.2026.uncoveredTitle",
        )}
      </h2>
      <p>
        {t(
          hasData
            ? "annualReport.editions.2026.closingNote"
            : report.Coverage
              ? "annualReport.editions.2026.emptyNote"
              : "annualReport.editions.2026.uncoveredNote",
        )}
      </p>
      <ReportShare report={report} name={name} />
      <span className={styles["report-ending-signature"]}>{name} · Goink</span>
    </div>
  );
  const pages = [
    {
      id: "cover",
      label: t("annualReport.editions.2026.pageCover"),
      content: (
        <>
          <ReportCover report={report} name={name} hasData={hasData} />
          <div className={styles["report-coverage"]}>
            <span className={styles["report-coverage-dot"]} />
            <span>{coverage}</span>
            <span className={styles["report-coverage-note"]}>
              {t("annualReport.editions.2026.approximate")}
            </span>
          </div>
        </>
      ),
    },
    ...(hasData
      ? [
          {
            id: "works",
            label: t("annualReport.editions.2026.pageWorlds"),
            content: <AchievementPage report={report} locale={locale} />,
          },
          {
            id: "world",
            label: t("annualReport.editions.2026.pageCharacters"),
            content: <WorldPage report={report} locale={locale} />,
          },
          {
            id: "story",
            label: t("annualReport.editions.2026.pageStory"),
            content: <StoryPage report={report} locale={locale} />,
          },
          {
            id: "habits",
            label: t("annualReport.editions.2026.pageCalendar"),
            content: <HabitsPage report={report} locale={locale} />,
          },
          {
            id: "inspiration",
            label: t("annualReport.editions.2026.pageInspiration"),
            content: <InspirationPage report={report} locale={locale} />,
          },
          {
            id: "ai",
            label: t("annualReport.editions.2026.pageAI"),
            content: <AIPage report={report} locale={locale} />,
          },
        ]
      : []),
    {
      id: "ending",
      label: t("annualReport.editions.2026.pageEnding"),
      content: ending,
    },
  ];
  return (
    <section className={styles.edition} data-report-edition="2026">
      <ReportBook
        pages={pages}
        year={report.Year}
        title={t("annualReport.editions.2026.personalTitle", { name })}
        footer={
          <span className={styles["report-tracking-note"]}>
            <CalendarDays size={11} />
            {t("annualReport.editions.2026.trackingSince", {
              date: date(report.TrackingStartDate),
            })}
            <span> · {t("annualReport.editions.2026.approximate")}</span>
          </span>
        }
      />
    </section>
  );
}

import { useState } from "react";
import { useTranslation } from "react-i18next";
import {
  ArrowLeft,
  Feather,
  RefreshCw,
  LoaderCircle,
  AlertCircle,
  CalendarDays,
} from "lucide-react";
import { useProfileSettings } from "@/components/profile/useProfileSettings";
import { useAnnualReport } from "./useAnnualReport";
import type { AnnualReport } from "./useAnnualReport";
import ReportCover from "./ReportCover";
import ReportCalendar from "./ReportCalendar";
import MonthlyChart from "./MonthlyChart";
import ReportBook from "./ReportBook";
import {
  AchievementPage,
  AIPage,
  LeadStat,
  NovelRanking,
  PageHeading,
} from "./ReportPages";
import ReportShare from "./ReportShare";
import "./annual-report.css";

export default function AnnualReportView({ onBack }: { onBack: () => void }) {
  const { t, i18n } = useTranslation();
  const [currentYear] = useState(() => new Date().getFullYear());
  const [year, setYear] = useState(currentYear);
  const query = useAnnualReport(year);
  const settings = useProfileSettings();
  const firstYear = Math.min(
    Number(query.data?.TrackingStartDate.slice(0, 4)) || year,
    year,
  );
  const years = Array.from(
    { length: currentYear - firstYear + 1 },
    (_, i) => currentYear - i,
  );
  const loading = query.isPending || query.isPlaceholderData;
  return (
    <main
      className="annual-report flex-1 min-w-0"
      aria-label={t("annualReport.title")}
    >
      <div className="report-shell">
        <header className="report-stage-tools">
          <button type="button" className="report-back" onClick={onBack}>
            <ArrowLeft size={15} />
            {t("annualReport.back")}
          </button>
          <div className="report-stage-actions">
            <label className="report-year-label">
              <span>{t("annualReport.yearLabel")}</span>
              <select
                aria-label={t("annualReport.yearLabel")}
                value={year}
                onChange={(event) => setYear(Number(event.target.value))}
              >
                {years.map((option) => (
                  <option key={option} value={option}>
                    {t("annualReport.yearOption", { year: option })}
                  </option>
                ))}
              </select>
            </label>
            <button
              type="button"
              className="report-refresh"
              onClick={() => void query.refetch()}
              disabled={query.isFetching}
              aria-label={t("annualReport.refresh")}
              title={t("annualReport.refresh")}
            >
              <RefreshCw
                size={15}
                className={query.isFetching ? "animate-spin" : ""}
              />
            </button>
          </div>
        </header>
        {loading ? (
          <div className="report-state" role="status">
            <LoaderCircle className="animate-spin" size={25} />
            <h1>{t("annualReport.loading")}</h1>
            <p>{t("annualReport.loadingNote")}</p>
            <div className="report-loading-lines">
              <i />
              <i />
              <i />
            </div>
          </div>
        ) : query.isError || !query.data ? (
          <div className="report-state" role="alert">
            <AlertCircle size={28} />
            <h1>{t("annualReport.loadFailed")}</h1>
            <p>{t("annualReport.errorNote")}</p>
            <button
              type="button"
              className="report-button"
              onClick={() => void query.refetch()}
            >
              {t("annualReport.retry")}
            </button>
          </div>
        ) : (
          <ReportContent
            key={year}
            report={query.data}
            name={settings.data?.user_name || t("annualReport.author")}
            locale={i18n.language}
          />
        )}
      </div>
    </main>
  );
}

function ReportContent({
  report,
  name,
  locale,
}: {
  report: AnnualReport;
  name: string;
  locale: string;
}) {
  const { t } = useTranslation();
  const number = (value: number) => value.toLocaleString(locale);
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
      ? t("annualReport.coverage", {
          start: date(report.Coverage.Start),
          end: new Intl.DateTimeFormat(locale, {
            year: "numeric",
            month: "short",
            day: "numeric",
          }).format(lastCovered),
        })
      : t("annualReport.uncoveredYear", {
          start: date(report.TrackingStartDate),
        });
  const ending = (
    <div className="report-ending">
      <span className="report-ending-year" aria-hidden="true">
        {report.Year}
      </span>
      <Feather size={30} />
      <span className="report-kicker">{t("annualReport.nextChapter")}</span>
      <h2>
        {t(
          hasData
            ? "annualReport.closingTitle"
            : report.Coverage
              ? "annualReport.emptyTitle"
              : "annualReport.uncoveredTitle",
        )}
      </h2>
      <p>
        {t(
          hasData
            ? "annualReport.closingNote"
            : report.Coverage
              ? "annualReport.emptyNote"
              : "annualReport.uncoveredNote",
        )}
      </p>
      <ReportShare report={report} name={name} />
      <span className="report-ending-signature">{name} · Goink</span>
    </div>
  );
  const pages = [
    {
      id: "cover",
      label: t("annualReport.pageCover"),
      content: (
        <>
          <ReportCover report={report} name={name} hasData={hasData} />
          <div className="report-coverage">
            <span className="report-coverage-dot" />
            <span>{coverage}</span>
            <span className="report-coverage-note">
              {t("annualReport.approximate")}
            </span>
          </div>
        </>
      ),
    },
    ...(hasData
      ? [
          {
            id: "achievements",
            label: t("annualReport.pageAchievements"),
            content: <AchievementPage report={report} locale={locale} />,
          },
          {
            id: "calendar",
            label: t("annualReport.pageCalendar"),
            content: (
              <>
                <PageHeading
                  index="02"
                  title={t("annualReport.habitsTitle")}
                  note={t("annualReport.habitsNote")}
                />
                <div className="report-calendar-stats">
                  <LeadStat
                    label={t("annualReport.activeDays")}
                    value={number(report.Writing.ActiveDays)}
                    unit={t("annualReport.days")}
                  />
                  <LeadStat
                    label={t("annualReport.longestStreak")}
                    value={number(report.Writing.LongestStreak)}
                    unit={t("annualReport.days")}
                  />
                </div>
                <ReportCalendar report={report} />
              </>
            ),
          },
          {
            id: "worlds",
            label: t("annualReport.pageWorlds"),
            content: (
              <>
                <PageHeading
                  index="03"
                  title={t("annualReport.growthTitle")}
                  note={t("annualReport.booksNote")}
                />
                <div
                  className={`report-two-columns ${report.Novels.length === 0 ? "report-single-column" : ""}`}
                >
                  <MonthlyChart report={report} />
                  <NovelRanking report={report} locale={locale} />
                </div>
              </>
            ),
          },
          {
            id: "ai",
            label: t("annualReport.pageAI"),
            content: <AIPage report={report} locale={locale} />,
          },
        ]
      : []),
    { id: "ending", label: t("annualReport.pageEnding"), content: ending },
  ];
  return (
    <ReportBook
      pages={pages}
      year={report.Year}
      title={t("annualReport.personalTitle", { name })}
      footer={
        <span className="report-tracking-note">
          <CalendarDays size={11} />
          {t("annualReport.trackingSince", {
            date: date(report.TrackingStartDate),
          })}
          <span> · {t("annualReport.approximate")}</span>
        </span>
      }
    />
  );
}

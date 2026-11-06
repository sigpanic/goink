import styles from "./report.module.css";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import WorldIllustration from "./WorldIllustration";
import type { AnnualReport } from "../../useAnnualReport";

export default function ReportCover({
  report,
  name,
  hasData,
}: {
  report: AnnualReport;
  name: string;
  hasData: boolean;
}) {
  const { t, i18n } = useTranslation();
  const [avatarFailed, setAvatarFailed] = useState(false);
  const seconds = report.Activity.CreativeSeconds;
  const minutes = seconds > 0 && seconds < 3600;
  const time = new Intl.NumberFormat(i18n.language, {
    maximumFractionDigits: 1,
  }).format(minutes ? seconds / 60 : seconds / 3600);
  return (
    <section
      className={styles["report-hero"]}
      aria-labelledby="report-hero-title"
    >
      <div className={styles["report-hero-copy"]}>
        <div className={styles["report-hero-edition"]}>
          <span />
          {report.Year}
          <i />
          {t("annualReport.editions.2026.edition")}
        </div>
        <span className={styles["report-cover-kicker"]}>
          {t("annualReport.editions.2026.eyebrow")}
        </span>
        <h1 id="report-hero-title">
          {t(
            hasData
              ? "annualReport.editions.2026.heroFirst"
              : "annualReport.editions.2026.emptyHeroFirst",
          )}
          <br />
          <em>
            {t(
              hasData
                ? "annualReport.editions.2026.heroSecond"
                : "annualReport.editions.2026.emptyHeroSecond",
            )}
          </em>
        </h1>
        <p className={styles["report-hero-description"]}>
          {t(
            hasData
              ? "annualReport.editions.2026.heroDescription"
              : "annualReport.editions.2026.emptyHeroDescription",
          )}
        </p>
        <div className={styles["report-signature"]}>
          <span>
            {Array.from(name)[0]}
            {!avatarFailed && (
              <img src="/avatar" alt="" onError={() => setAvatarFailed(true)} />
            )}
          </span>
          <div>
            {name}
            <small>{t("annualReport.editions.2026.signature")}</small>
          </div>
        </div>
        {hasData && (
          <dl className={styles["report-cover-totals"]}>
            <div>
              <dt>{t("annualReport.editions.2026.wordsAdded")}</dt>
              <dd>
                {report.Writing.WordsAdded.toLocaleString(i18n.language)}
                <small>{t("annualReport.editions.2026.wordsUnit")}</small>
              </dd>
            </div>
            <div>
              <dt>{t("annualReport.editions.2026.creativeTime")}</dt>
              <dd>
                {time}
                <small>
                  {t(
                    minutes
                      ? "annualReport.editions.2026.minutes"
                      : "annualReport.editions.2026.hours",
                  )}
                </small>
              </dd>
            </div>
          </dl>
        )}
      </div>
      <div className={styles["report-hero-year"]} aria-hidden="true">
        {report.Year}
      </div>
      <WorldIllustration />
      <span className={styles["report-hero-caption"]}>
        {t("annualReport.editions.2026.heroCaption")}
      </span>
    </section>
  );
}

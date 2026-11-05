import { useState } from "react";
import { useTranslation } from "react-i18next";
import WorldIllustration from "./WorldIllustration";
import type { AnnualReport } from "./useAnnualReport";

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
    <section className="report-hero" aria-labelledby="report-hero-title">
      <div className="report-hero-copy">
        <div className="report-hero-edition">
          <span />
          {report.Year}
          <i />
          {t("annualReport.edition")}
        </div>
        <span className="report-cover-kicker">{t("annualReport.eyebrow")}</span>
        <h1 id="report-hero-title">
          {t(
            hasData ? "annualReport.heroFirst" : "annualReport.emptyHeroFirst",
          )}
          <br />
          <em>
            {t(
              hasData
                ? "annualReport.heroSecond"
                : "annualReport.emptyHeroSecond",
            )}
          </em>
        </h1>
        <p className="report-hero-description">
          {t(
            hasData
              ? "annualReport.heroDescription"
              : "annualReport.emptyHeroDescription",
          )}
        </p>
        <div className="report-signature">
          <span>
            {Array.from(name)[0]}
            {!avatarFailed && (
              <img src="/avatar" alt="" onError={() => setAvatarFailed(true)} />
            )}
          </span>
          <div>
            {name}
            <small>{t("annualReport.signature")}</small>
          </div>
        </div>
        {hasData && (
          <dl className="report-cover-totals">
            <div>
              <dt>{t("annualReport.wordsAdded")}</dt>
              <dd>
                {report.Writing.WordsAdded.toLocaleString(i18n.language)}
                <small>{t("annualReport.wordsUnit")}</small>
              </dd>
            </div>
            <div>
              <dt>{t("annualReport.creativeTime")}</dt>
              <dd>
                {time}
                <small>
                  {t(minutes ? "annualReport.minutes" : "annualReport.hours")}
                </small>
              </dd>
            </div>
          </dl>
        )}
      </div>
      <div className="report-hero-year" aria-hidden="true">
        {report.Year}
      </div>
      <WorldIllustration />
      <span className="report-hero-caption">
        {t("annualReport.heroCaption")}
      </span>
    </section>
  );
}

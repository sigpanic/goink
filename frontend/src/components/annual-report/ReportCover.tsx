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
  const { t } = useTranslation();
  const [avatarFailed, setAvatarFailed] = useState(false);
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

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { ArrowLeft, RefreshCw, LoaderCircle, AlertCircle } from "lucide-react";
import { useProfileSettings } from "@/components/profile/useProfileSettings";
import { AppErr } from "@/utils/wailsResult";
import { errorCodeMessage, toErrorMessage } from "@/utils/error";
import { useAnnualReport } from "./useAnnualReport";
import { DefaultReportEdition, reportEditions } from "./editions";
import { defaultReportYear } from "./useReportSeason";
import styles from "./annual-report.module.css";

export default function AnnualReportView({ onBack }: { onBack: () => void }) {
  const { t, i18n } = useTranslation();
  const [currentYear] = useState(() => new Date().getFullYear());
  const [year, setYear] = useState(() => defaultReportYear(new Date()));
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
  const Edition =
    reportEditions[query.data?.Year ?? year] ?? DefaultReportEdition;
  return (
    <main
      className={[styles["annual-report"], "flex-1", "min-w-0"].join(" ")}
      aria-label={t("annualReport.title")}
    >
      <div className={styles["report-shell"]}>
        <header className={styles["report-stage-tools"]}>
          <button
            type="button"
            className={styles["report-back"]}
            onClick={onBack}
          >
            <ArrowLeft size={15} />
            {t("annualReport.back")}
          </button>
          <div className={styles["report-stage-actions"]}>
            <label className={styles["report-year-label"]}>
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
              className={styles["report-refresh"]}
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
          <div className={styles["report-state"]} role="status">
            <LoaderCircle className="animate-spin" size={25} />
            <h1>{t("annualReport.loading")}</h1>
            <p>{t("annualReport.loadingNote")}</p>
            <div className={styles["report-loading-lines"]}>
              <i />
              <i />
              <i />
            </div>
          </div>
        ) : query.isError || !query.data ? (
          <div className={styles["report-state"]} role="alert">
            <AlertCircle size={28} />
            <h1>{t("annualReport.loadFailed")}</h1>
            <p>{t("annualReport.errorNote")}</p>
            {query.error && (
              <p className="max-w-full break-words">
                {query.error instanceof AppErr
                  ? (errorCodeMessage(query.error.errCode, t) ??
                    toErrorMessage(query.error))
                  : toErrorMessage(query.error)}
              </p>
            )}
            <button
              type="button"
              className={styles["report-button"]}
              onClick={() => void query.refetch()}
            >
              {t("annualReport.retry")}
            </button>
          </div>
        ) : (
          <Edition
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

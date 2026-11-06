import { ArrowUpRight, BookOpen } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { useReportSeason } from "@/components/annual-report/useReportSeason";
import styles from "@/components/annual-report/annual-report.module.css";

export default function AnnualReportEntry({
  season,
  onOpen,
}: {
  season: ReturnType<typeof useReportSeason>;
  onOpen: () => void;
}) {
  const { t } = useTranslation();
  if (!season.available) return null;

  return (
    <button
      type="button"
      className={styles["annual-report-entry"]}
      data-annual-report-entry
      onClick={onOpen}
    >
      <span className={styles["annual-report-entry-icon"]}>
        <BookOpen size={24} />
      </span>
      <span className={styles["annual-report-entry-copy"]}>
        <strong>{t("annualReport.entryTitle")}</strong>
        <span>{t("annualReport.entryNote")}</span>
      </span>
      <span className={styles["annual-report-entry-year"]} aria-hidden="true">
        {season.reportYear}
      </span>
      <ArrowUpRight size={21} />
    </button>
  );
}

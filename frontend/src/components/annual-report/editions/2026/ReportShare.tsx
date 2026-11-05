import { useTranslation } from "react-i18next";
import { Dialog } from "radix-ui";
import { Download, ImagePlus, LoaderCircle, Check, X } from "lucide-react";
import { createSharePoster } from "./sharePoster";
import type { AnnualReport } from "../../useAnnualReport";
import { useReportShare } from "../../shared/useReportShare";
import styles from "./report.module.css";

export default function ReportShare({
  report,
  name,
}: {
  report: AnnualReport;
  name: string;
}) {
  const { t, i18n } = useTranslation();
  const {
    image,
    open,
    setOpen,
    generating,
    saving,
    saved,
    error,
    generate,
    save,
  } = useReportShare(report.Year, async () => {
    const hasData =
      report.Writing.ActiveDays > 0 ||
      report.UsageCount > 0 ||
      Object.values(report.Activity).some((value) => value > 0);
    const end = report.Coverage
      ? new Date(report.Coverage.End + "T00:00:00")
      : null;
    if (end) end.setDate(end.getDate() - 1);
    const date = (value: Date) =>
      new Intl.DateTimeFormat(i18n.language, {
        year: "numeric",
        month: "short",
        day: "numeric",
      }).format(value);
    const data = await createSharePoster(report, i18n.language, {
      owner: t("annualReport.editions.2026.posterOwner", {
        year: report.Year,
        name,
      }),
      first: t(
        hasData
          ? "annualReport.editions.2026.heroFirst"
          : "annualReport.editions.2026.emptyHeroFirst",
      ),
      second: t(
        hasData
          ? "annualReport.editions.2026.heroSecond"
          : "annualReport.editions.2026.emptyHeroSecond",
      ),
      words: t("annualReport.editions.2026.wordsAdded"),
      days: t("annualReport.editions.2026.activeDays"),
      characters: t("annualReport.editions.2026.characters"),
      time: t(
        report.Activity.CreativeSeconds > 0 &&
          report.Activity.CreativeSeconds < 3600
          ? "annualReport.editions.2026.posterMinutes"
          : "annualReport.editions.2026.posterHours",
      ),
      summary: t("annualReport.editions.2026.posterSummary", {
        novels: report.Activity.NovelsCreated,
        chapters: report.Activity.ChaptersCreated,
        locations: report.Activity.LocationsCreated,
      }),
      storySummary: t("annualReport.editions.2026.posterStorySummary", {
        arcs: report.Activity.StoryArcsCreated,
        entries: report.Activity.TimelineEntriesCreated,
      }),
      coverage:
        report.Coverage && end
          ? t("annualReport.editions.2026.coverage", {
              start: date(new Date(report.Coverage.Start + "T00:00:00")),
              end: date(end),
            })
          : t("annualReport.editions.2026.uncoveredYear", {
              start: date(new Date(report.TrackingStartDate + "T00:00:00")),
            }),
      approximate: t("annualReport.editions.2026.approximate"),
      footer: t("annualReport.editions.2026.signature"),
    });
    return data;
  });
  return (
    <div className={styles["report-share"]}>
      <button
        type="button"
        className={styles["report-share-trigger"]}
        onClick={() => void generate()}
        disabled={generating}
      >
        {generating ? (
          <LoaderCircle size={17} className="animate-spin" />
        ) : (
          <ImagePlus size={17} />
        )}
        {t(
          generating
            ? "annualReport.editions.2026.shareGenerating"
            : "annualReport.editions.2026.shareGenerate",
        )}
      </button>
      <p className={styles["report-share-note"]}>
        {t("annualReport.editions.2026.shareLocal")}
      </p>
      {error && !open && (
        <p className={styles["report-share-error"]} role="alert">
          {error}
        </p>
      )}
      <Dialog.Root
        open={open}
        onOpenChange={(value) => {
          if (!saving) setOpen(value);
        }}
      >
        <Dialog.Portal>
          <Dialog.Overlay className={styles["report-share-overlay"]} />
          <Dialog.Content className={styles["report-share-dialog"]}>
            <div className={styles["report-share-dialog-heading"]}>
              <Dialog.Title>
                {t("annualReport.editions.2026.shareTitle")}
              </Dialog.Title>
              <Dialog.Close
                className={styles["report-share-close"]}
                aria-label={t("annualReport.editions.2026.closePreview")}
                disabled={saving}
              >
                <X size={18} />
              </Dialog.Close>
            </div>
            <Dialog.Description>
              {t("annualReport.editions.2026.shareDescription")}
            </Dialog.Description>
            <div className={styles["report-poster-preview"]}>
              {image && (
                <img
                  src={image}
                  alt={t("annualReport.editions.2026.posterAlt", {
                    year: report.Year,
                    name,
                  })}
                />
              )}
            </div>
            {error && (
              <p className={styles["report-share-error"]} role="alert">
                {error}
              </p>
            )}
            {saved && (
              <p className={styles["report-share-saved"]} role="status">
                <Check size={14} />
                {t("annualReport.editions.2026.imageSaved")}
              </p>
            )}
            <button
              type="button"
              className={styles["report-share-save"]}
              onClick={() => void save()}
              disabled={saving}
            >
              {saving ? (
                <LoaderCircle size={17} className="animate-spin" />
              ) : (
                <Download size={17} />
              )}
              {t(
                saving
                  ? "annualReport.editions.2026.savingImage"
                  : "annualReport.editions.2026.saveImage",
              )}
            </button>
          </Dialog.Content>
        </Dialog.Portal>
      </Dialog.Root>
    </div>
  );
}

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Dialog } from "radix-ui";
import { Download, ImagePlus, LoaderCircle, Check, X } from "lucide-react";
import { SaveAnnualReportImage } from "@/lib/wailsjs/go/app/App";
import { AppErr, unwrapResult } from "@/utils/wailsResult";
import { errorCodeMessage, toErrorMessage } from "@/utils/error";
import { createSharePoster } from "./sharePoster";
import type { AnnualReport } from "./useAnnualReport";

export default function ReportShare({
  report,
  name,
}: {
  report: AnnualReport;
  name: string;
}) {
  const { t, i18n } = useTranslation();
  const [image, setImage] = useState("");
  const [open, setOpen] = useState(false);
  const [generating, setGenerating] = useState(false);
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState("");
  async function generate() {
    setGenerating(true);
    setError("");
    setSaved(false);
    try {
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
        owner: t("annualReport.posterOwner", { year: report.Year, name }),
        first: t(
          hasData ? "annualReport.heroFirst" : "annualReport.emptyHeroFirst",
        ),
        second: t(
          hasData ? "annualReport.heroSecond" : "annualReport.emptyHeroSecond",
        ),
        words: t("annualReport.wordsAdded"),
        days: t("annualReport.activeDays"),
        characters: t("annualReport.characters"),
        time: t(
          report.Activity.CreativeSeconds > 0 &&
            report.Activity.CreativeSeconds < 3600
            ? "annualReport.posterMinutes"
            : "annualReport.posterHours",
        ),
        summary: t("annualReport.posterSummary", {
          novels: report.Activity.NovelsCreated,
          chapters: report.Activity.ChaptersCreated,
          locations: report.Activity.LocationsCreated,
        }),
        coverage:
          report.Coverage && end
            ? t("annualReport.coverage", {
                start: date(new Date(report.Coverage.Start + "T00:00:00")),
                end: date(end),
              })
            : t("annualReport.uncoveredYear", {
                start: date(new Date(report.TrackingStartDate + "T00:00:00")),
              }),
        approximate: t("annualReport.approximate"),
        footer: t("annualReport.signature"),
      });
      setImage(data);
      setOpen(true);
    } catch (err) {
      setError(`${t("annualReport.shareFailed")}: ${toErrorMessage(err)}`);
    } finally {
      setGenerating(false);
    }
  }
  async function save() {
    setSaving(true);
    setError("");
    setSaved(false);
    try {
      const result = unwrapResult(
        await SaveAnnualReportImage({
          year: report.Year,
          png_base64: image.split(",")[1],
        }),
      );
      setSaved(result);
    } catch (err) {
      const message =
        err instanceof AppErr ? errorCodeMessage(err.errCode, t) : undefined;
      setError(
        message ?? `${t("annualReport.saveFailed")}: ${toErrorMessage(err)}`,
      );
    } finally {
      setSaving(false);
    }
  }
  return (
    <div className="report-share">
      <button
        type="button"
        className="report-share-trigger"
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
            ? "annualReport.shareGenerating"
            : "annualReport.shareGenerate",
        )}
      </button>
      <p className="report-share-note">{t("annualReport.shareLocal")}</p>
      {error && !open && (
        <p className="report-share-error" role="alert">
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
          <Dialog.Overlay className="report-share-overlay" />
          <Dialog.Content className="report-share-dialog">
            <div className="report-share-dialog-heading">
              <Dialog.Title>{t("annualReport.shareTitle")}</Dialog.Title>
              <Dialog.Close
                className="report-share-close"
                aria-label={t("annualReport.closePreview")}
                disabled={saving}
              >
                <X size={18} />
              </Dialog.Close>
            </div>
            <Dialog.Description>
              {t("annualReport.shareDescription")}
            </Dialog.Description>
            <div className="report-poster-preview">
              {image && (
                <img
                  src={image}
                  alt={t("annualReport.posterAlt", { year: report.Year, name })}
                />
              )}
            </div>
            {error && (
              <p className="report-share-error" role="alert">
                {error}
              </p>
            )}
            {saved && (
              <p className="report-share-saved" role="status">
                <Check size={14} />
                {t("annualReport.imageSaved")}
              </p>
            )}
            <button
              type="button"
              className="report-share-save"
              onClick={() => void save()}
              disabled={saving}
            >
              {saving ? (
                <LoaderCircle size={17} className="animate-spin" />
              ) : (
                <Download size={17} />
              )}
              {t(
                saving ? "annualReport.savingImage" : "annualReport.saveImage",
              )}
            </button>
          </Dialog.Content>
        </Dialog.Portal>
      </Dialog.Root>
    </div>
  );
}

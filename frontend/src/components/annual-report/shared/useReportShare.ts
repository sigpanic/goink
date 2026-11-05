import { useState } from "react";
import { useTranslation } from "react-i18next";
import { SaveAnnualReportImage } from "@/lib/wailsjs/go/app/App";
import { AppErr, unwrapResult } from "@/utils/wailsResult";
import { errorCodeMessage, toErrorMessage } from "@/utils/error";

export function useReportShare(
  year: number,
  createPoster: () => Promise<string>,
) {
  const { t } = useTranslation();
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
      setImage(await createPoster());
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
          year,
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
  return {
    image,
    open,
    setOpen,
    generating,
    saving,
    saved,
    error,
    generate,
    save,
  };
}
